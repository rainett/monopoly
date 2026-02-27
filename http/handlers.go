package http

import (
	"encoding/json"
	"log"
	"monopoly/auth"
	"monopoly/game"
	"monopoly/store"
	"monopoly/ws"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// In production, check against allowed origins
		// For now, only allow same origin
		return origin == "" || origin == "http://"+r.Host || origin == "https://"+r.Host
	},
}

type Handlers struct {
	authService *auth.Service
	lobby       *game.Lobby
	engine      *game.Engine
	wsManager   *ws.Manager
	store       store.Store
}

func NewHandlers(authService *auth.Service, lobby *game.Lobby, engine *game.Engine, wsManager *ws.Manager, store store.Store) *Handlers {
	return &Handlers{
		authService: authService,
		lobby:       lobby,
		engine:      engine,
		wsManager:   wsManager,
		store:       store,
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("Failed to write JSON response: %v", err)
	}
}

// Auth handlers
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.authService.Register(req.Username, req.Password); err != nil {
		switch err {
		case auth.ErrInvalidUsername, auth.ErrInvalidPassword, auth.ErrUserExists:
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			log.Printf("Register error: %v", err)
			http.Error(w, "Registration failed", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "User registered successfully"})
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	sessionID, err := h.authService.Login(req.Username, req.Password)
	if err != nil {
		if err == auth.ErrInvalidCredentials {
			http.Error(w, err.Error(), http.StatusUnauthorized)
		} else {
			log.Printf("Login error: %v", err)
			http.Error(w, "Login failed", http.StatusInternalServerError)
		}
		return
	}

	h.authService.GetSessionManager().SetSessionCookie(w, sessionID)

	user, err := h.store.GetUserByUsername(req.Username)
	if err != nil {
		log.Printf("Login: Failed to get user info for %s: %v", req.Username, err)
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}
	if user == nil {
		log.Printf("Login: User not found after successful auth: %s", req.Username)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	log.Printf("Login successful for user %s (ID: %d)", user.Username, user.ID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Login successful",
		"userId":   user.ID,
		"username": user.Username,
	})
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	sessionID := auth.GetSessionFromRequest(r)
	if sessionID != "" {
		h.authService.Logout(sessionID)
		h.authService.GetSessionManager().ClearSessionCookie(w)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Logged out successfully"})
}

// Lobby handlers
func (h *Handlers) ListGames(w http.ResponseWriter, r *http.Request) {
	games, err := h.lobby.ListGames()
	if err != nil {
		log.Printf("ListGames error: %v", err)
		http.Error(w, "Failed to list games", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, games)
}

func (h *Handlers) CreateGame(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxPlayers int `json:"maxPlayers"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.MaxPlayers = 4 // default
	}

	gameID, err := h.lobby.CreateGame(req.MaxPlayers)
	if err != nil {
		log.Printf("CreateGame error: %v", err)
		http.Error(w, "Failed to create game", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"gameId": gameID,
	})
}

func (h *Handlers) JoinGame(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gameID, err := strconv.ParseInt(vars["gameId"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get username
	user, err := h.store.GetUserByID(userID)
	if err != nil {
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	event, err := h.engine.JoinGame(gameID, userID, user.Username)
	if err != nil {
		switch err {
		case game.ErrGameFull, game.ErrGameStarted, game.ErrAlreadyInGame, game.ErrGameNotFound:
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			log.Printf("JoinGame error: %v", err)
			http.Error(w, "Failed to join game", http.StatusInternalServerError)
		}
		return
	}

	// Broadcast to WebSocket room
	room := h.wsManager.GetRoom(gameID)
	room.Broadcast(ws.OutgoingMessage{
		Type:    event.Type,
		Payload: event.Payload,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Joined game successfully",
		"gameId":  gameID,
	})
}

func (h *Handlers) GetGame(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gameID, err := strconv.ParseInt(vars["gameId"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	gameState, err := h.lobby.GetGame(gameID)
	if err != nil {
		log.Printf("GetGame error: %v", err)
		http.Error(w, "Failed to get game", http.StatusInternalServerError)
		return
	}

	if gameState == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, gameState)
}

// WebSocket handler
func (h *Handlers) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gameID, err := strconv.ParseInt(vars["gameId"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	h.wsManager.HandleConnection(conn, gameID, userID)
}
