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
		return true // Allow all origins for development
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "User registered successfully"})
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
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	h.authService.GetSessionManager().SetSessionCookie(w, sessionID)

	user, _ := h.store.GetUserByUsername(req.Username)
	json.NewEncoder(w).Encode(map[string]interface{}{
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

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out successfully"})
}

// Lobby handlers
func (h *Handlers) ListGames(w http.ResponseWriter, r *http.Request) {
	games, err := h.lobby.ListGames()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(games)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Broadcast to WebSocket room
	room := h.wsManager.GetRoom(gameID)
	room.Broadcast(event)

	json.NewEncoder(w).Encode(map[string]interface{}{
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if gameState == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(gameState)
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
