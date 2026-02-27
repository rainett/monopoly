package ws

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// LobbyManager manages WebSocket connections for the lobby
type LobbyManager struct {
	clients map[int64]*LobbyClient
	mu      sync.RWMutex
}

// LobbyClient represents a connected client in the lobby
type LobbyClient struct {
	conn   *websocket.Conn
	userID int64
	send   chan []byte
}

// NewLobbyManager creates a new lobby manager
func NewLobbyManager() *LobbyManager {
	return &LobbyManager{
		clients: make(map[int64]*LobbyClient),
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

// BroadcastUpdate sends a games list update to all connected lobby clients
func (lm *LobbyManager) BroadcastUpdate(games interface{}) {
	message := map[string]interface{}{
		"type":    "games_update",
		"payload": games,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal lobby update: %v", err)
		return
	}

	lm.mu.RLock()
	defer lm.mu.RUnlock()

	for _, client := range lm.clients {
		select {
		case client.send <- data:
		default:
			// Client buffer full, skip
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
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
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
