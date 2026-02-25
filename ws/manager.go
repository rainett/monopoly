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
	rooms  map[int64]*Room
	engine *game.Engine
	mu     sync.RWMutex
}

func NewManager(engine *game.Engine) *Manager {
	return &Manager{
		rooms:  make(map[int64]*Room),
		engine: engine,
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

			w, err := client.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to current websocket message
			n := len(client.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-client.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (m *Manager) handleMessage(client *Client, room *Room, msg *IncomingMessage) {
	var event *game.Event
	var err error

	switch msg.Type {
	case "ready":
		isReady := true
		if readyPayload, ok := msg.Payload["isReady"]; ok {
			if readyBool, ok := readyPayload.(bool); ok {
				isReady = readyBool
			} else {
				log.Printf("Invalid isReady payload type: %T", readyPayload)
				errorMsg := OutgoingMessage{
					Type:    "error",
					Payload: map[string]string{"message": "Invalid isReady value"},
				}
				data, _ := json.Marshal(errorMsg)
				select {
				case client.send <- data:
				default:
				}
				return
			}
		}
		event, err = m.engine.SetReady(room.gameID, client.userID, isReady)

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
	}
}
