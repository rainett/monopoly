package ws

import (
	"encoding/json"
	"log"
	"monopoly/store"
	"sync"

	"github.com/gorilla/websocket"
)

// LobbyLister defines interface for getting game lists
type LobbyLister interface {
	ListGames(userID int64) ([]*store.LobbyGameDTO, error)
	GetGameWithPlayers(gameID, userID int64) (*store.LobbyGameDTO, error)
}

// LobbyManager manages WebSocket connections for the lobby
type LobbyManager struct {
	clients map[int64]*LobbyClient
	lobby   LobbyLister
	mu      sync.RWMutex
}

// LobbyClient represents a connected client in the lobby
type LobbyClient struct {
	conn   *websocket.Conn
	userID int64
	send   chan []byte
}

// NewLobbyManager creates a new lobby manager
func NewLobbyManager(lobby LobbyLister) *LobbyManager {
	return &LobbyManager{
		clients: make(map[int64]*LobbyClient),
		lobby:   lobby,
	}
}

// HandleConnection handles a new WebSocket connection to the lobby
func (lm *LobbyManager) HandleConnection(conn *websocket.Conn, userID int64) {
	client := &LobbyClient{
		conn:   conn,
		userID: userID,
		send:   make(chan []byte, 256),
	}

	lm.mu.Lock()
	lm.clients[userID] = client
	lm.mu.Unlock()

	go client.writePump()
	client.readPump(lm)
}

// BroadcastUpdate sends personalized games list updates to all connected lobby clients
func (lm *LobbyManager) BroadcastUpdate() {
	lm.mu.RLock()
	clients := make([]*LobbyClient, 0, len(lm.clients))
	for _, client := range lm.clients {
		clients = append(clients, client)
	}
	lm.mu.RUnlock()

	// Send personalized update to each client
	for _, client := range clients {
		games, err := lm.lobby.ListGames(client.userID)
		if err != nil {
			log.Printf("Failed to list games for user %d: %v", client.userID, err)
			continue
		}

		message := map[string]interface{}{
			"type":    "games_update",
			"payload": games,
		}

		data, err := json.Marshal(message)
		if err != nil {
			log.Printf("Failed to marshal lobby update: %v", err)
			continue
		}

		select {
		case client.send <- data:
		default:
			// Client buffer full, skip
			log.Printf("Client %d send buffer full, skipping update", client.userID)
		}
	}
}

// readPump handles incoming messages from the client
func (c *LobbyClient) readPump(lm *LobbyManager) {
	defer func() {
		lm.removeClient(c.userID)
		c.conn.Close()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				log.Printf("Lobby WebSocket error: %v", err)
			}
			break
		}
		// Lobby doesn't handle incoming messages, just keeps connection alive
	}
}

// writePump sends messages to the client
func (c *LobbyClient) writePump() {
	defer c.conn.Close()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}
}

// removeClient removes a client from the lobby
func (lm *LobbyManager) removeClient(userID int64) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if client, ok := lm.clients[userID]; ok {
		close(client.send)
		delete(lm.clients, userID)
	}
}
