package ws

import (
	"encoding/json"
	"log"
	"monopoly/game"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type Manager struct {
	rooms        map[int64]*Room
	engine       *game.Engine
	lobbyManager *LobbyManager
	mu           sync.RWMutex
}

func NewManager(engine *game.Engine, lobbyManager *LobbyManager) *Manager {
	return &Manager{
		rooms:        make(map[int64]*Room),
		engine:       engine,
		lobbyManager: lobbyManager,
	}
}

func (m *Manager) GetRoom(gameID int64) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[gameID]
	if !exists {
		room = NewRoom(gameID)
		m.rooms[gameID] = room
	}
	return room
}

func (m *Manager) HandleConnection(conn *websocket.Conn, gameID, userID int64) {
	client := &Client{
		conn:   conn,
		userID: userID,
		send:   make(chan []byte, 256),
	}

	room := m.GetRoom(gameID)
	room.AddClient(client)

	go m.writePump(client)
	go m.readPump(client, room)
}

func (m *Manager) readPump(client *Client, room *Room) {
	defer func() {
		room.RemoveClient(client)
		client.conn.Close()
		m.cleanupRoomIfNeeded(room.gameID)
	}()

	client.conn.SetReadDeadline(time.Now().Add(pongWait))
	client.conn.SetReadLimit(maxMessageSize)
	client.conn.SetPongHandler(func(string) error {
		client.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var inMsg IncomingMessage
		if err := json.Unmarshal(message, &inMsg); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			continue
		}

		m.handleMessage(client, room, &inMsg)
	}
}

func (m *Manager) writePump(client *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		client.conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Send any queued messages, each as its own frame
			n := len(client.send)
			for i := 0; i < n; i++ {
				if err := client.conn.WriteMessage(websocket.TextMessage, <-client.send); err != nil {
					return
				}
			}

		case <-ticker.C:
			client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (m *Manager) cleanupRoomIfNeeded(gameID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[gameID]
	if !exists {
		return
	}

	// Only cleanup if room is empty
	if !room.IsEmpty() {
		return
	}

	// Check if game is finished
	state, err := m.engine.GetGameState(gameID)
	if err != nil || state == nil {
		// If we can't get game state, clean up anyway (game might be deleted)
		delete(m.rooms, gameID)
		log.Printf("Cleaned up room for game %d (game not found)", gameID)
		return
	}

	// Only cleanup finished games
	if state.Status == "finished" {
		delete(m.rooms, gameID)
		log.Printf("Cleaned up empty room for finished game %d", gameID)
	}
}

func (m *Manager) handleMessage(client *Client, room *Room, msg *IncomingMessage) {
	var event *game.Event
	var err error

	switch msg.Type {
	case "end_turn":
		event, err = m.engine.EndTurn(room.gameID, client.userID)

	default:
		log.Printf("Unknown message type: %s", msg.Type)
		return
	}

	if err != nil {
		log.Printf("Error handling message: %v", err)
		// Send error to client
		errorMsg := OutgoingMessage{
			Type:    "error",
			Payload: map[string]string{"message": err.Error()},
		}
		data, _ := json.Marshal(errorMsg)
		select {
		case client.send <- data:
		default:
		}
		return
	}

	if event != nil {
		room.Broadcast(OutgoingMessage{
			Type:    event.Type,
			Payload: event.Payload,
		})

		// Notify lobby of important game status changes
		if event.Type == "game_started" {
			go m.lobbyManager.BroadcastGameStatusChange(room.gameID, "in_progress")
		} else if event.Type == "game_finished" {
			go m.lobbyManager.BroadcastGameStatusChange(room.gameID, "finished")
		}
	}
}
