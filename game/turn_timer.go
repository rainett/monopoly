package game

import (
	"log"
	"sync"
	"time"
)

const TurnTimeout = 60 * time.Second
const MaxConsecutiveTimeouts = 3

// TurnTimer manages turn timeouts for games
type TurnTimer struct {
	timers           map[int64]*time.Timer    // gameID -> timer
	timeoutCounts    map[int64]map[int64]int  // gameID -> userID -> consecutive timeout count
	currentPlayerIDs map[int64]int64          // gameID -> current player ID (for tracking)
	mu               sync.Mutex
	engine           *Engine
}

// NewTurnTimer creates a new turn timer manager
func NewTurnTimer(engine *Engine) *TurnTimer {
	return &TurnTimer{
		timers:           make(map[int64]*time.Timer),
		timeoutCounts:    make(map[int64]map[int64]int),
		currentPlayerIDs: make(map[int64]int64),
		engine:           engine,
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

	// Track current player
	tt.currentPlayerIDs[gameID] = currentPlayerID

	// Initialize timeout counts for game if needed
	if tt.timeoutCounts[gameID] == nil {
		tt.timeoutCounts[gameID] = make(map[int64]int)
	}

	// Create new timer
	timer := time.AfterFunc(TurnTimeout, func() {
		log.Printf("Turn timeout for game %d, player %d", gameID, currentPlayerID)

		tt.mu.Lock()
		// Increment timeout count for this player
		if tt.timeoutCounts[gameID] == nil {
			tt.timeoutCounts[gameID] = make(map[int64]int)
		}
		tt.timeoutCounts[gameID][currentPlayerID]++
		timeoutCount := tt.timeoutCounts[gameID][currentPlayerID]
		tt.mu.Unlock()

		log.Printf("Player %d has %d consecutive timeouts", currentPlayerID, timeoutCount)

		var event *Event
		var err error

		// Check if player should be eliminated (3 consecutive timeouts)
		if timeoutCount >= MaxConsecutiveTimeouts {
			log.Printf("Player %d eliminated due to %d consecutive timeouts", currentPlayerID, timeoutCount)
			event, err = tt.engine.EliminatePlayerForTimeouts(gameID, currentPlayerID)
		} else {
			// Auto-skip the turn (force=true bypasses has_rolled/pending_action checks)
			event, err = tt.engine.ForceEndTurn(gameID, currentPlayerID)
		}

		if err != nil {
			log.Printf("Failed to handle timeout: %v", err)
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
					"timeoutCount":     timeoutCount,
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

// ResetPlayerTimeouts resets the consecutive timeout count for a player
// Called when a player takes an action (rolls dice, ends turn manually, etc.)
func (tt *TurnTimer) ResetPlayerTimeouts(gameID, userID int64) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	if tt.timeoutCounts[gameID] != nil {
		if tt.timeoutCounts[gameID][userID] > 0 {
			log.Printf("Reset timeout count for player %d in game %d", userID, gameID)
			tt.timeoutCounts[gameID][userID] = 0
		}
	}
}

// GetTimeoutCount returns the current consecutive timeout count for a player
func (tt *TurnTimer) GetTimeoutCount(gameID, userID int64) int {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	if tt.timeoutCounts[gameID] == nil {
		return 0
	}
	return tt.timeoutCounts[gameID][userID]
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
