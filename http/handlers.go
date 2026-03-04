package http

import (
	"encoding/json"
	"errors"
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
	authService  *auth.Service
	authStore    store.AuthStore
	lobby        *game.Lobby
	engine       *game.Engine
	wsManager    *ws.Manager
	lobbyManager *ws.LobbyManager
}

func NewHandlers(authService *auth.Service, authStore store.AuthStore, lobby *game.Lobby, engine *game.Engine, wsManager *ws.Manager, lobbyManager *ws.LobbyManager) *Handlers {
	return &Handlers{
		authService:  authService,
		authStore:    authStore,
		lobby:        lobby,
		engine:       engine,
		wsManager:    wsManager,
		lobbyManager: lobbyManager,
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("Failed to write JSON response: %v", err)
	}
}

// getUserOrError retrieves a user by ID and writes an HTTP error if not found
func (h *Handlers) getUserOrError(w http.ResponseWriter, userID int64) (*store.User, bool) {
	user, err := h.authStore.GetUserByID(userID)
	if err != nil {
		log.Printf("Failed to get user info for ID %d: %v", userID, err)
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return nil, false
	}
	if user == nil {
		log.Printf("User not found after auth: ID %d", userID)
		http.Error(w, "User not found", http.StatusNotFound)
		return nil, false
	}
	return user, true
}

// broadcastLobbyUpdate sends personalized full state updates to all lobby clients
// This is kept for backward compatibility but should be avoided in favor of specific events
func (h *Handlers) broadcastLobbyUpdate() {
	h.lobbyManager.BroadcastUpdate()
}

// Register Auth handlers
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
		switch {
		case errors.Is(err, auth.ErrInvalidUsername), errors.Is(err, auth.ErrInvalidPassword), errors.Is(err, auth.ErrUserExists):
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

	user, err := h.authStore.GetUserByUsername(req.Username)
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

// ListGames returns a list of active games
func (h *Handlers) ListGames(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	games, err := h.lobby.ListGames(userID)
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

	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	user, ok := h.getUserOrError(w, userID)
	if !ok {
		return
	}

	game, err := h.lobby.CreateGame(req.MaxPlayers, userID, user.Username)
	if err != nil {
		log.Printf("CreateGame error: %v", err)
		http.Error(w, "Failed to create game", http.StatusInternalServerError)
		return
	}

	// Broadcast game_created event to all connected clients
	go h.lobbyManager.BroadcastGameCreated(game.ID)

	writeJSON(w, http.StatusCreated, game)
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

	user, ok := h.getUserOrError(w, userID)
	if !ok {
		return
	}

	// Join game using lobby store
	err = h.lobby.JoinGame(gameID, userID, user.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Broadcast player_joined event to all connected clients
	go h.lobbyManager.BroadcastPlayerJoined(gameID, userID, user.Username)

	// Check if game should start (when game is full)
	event, err := h.engine.StartGameIfFull(gameID)
	if err != nil {
		log.Printf("Error starting game: %v", err)
	} else if event != nil {
		// Game started! Broadcast to game room and lobby
		log.Printf("Game %d started (full)", gameID)

		// Broadcast to game room (if anyone connected)
		gameRoom := h.wsManager.GetRoom(gameID)
		if gameRoom != nil {
			go gameRoom.Broadcast(ws.OutgoingMessage{
				Type:    event.Type,
				Payload: event.Payload,
			})
		}

		// Broadcast status change to lobby
		go h.lobbyManager.BroadcastGameStatusChange(gameID, "in_progress")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Joined game successfully",
		"gameId":  gameID,
	})
}

func (h *Handlers) LeaveGame(w http.ResponseWriter, r *http.Request) {
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

	err = h.lobby.LeaveGame(gameID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Broadcast player_left event to all connected clients
	go h.lobbyManager.BroadcastPlayerLeft(gameID, userID)

	// Check if game still exists (it gets deleted if empty)
	games, err := h.lobby.ListGames(0)
	if err == nil {
		gameExists := false
		for _, g := range games {
			if g.ID == gameID {
				gameExists = true
				break
			}
		}
		// If game doesn't exist anymore, broadcast game_deleted
		if !gameExists {
			go h.lobbyManager.BroadcastGameDeleted(gameID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Left game successfully",
	})
}

func (h *Handlers) GetGame(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gameID, err := strconv.ParseInt(vars["gameId"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	gameState, err := h.engine.GetGameState(gameID)
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

// WebSocket handler for game rooms
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

// WebSocket handler for lobby
func (h *Handlers) HandleLobbyWebSocket(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Lobby WebSocket upgrade error: %v", err)
		return
	}

	h.lobbyManager.HandleConnection(conn, userID)
}
