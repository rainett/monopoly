package ws

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn   *websocket.Conn
	userID int64
	send   chan []byte
}

type Room struct {
	gameID  int64
	clients map[*Client]bool
	mu      sync.RWMutex
}

func NewRoom(gameID int64) *Room {
	return &Room{
		gameID:  gameID,
		clients: make(map[*Client]bool),
	}
}

func (r *Room) AddClient(client *Client) {
	r.mu.Lock()
	r.clients[client] = true
	r.mu.Unlock()
}

func (r *Room) RemoveClient(client *Client) {
	r.mu.Lock()
	if _, ok := r.clients[client]; ok {
		delete(r.clients, client)
		close(client.send)
	}
	r.mu.Unlock()
}

func (r *Room) Broadcast(message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for client := range r.clients {
		select {
		case client.send <- data:
		default:
			// Client's send channel is full, skip
			log.Printf("Client %d send buffer full", client.userID)
		}
	}
}

func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}
