package http

import (
	"encoding/json"
	"log"
	"monopoly/auth"
	"monopoly/errors"
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

// writeError writes an error response with proper handling of AppError types
func writeError(w http.ResponseWriter, err error) {
	var appErr *errors.AppError
	if e, ok := err.(*errors.AppError); ok {
		appErr = e
	} else {
		// Wrap unknown errors
		appErr = errors.InternalError(err.Error())
	}

	// Log internal details
	if appErr.Detail != "" {
		log.Printf("Error [%s]: %s (detail: %s)", appErr.Code, appErr.Message, appErr.Detail)
	} else {
		log.Printf("Error [%s]: %s", appErr.Code, appErr.Message)
	}

	// Determine HTTP status code based on error code
	statusCode := http.StatusInternalServerError
	switch appErr.Code {
	case errors.ErrCodeUnauthorized:
		statusCode = http.StatusUnauthorized
	case errors.ErrCodeInvalidCredentials:
		statusCode = http.StatusUnauthorized
	case errors.ErrCodeNotFound, errors.ErrCodeGameNotFound, errors.ErrCodeUserNotFound:
		statusCode = http.StatusNotFound
	case errors.ErrCodeBadRequest, errors.ErrCodeInvalidUsername, errors.ErrCodeInvalidPassword:
		statusCode = http.StatusBadRequest
	case errors.ErrCodeForbidden, errors.ErrCodeNotPlayer:
		statusCode = http.StatusForbidden
	case errors.ErrCodeGameFull, errors.ErrCodeGameStarted, errors.ErrCodeAlreadyInGame,
		errors.ErrCodeNotInGame, errors.ErrCodeNotYourTurn, errors.ErrCodeUserExists,
		errors.ErrCodeAlreadyRolled, errors.ErrCodeMustRoll, errors.ErrCodePendingAction,
		errors.ErrCodeCannotBuy, errors.ErrCodeInsufficientFunds, errors.ErrCodePlayerBankrupt:
		statusCode = http.StatusBadRequest
	}

	writeJSON(w, statusCode, map[string]interface{}{
		"error":   string(appErr.Code),
		"message": appErr.UserMessage(),
	})
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

// checkUserInGame verifies if user is a player in the specified game
// Returns true if user is in game, false otherwise
func (h *Handlers) checkUserInGame(gameID, userID int64) (bool, error) {
	gameState, err := h.engine.GetGameState(gameID)
	if err != nil {
		return false, err
	}
	if gameState == nil {
		return false, errors.GameNotFound()
	}

	for _, player := range gameState.Players {
		if player.UserID == userID {
			return true, nil
		}
	}
	return false, nil
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
		writeError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.authService.Register(req.Username, req.Password); err != nil {
		writeError(w, err)
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
		writeError(w, errors.BadRequest("Invalid request body"))
		return
	}

	sessionID, err := h.authService.Login(req.Username, req.Password)
	if err != nil {
		writeError(w, err)
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

		// Broadcast to game room with turn timer handling
		go h.wsManager.BroadcastGameEvent(gameID, event)

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

	// Check if user is a player in this game
	isPlayer, err := h.checkUserInGame(gameID, userID)
	if err != nil {
		log.Printf("Failed to check game authorization: %v", err)
		http.Error(w, "Failed to verify game access", http.StatusInternalServerError)
		return
	}

	if !isPlayer {
		log.Printf("User %d attempted to access game %d without being a player", userID, gameID)
		http.Error(w, "You are not a player in this game", http.StatusForbidden)
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

// Friends handlers

func (h *Handlers) SearchUsers(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query().Get("q")
	if len(query) < 2 {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	users, err := h.authStore.SearchUsers(query, userID, 10)
	if err != nil {
		log.Printf("SearchUsers error: %v", err)
		http.Error(w, "Failed to search users", http.StatusInternalServerError)
		return
	}

	// Convert to safe response format
	result := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		result = append(result, map[string]interface{}{
			"userId":   u.ID,
			"username": u.Username,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) SendFriendRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		UserID int64 `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.UserID == userID {
		http.Error(w, "Cannot send friend request to yourself", http.StatusBadRequest)
		return
	}

	err := h.authStore.SendFriendRequest(userID, req.UserID)
	if err != nil {
		log.Printf("SendFriendRequest error: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Friend request sent"})
}

func (h *Handlers) AcceptFriendRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	friendID, err := strconv.ParseInt(vars["friendId"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid friend ID", http.StatusBadRequest)
		return
	}

	err = h.authStore.AcceptFriendRequest(userID, friendID)
	if err != nil {
		log.Printf("AcceptFriendRequest error: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Friend request accepted"})
}

func (h *Handlers) DeclineFriendRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	friendID, err := strconv.ParseInt(vars["friendId"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid friend ID", http.StatusBadRequest)
		return
	}

	err = h.authStore.DeclineFriendRequest(userID, friendID)
	if err != nil {
		log.Printf("DeclineFriendRequest error: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Friend request declined"})
}

func (h *Handlers) GetFriends(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	friends, err := h.authStore.GetFriends(userID)
	if err != nil {
		log.Printf("GetFriends error: %v", err)
		http.Error(w, "Failed to get friends", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(friends))
	for _, f := range friends {
		result = append(result, map[string]interface{}{
			"userId":   f.ID,
			"username": f.Username,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) GetPendingRequests(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	requests, err := h.authStore.GetPendingRequests(userID)
	if err != nil {
		log.Printf("GetPendingRequests error: %v", err)
		http.Error(w, "Failed to get requests", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(requests))
	for _, req := range requests {
		result = append(result, map[string]interface{}{
			"fromUserId":   req.FromUserID,
			"fromUsername": req.FromUsername,
			"createdAt":    req.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, result)
}
