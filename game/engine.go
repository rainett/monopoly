package game

import (
	"errors"
	"monopoly/store"
)

var (
	ErrGameFull         = errors.New("game is full")
	ErrGameStarted      = errors.New("game already started")
	ErrNotEnoughPlayers = errors.New("not enough players to start")
	ErrNotYourTurn      = errors.New("not your turn")
	ErrGameNotStarted   = errors.New("game not started")
	ErrGameNotFound     = errors.New("game not found")
	ErrAlreadyInGame    = errors.New("already in game")
	ErrUserNotInGame    = errors.New("user not in game")
)

type Engine struct {
	store store.Store
}

func NewEngine(store store.Store) *Engine {
	return &Engine{store: store}
}

func (e *Engine) GetGameState(gameID int64) (*GameState, error) {
	if gameID <= 0 {
		return nil, ErrGameNotFound
	}

	game, err := e.store.GetGame(gameID)
	if err != nil {
		return nil, err
	}
	if game == nil {
		return nil, ErrGameNotFound
	}

	players, err := e.store.GetGamePlayers(gameID)
	if err != nil {
		return nil, err
	}

	gamePlayers := make([]*Player, len(players))
	var currentPlayerID int64
	for i, p := range players {
		gamePlayers[i] = &Player{
			UserID:        p.UserID,
			Username:      p.Username,
			Order:         p.PlayerOrder,
			IsReady:       p.IsReady,
			IsCurrentTurn: p.IsCurrentTurn,
		}
		if p.IsCurrentTurn {
			currentPlayerID = p.UserID
		}
	}

	return &GameState{
		ID:              game.ID,
		Status:          game.Status,
		Players:         gamePlayers,
		CurrentPlayerID: currentPlayerID,
		MaxPlayers:      game.MaxPlayers,
	}, nil
}

func (e *Engine) JoinGame(gameID, userID int64, username string) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusWaiting {
		return nil, ErrGameStarted
	}

	if len(state.Players) >= state.MaxPlayers {
		return nil, ErrGameFull
	}

	// Check if user already in game
	for _, p := range state.Players {
		if p.UserID == userID {
			return nil, ErrAlreadyInGame
		}
	}

	playerOrder := len(state.Players)
	if err := e.store.JoinGame(gameID, userID, playerOrder); err != nil {
		return nil, err
	}

	newPlayer := &Player{
		UserID:   userID,
		Username: username,
		Order:    playerOrder,
		IsReady:  false,
	}

	return &Event{
		Type:   "player_joined",
		GameID: gameID,
		Payload: PlayerJoinedPayload{
			Player: newPlayer,
		},
	}, nil
}

func (e *Engine) SetReady(gameID, userID int64, isReady bool) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusWaiting {
		return nil, ErrGameStarted
	}

	// Verify user is in game
	found := false
	for _, p := range state.Players {
		if p.UserID == userID {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrUserNotInGame
	}

	if err := e.store.UpdatePlayerReady(gameID, userID, isReady); err != nil {
		return nil, err
	}

	// Check if all players are ready and we have at least 2 players
	state, err = e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if len(state.Players) >= 2 {
		allReady := true
		for _, p := range state.Players {
			if !p.IsReady {
				allReady = false
				break
			}
		}

		if allReady {
			// Start the game
			if err := e.store.UpdateGameStatus(gameID, StatusInProgress); err != nil {
				return nil, err
			}

			// Set first player's turn
			firstPlayer := state.Players[0]
			if err := e.store.UpdateCurrentTurn(gameID, firstPlayer.UserID); err != nil {
				return nil, err
			}

			return &Event{
				Type:   "game_started",
				GameID: gameID,
				Payload: GameStartedPayload{
					CurrentPlayerID: firstPlayer.UserID,
				},
			}, nil
		}
	}

	return &Event{
		Type:   "player_ready",
		GameID: gameID,
		Payload: PlayerReadyPayload{
			UserID:  userID,
			IsReady: isReady,
		},
	}, nil
}

func (e *Engine) EndTurn(gameID, userID int64) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, ErrGameNotStarted
	}

	if state.CurrentPlayerID != userID {
		return nil, ErrNotYourTurn
	}

	// Find next player
	currentIdx := -1
	for i, p := range state.Players {
		if p.UserID == userID {
			currentIdx = i
			break
		}
	}

	nextIdx := (currentIdx + 1) % len(state.Players)
	nextPlayer := state.Players[nextIdx]

	if err := e.store.UpdateCurrentTurn(gameID, nextPlayer.UserID); err != nil {
		return nil, err
	}

	return &Event{
		Type:   "turn_changed",
		GameID: gameID,
		Payload: TurnChangedPayload{
			PreviousPlayerID: userID,
			CurrentPlayerID:  nextPlayer.UserID,
		},
	}, nil
}
