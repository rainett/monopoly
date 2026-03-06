package game

import (
	"log"
	"sync"
	"time"
)

const TurnTimeout = 60 * time.Second

// TurnTimer manages turn timeouts for games
type TurnTimer struct {
	timers map[int64]*time.Timer // gameID -> timer
	mu     sync.Mutex
	engine *Engine
}

// NewTurnTimer creates a new turn timer manager
func NewTurnTimer(engine *Engine) *TurnTimer {
	return &TurnTimer{
		timers: make(map[int64]*time.Timer),
		engine: engine,
	}
}

// StartTurn starts a timer for the current turn
// If the turn is not ended within 60 seconds, it will auto-skip
func (tt *TurnTimer) StartTurn(gameID, currentPlayerID int64, onTimeout func(*Event)) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	// Cancel any existing timer for this game
	if existingTimer, ok := tt.timers[gameID]; ok {
		existingTimer.Stop()
		delete(tt.timers, gameID)
	}

	// Create new timer
	timer := time.AfterFunc(TurnTimeout, func() {
		log.Printf("Turn timeout for game %d, player %d", gameID, currentPlayerID)

		// Auto-skip the turn (force=true bypasses has_rolled/pending_action checks)
		event, err := tt.engine.ForceEndTurn(gameID, currentPlayerID)
		if err != nil {
			log.Printf("Failed to auto-skip turn: %v", err)
			return
		}

		// Modify event to indicate it was a timeout
		if event != nil {
			// Add timeout flag to payload if possible
			if payload, ok := event.Payload.(TurnChangedPayload); ok {
				event.Type = "turn_timeout"
				event.Payload = map[string]interface{}{
					"previousPlayerId": payload.PreviousPlayerID,
					"currentPlayerId":  payload.CurrentPlayerID,
					"reason":           "timeout",
				}
			} else if payload, ok := event.Payload.(GameFinishedPayload); ok {
				// Game finished with timeout
				event.Type = "game_finished"
				event.Payload = payload
			}
		}

		// Call the callback to broadcast the event
		if onTimeout != nil && event != nil {
			onTimeout(event)
		}

		// Clean up timer reference
		tt.mu.Lock()
		delete(tt.timers, gameID)
		tt.mu.Unlock()
	})

	tt.timers[gameID] = timer
}

// CancelTurn stops the timer for a game (called when turn ends normally)
func (tt *TurnTimer) CancelTurn(gameID int64) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	if timer, ok := tt.timers[gameID]; ok {
		timer.Stop()
		delete(tt.timers, gameID)
	}
}

// CancelAll stops all timers (for cleanup/shutdown)
func (tt *TurnTimer) CancelAll() {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	for gameID, timer := range tt.timers {
		timer.Stop()
		delete(tt.timers, gameID)
	}
}
