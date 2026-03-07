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
	} else if event.Type == "auction_started" {
		// Start timer for first bidder
		if payload, ok := event.Payload.(game.AuctionStartedPayload); ok {
			m.startTurnTimer(gameID, payload.CurrentBidder, room)
		}
	} else if event.Type == "auction_bid" {
		// Start timer for next bidder
		if payload, ok := event.Payload.(game.AuctionBidPayload); ok {
			m.startTurnTimer(gameID, payload.NextBidderID, room)
		}
	} else if event.Type == "auction_passed" {
		// Start timer for next bidder
		if payload, ok := event.Payload.(game.AuctionPassedPayload); ok {
			m.startTurnTimer(gameID, payload.NextBidderID, room)
		}
	} else if event.Type == "auction_ended" {
		// Don't cancel timer here - turn_changed will handle it
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

	// If game is already in progress, send timer_started event to the new client
	// This ensures players see the timer even if they connect after the game starts
	go func() {
		state, err := m.engine.GetGameState(gameID)
		if err == nil && state != nil && state.Status == game.StatusInProgress {
			// Find current player and send timer_started
			for _, p := range state.Players {
				if p.IsCurrentTurn {
					timerMsg := OutgoingMessage{
						Type: "timer_started",
						Payload: map[string]interface{}{
							"playerId": p.UserID,
							"duration": int(game.TurnTimeout.Seconds()),
						},
					}
					data, _ := json.Marshal(timerMsg)
					select {
					case client.send <- data:
					default:
					}
					break
				}
			}
		}
	}()

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
		m.handleMultiEventWithTimerRestart(client, room, func() ([]*game.Event, error) {
			return m.engine.BuyProperty(room.gameID, client.userID)
		})
	case "pass_property":
		m.handleMultiEventWithTimerRestart(client, room, func() ([]*game.Event, error) {
			return m.engine.PassProperty(room.gameID, client.userID)
		})
	case "place_bid":
		m.handlePlaceBid(client, room, msg)
	case "pass_auction":
		m.handlePassAuction(client, room)
	case "end_turn":
		m.turnTimer.CancelTurn(room.gameID)
		m.handleSingleEvent(client, room, func() (*game.Event, error) {
			return m.engine.EndTurn(room.gameID, client.userID)
		})
	case "chat":
		m.handleChat(client, room, msg)
	case "pay_jail_bail":
		m.handleMultiEventWithTimerRestart(client, room, func() ([]*game.Event, error) {
			return m.engine.PayJailBail(room.gameID, client.userID)
		})
	case "use_jail_card":
		m.handleMultiEventWithTimerRestart(client, room, func() ([]*game.Event, error) {
			return m.engine.UseJailFreeCard(room.gameID, client.userID)
		})
	case "mortgage_property":
		m.handleMortgageWithTimerRestart(client, room, msg)
	case "unmortgage_property":
		m.handleUnmortgageWithTimerRestart(client, room, msg)
	case "buy_house":
		m.handleBuyHouseWithTimerRestart(client, room, msg)
	case "sell_house":
		m.handleSellHouseWithTimerRestart(client, room, msg)
	case "propose_trade":
		m.handleProposeTrade(client, room, msg)
	case "accept_trade":
		m.handleAcceptTrade(client, room, msg)
	case "decline_trade":
		m.handleDeclineTrade(client, room, msg)
	case "cancel_trade":
		m.handleCancelTrade(client, room, msg)
	case "give_up":
		m.handleGiveUp(client, room)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

func (m *Manager) handleMortgage(client *Client, room *Room, msg *IncomingMessage) {
	posFloat, ok := msg.Payload["position"].(float64)
	if !ok {
		return
	}
	position := int(posFloat)

	m.handleSingleEvent(client, room, func() (*game.Event, error) {
		return m.engine.MortgageProperty(room.gameID, client.userID, position)
	})
}

func (m *Manager) handleMortgageWithTimerRestart(client *Client, room *Room, msg *IncomingMessage) {
	posFloat, ok := msg.Payload["position"].(float64)
	if !ok {
		return
	}
	position := int(posFloat)

	m.handleSingleEventWithTimerRestart(client, room, func() (*game.Event, error) {
		return m.engine.MortgageProperty(room.gameID, client.userID, position)
	})
}

func (m *Manager) handleUnmortgage(client *Client, room *Room, msg *IncomingMessage) {
	posFloat, ok := msg.Payload["position"].(float64)
	if !ok {
		return
	}
	position := int(posFloat)

	m.handleSingleEvent(client, room, func() (*game.Event, error) {
		return m.engine.UnmortgageProperty(room.gameID, client.userID, position)
	})
}

func (m *Manager) handleUnmortgageWithTimerRestart(client *Client, room *Room, msg *IncomingMessage) {
	posFloat, ok := msg.Payload["position"].(float64)
	if !ok {
		return
	}
	position := int(posFloat)

	m.handleSingleEventWithTimerRestart(client, room, func() (*game.Event, error) {
		return m.engine.UnmortgageProperty(room.gameID, client.userID, position)
	})
}

func (m *Manager) handleBuyHouse(client *Client, room *Room, msg *IncomingMessage) {
	posFloat, ok := msg.Payload["position"].(float64)
	if !ok {
		return
	}
	position := int(posFloat)

	m.handleSingleEvent(client, room, func() (*game.Event, error) {
		return m.engine.BuyHouse(room.gameID, client.userID, position)
	})
}

func (m *Manager) handleBuyHouseWithTimerRestart(client *Client, room *Room, msg *IncomingMessage) {
	posFloat, ok := msg.Payload["position"].(float64)
	if !ok {
		return
	}
	position := int(posFloat)

	m.handleSingleEventWithTimerRestart(client, room, func() (*game.Event, error) {
		return m.engine.BuyHouse(room.gameID, client.userID, position)
	})
}

func (m *Manager) handleSellHouse(client *Client, room *Room, msg *IncomingMessage) {
	posFloat, ok := msg.Payload["position"].(float64)
	if !ok {
		return
	}
	position := int(posFloat)

	m.handleSingleEvent(client, room, func() (*game.Event, error) {
		return m.engine.SellHouse(room.gameID, client.userID, position)
	})
}

func (m *Manager) handleSellHouseWithTimerRestart(client *Client, room *Room, msg *IncomingMessage) {
	posFloat, ok := msg.Payload["position"].(float64)
	if !ok {
		return
	}
	position := int(posFloat)

	m.handleSingleEventWithTimerRestart(client, room, func() (*game.Event, error) {
		return m.engine.SellHouse(room.gameID, client.userID, position)
	})
}

func (m *Manager) handleProposeTrade(client *Client, room *Room, msg *IncomingMessage) {
	toUserIDFloat, ok := msg.Payload["toUserId"].(float64)
	if !ok {
		return
	}
	toUserID := int64(toUserIDFloat)

	offer := game.TradeOffer{}

	if offerMap, ok := msg.Payload["offer"].(map[string]interface{}); ok {
		if v, ok := offerMap["offeredMoney"].(float64); ok {
			offer.OfferedMoney = int(v)
		}
		if v, ok := offerMap["requestedMoney"].(float64); ok {
			offer.RequestedMoney = int(v)
		}
		if arr, ok := offerMap["offeredProperties"].([]interface{}); ok {
			for _, v := range arr {
				if f, ok := v.(float64); ok {
					offer.OfferedProperties = append(offer.OfferedProperties, int(f))
				}
			}
		}
		if arr, ok := offerMap["requestedProperties"].([]interface{}); ok {
			for _, v := range arr {
				if f, ok := v.(float64); ok {
					offer.RequestedProperties = append(offer.RequestedProperties, int(f))
				}
			}
		}
	}

	m.handleSingleEvent(client, room, func() (*game.Event, error) {
		return m.engine.ProposeTrade(room.gameID, client.userID, toUserID, offer)
	})
}

func (m *Manager) handleAcceptTrade(client *Client, room *Room, msg *IncomingMessage) {
	tradeIDFloat, ok := msg.Payload["tradeId"].(float64)
	if !ok {
		return
	}
	tradeID := int64(tradeIDFloat)

	m.handleMultiEvent(client, room, func() ([]*game.Event, error) {
		return m.engine.AcceptTrade(room.gameID, client.userID, tradeID)
	})
}

func (m *Manager) handleDeclineTrade(client *Client, room *Room, msg *IncomingMessage) {
	tradeIDFloat, ok := msg.Payload["tradeId"].(float64)
	if !ok {
		return
	}
	tradeID := int64(tradeIDFloat)

	m.handleSingleEvent(client, room, func() (*game.Event, error) {
		return m.engine.DeclineTrade(room.gameID, client.userID, tradeID)
	})
}

func (m *Manager) handleCancelTrade(client *Client, room *Room, msg *IncomingMessage) {
	tradeIDFloat, ok := msg.Payload["tradeId"].(float64)
	if !ok {
		return
	}
	tradeID := int64(tradeIDFloat)

	m.handleSingleEvent(client, room, func() (*game.Event, error) {
		return m.engine.CancelTrade(room.gameID, client.userID, tradeID)
	})
}

func (m *Manager) handleChat(client *Client, room *Room, msg *IncomingMessage) {
	// Extract message text from payload
	text, ok := msg.Payload["message"].(string)
	if !ok || text == "" {
		return
	}

	// Limit message length
	if len(text) > 200 {
		text = text[:200]
	}

	// Get username from game state
	state, err := m.engine.GetGameState(room.gameID)
	if err != nil {
		return
	}

	var username string
	for _, p := range state.Players {
		if p.UserID == client.userID {
			username = p.Username
			break
		}
	}
	if username == "" {
		return
	}

	// Broadcast chat message to room
	room.Broadcast(OutgoingMessage{
		Type: "chat",
		Payload: map[string]interface{}{
			"userId":   client.userID,
			"username": username,
			"message":  text,
		},
	})
}

func (m *Manager) handleRollDice(client *Client, room *Room) {
	events, err := m.engine.RollDice(room.gameID, client.userID)
	if err != nil {
		m.sendError(client, err)
		return
	}

	// Check if turn changed (auto-end) or game finished - if so, don't restart timer
	turnEnded := false
	for _, event := range events {
		if event.Type == "turn_changed" || event.Type == "game_finished" {
			turnEnded = true
		}
	}

	for _, event := range events {
		room.Broadcast(OutgoingMessage{
			Type:    event.Type,
			Payload: event.Payload,
		})
		m.handleEventSideEffects(event, room)
	}

	// Restart timer only if turn didn't end
	if !turnEnded && len(events) > 0 {
		m.restartTurnTimer(room.gameID, client.userID, room)
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

func (m *Manager) handleMultiEvent(client *Client, room *Room, action func() ([]*game.Event, error)) {
	events, err := action()
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

func (m *Manager) handleMultiEventWithTimerRestart(client *Client, room *Room, action func() ([]*game.Event, error)) {
	events, err := action()
	if err != nil {
		m.sendError(client, err)
		return
	}

	// Check if turn changed or game finished - if so, don't restart timer
	turnEnded := false
	for _, event := range events {
		if event.Type == "turn_changed" || event.Type == "game_finished" {
			turnEnded = true
		}
	}

	for _, event := range events {
		room.Broadcast(OutgoingMessage{
			Type:    event.Type,
			Payload: event.Payload,
		})
		m.handleEventSideEffects(event, room)
	}

	// Restart timer only if action succeeded and turn didn't end
	if !turnEnded && len(events) > 0 {
		m.restartTurnTimer(room.gameID, client.userID, room)
	}
}

func (m *Manager) handleSingleEventWithTimerRestart(client *Client, room *Room, action func() (*game.Event, error)) {
	event, err := action()
	if err != nil {
		m.sendError(client, err)
		return
	}

	turnEnded := false
	if event != nil {
		if event.Type == "turn_changed" || event.Type == "game_finished" {
			turnEnded = true
		}
		room.Broadcast(OutgoingMessage{
			Type:    event.Type,
			Payload: event.Payload,
		})
		m.handleEventSideEffects(event, room)
	}

	// Restart timer only if action succeeded and turn didn't end
	if !turnEnded && event != nil {
		m.restartTurnTimer(room.gameID, client.userID, room)
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
	// Broadcast timer_started event so frontend can display countdown
	room.Broadcast(OutgoingMessage{
		Type: "timer_started",
		Payload: map[string]interface{}{
			"playerId": currentPlayerID,
			"duration": int(game.TurnTimeout.Seconds()),
		},
	})

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
					// The payload is set directly in Go, so values are int64
					if nextPlayerID, ok := payload["currentPlayerId"].(int64); ok {
						m.startTurnTimer(gameID, nextPlayerID, room)
					}
				}
			}
		}
	})
}

// restartTurnTimer restarts the 60s timer when a player takes an action.
// This resets the countdown and broadcasts timer_started to update the frontend.
func (m *Manager) restartTurnTimer(gameID, currentPlayerID int64, room *Room) {
	// Broadcast timer_started event so frontend can reset the countdown display
	room.Broadcast(OutgoingMessage{
		Type: "timer_started",
		Payload: map[string]interface{}{
			"playerId": currentPlayerID,
			"duration": int(game.TurnTimeout.Seconds()),
		},
	})

	m.turnTimer.RestartTurn(gameID, currentPlayerID, func(event *game.Event) {
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

func (m *Manager) handlePlaceBid(client *Client, room *Room, msg *IncomingMessage) {
	amountFloat, ok := msg.Payload["amount"].(float64)
	if !ok {
		return
	}
	amount := int(amountFloat)

	m.handleMultiEvent(client, room, func() ([]*game.Event, error) {
		return m.engine.PlaceBid(room.gameID, client.userID, amount)
	})
}

func (m *Manager) handlePassAuction(client *Client, room *Room) {
	m.handleMultiEvent(client, room, func() ([]*game.Event, error) {
		return m.engine.PassAuction(room.gameID, client.userID)
	})
}

func (m *Manager) handleGiveUp(client *Client, room *Room) {
	m.turnTimer.CancelTurn(room.gameID)
	m.handleMultiEvent(client, room, func() ([]*game.Event, error) {
		return m.engine.GiveUp(room.gameID, client.userID)
	})
}
