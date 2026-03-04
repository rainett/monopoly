package ws

import (
	"encoding/json"
	"log"
	"monopoly/store"
)

// Lobby event types
const (
	EventGameCreated      = "game_created"
	EventGameDeleted      = "game_deleted"
	EventPlayerJoined     = "player_joined"
	EventPlayerLeft       = "player_left"
	EventGameStatusChange = "game_status_changed"
)

// GameCreatedPayload contains data for a newly created game
type GameCreatedPayload struct {
	Game *store.LobbyGameDTO `json:"game"`
}

// GameDeletedPayload contains the ID of a deleted game
type GameDeletedPayload struct {
	GameID int64 `json:"gameId"`
}

// PlayerJoinedPayload contains data about a player joining a game
type PlayerJoinedPayload struct {
	GameID   int64               `json:"gameId"`
	Player   store.LobbyPlayerDTO `json:"player"`
	IsYou    bool                `json:"isYou"` // true if this is the current user
}

// PlayerLeftPayload contains data about a player leaving a game
type PlayerLeftPayload struct {
	GameID int64 `json:"gameId"`
	UserID int64 `json:"userId"`
	IsYou  bool  `json:"isYou"` // true if this is the current user
}

// GameStatusChangePayload contains data about a game status change
type GameStatusChangePayload struct {
	GameID int64  `json:"gameId"`
	Status string `json:"status"`
}

// BroadcastGameCreated sends a game_created event to all connected lobby clients
func (lm *LobbyManager) BroadcastGameCreated(gameID int64) {
	lm.mu.RLock()
	clients := make([]*LobbyClient, 0, len(lm.clients))
	for _, client := range lm.clients {
		clients = append(clients, client)
	}
	lm.mu.RUnlock()

	for _, client := range clients {
		// Get game with personalized isJoined flag for this client
		game, err := lm.lobby.GetGameWithPlayers(gameID, client.userID)
		if err != nil {
			log.Printf("Failed to get game %d for client %d: %v", gameID, client.userID, err)
			continue
		}

		if game == nil {
			continue
		}

		payload := GameCreatedPayload{Game: game}
		lm.sendToClient(client, EventGameCreated, payload)
	}
}

// BroadcastGameDeleted sends a game_deleted event to all connected lobby clients
func (lm *LobbyManager) BroadcastGameDeleted(gameID int64) {
	payload := GameDeletedPayload{GameID: gameID}
	lm.broadcastToAll(EventGameDeleted, payload)
}

// BroadcastPlayerJoined sends a player_joined event to all connected lobby clients
func (lm *LobbyManager) BroadcastPlayerJoined(gameID, userID int64, username string) {
	lm.mu.RLock()
	clients := make([]*LobbyClient, 0, len(lm.clients))
	for _, client := range lm.clients {
		clients = append(clients, client)
	}
	lm.mu.RUnlock()

	player := store.LobbyPlayerDTO{
		UserID:   userID,
		Username: username,
	}

	for _, client := range clients {
		payload := PlayerJoinedPayload{
			GameID: gameID,
			Player: player,
			IsYou:  client.userID == userID,
		}
		lm.sendToClient(client, EventPlayerJoined, payload)
	}
}

// BroadcastPlayerLeft sends a player_left event to all connected lobby clients
func (lm *LobbyManager) BroadcastPlayerLeft(gameID, userID int64) {
	lm.mu.RLock()
	clients := make([]*LobbyClient, 0, len(lm.clients))
	for _, client := range lm.clients {
		clients = append(clients, client)
	}
	lm.mu.RUnlock()

	for _, client := range clients {
		payload := PlayerLeftPayload{
			GameID: gameID,
			UserID: userID,
			IsYou:  client.userID == userID,
		}
		lm.sendToClient(client, EventPlayerLeft, payload)
	}
}

// BroadcastGameStatusChange sends a game_status_changed event to all connected lobby clients
func (lm *LobbyManager) BroadcastGameStatusChange(gameID int64, status string) {
	payload := GameStatusChangePayload{
		GameID: gameID,
		Status: status,
	}
	lm.broadcastToAll(EventGameStatusChange, payload)
}

// sendToClient sends a message to a specific client
func (lm *LobbyManager) sendToClient(client *LobbyClient, eventType string, payload interface{}) {
	message := map[string]interface{}{
		"type":    eventType,
		"payload": payload,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	select {
	case client.send <- data:
	default:
		log.Printf("Client %d send buffer full, skipping event", client.userID)
	}
}

// broadcastToAll sends the same message to all connected clients
func (lm *LobbyManager) broadcastToAll(eventType string, payload interface{}) {
	message := map[string]interface{}{
		"type":    eventType,
		"payload": payload,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	lm.mu.RLock()
	defer lm.mu.RUnlock()

	for _, client := range lm.clients {
		select {
		case client.send <- data:
		default:
			log.Printf("Client %d send buffer full, skipping event", client.userID)
		}
	}
}
