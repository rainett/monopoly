package ws

import (
	"encoding/json"
	"log"
	"monopoly/errors"
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
	turnTimer    *game.TurnTimer
	mu           sync.RWMutex
}

func NewManager(engine *game.Engine, lobbyManager *LobbyManager) *Manager {
	m := &Manager{
		rooms:        make(map[int64]*Room),
		engine:       engine,
		lobbyManager: lobbyManager,
	}
	m.turnTimer = game.NewTurnTimer(engine)
	return m
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

// BroadcastGameEvent broadcasts a game event to a room and handles turn timer
func (m *Manager) BroadcastGameEvent(gameID int64, event *game.Event) {
	room := m.GetRoom(gameID)

	room.Broadcast(OutgoingMessage{
		Type:    event.Type,
		Payload: event.Payload,
	})

	// Handle turn timer based on event type
	if event.Type == "game_started" {
		if payload, ok := event.Payload.(game.GameStartedPayload); ok {
			m.startTurnTimer(gameID, payload.CurrentPlayerID, room)
		}
	} else if event.Type == "turn_changed" {
		if payload, ok := event.Payload.(game.TurnChangedPayload); ok {
			m.startTurnTimer(gameID, payload.CurrentPlayerID, room)
		}
	} else if event.Type == "game_finished" {
		m.turnTimer.CancelTurn(gameID)
	}
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
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
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
	switch msg.Type {
	case "roll_dice":
		m.handleRollDice(client, room)
	case "buy_property":
		m.handleSingleEvent(client, room, func() (*game.Event, error) {
			return m.engine.BuyProperty(room.gameID, client.userID)
		})
	case "pass_property":
		m.handleSingleEvent(client, room, func() (*game.Event, error) {
			return m.engine.PassProperty(room.gameID, client.userID)
		})
	case "end_turn":
		m.turnTimer.CancelTurn(room.gameID)
		m.handleSingleEvent(client, room, func() (*game.Event, error) {
			return m.engine.EndTurn(room.gameID, client.userID)
		})
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

func (m *Manager) handleRollDice(client *Client, room *Room) {
	events, err := m.engine.RollDice(room.gameID, client.userID)
	if err != nil {
		m.sendError(client, err)
		return
	}

	for _, event := range events {
		room.Broadcast(OutgoingMessage{
			Type:    event.Type,
			Payload: event.Payload,
		})
		m.handleEventSideEffects(event, room)
	}
}

func (m *Manager) handleSingleEvent(client *Client, room *Room, action func() (*game.Event, error)) {
	event, err := action()
	if err != nil {
		m.sendError(client, err)
		return
	}

	if event != nil {
		room.Broadcast(OutgoingMessage{
			Type:    event.Type,
			Payload: event.Payload,
		})
		m.handleEventSideEffects(event, room)
	}
}

func (m *Manager) handleEventSideEffects(event *game.Event, room *Room) {
	if event == nil {
		return
	}

	switch event.Type {
	case "game_started":
		if payload, ok := event.Payload.(game.GameStartedPayload); ok {
			m.startTurnTimer(room.gameID, payload.CurrentPlayerID, room)
		}
		go m.lobbyManager.BroadcastGameStatusChange(room.gameID, "in_progress")
	case "turn_changed":
		if payload, ok := event.Payload.(game.TurnChangedPayload); ok {
			m.startTurnTimer(room.gameID, payload.CurrentPlayerID, room)
		}
	case "game_finished":
		m.turnTimer.CancelTurn(room.gameID)
		go m.lobbyManager.BroadcastGameStatusChange(room.gameID, "finished")
	}
}

func (m *Manager) sendError(client *Client, err error) {
	var userMessage string
	var errorCode string

	if appErr, ok := err.(*errors.AppError); ok {
		userMessage = appErr.UserMessage()
		errorCode = string(appErr.Code)
		log.Printf("WS Error [%s]: %s", appErr.Code, appErr.Error())
	} else {
		userMessage = "An error occurred. Please try again."
		errorCode = "UNKNOWN_ERROR"
		log.Printf("WS Error: %v", err)
	}

	errorMsg := OutgoingMessage{
		Type: "error",
		Payload: map[string]string{
			"code":    errorCode,
			"message": userMessage,
		},
	}
	data, _ := json.Marshal(errorMsg)
	select {
	case client.send <- data:
	default:
	}
}

// startTurnTimer starts a timer for the current player's turn
func (m *Manager) startTurnTimer(gameID, currentPlayerID int64, room *Room) {
	m.turnTimer.StartTurn(gameID, currentPlayerID, func(event *game.Event) {
		// Broadcast timeout event to room
		if event != nil {
			room.Broadcast(OutgoingMessage{
				Type:    event.Type,
				Payload: event.Payload,
			})

			// If game finished due to timeout, notify lobby
			if event.Type == "game_finished" {
				go m.lobbyManager.BroadcastGameStatusChange(gameID, "finished")
			} else if event.Type == "turn_timeout" {
				// Start timer for next player if turn changed
				if payload, ok := event.Payload.(map[string]interface{}); ok {
					if nextPlayerID, ok := payload["currentPlayerId"].(int64); ok {
						m.startTurnTimer(gameID, nextPlayerID, room)
					}
				}
			}
		}
	})
}
