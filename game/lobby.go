package game

import (
	"monopoly/store"
)

type Lobby struct {
	store store.Store
}

func NewLobby(store store.Store) *Lobby {
	return &Lobby{store: store}
}

func (l *Lobby) CreateGame(maxPlayers int) (int64, error) {
	if maxPlayers < 2 {
		maxPlayers = 2
	}
	if maxPlayers > 8 {
		maxPlayers = 8
	}

	gameID, err := l.store.CreateGame(maxPlayers)
	if err != nil {
		return 0, err
	}

	return gameID, nil
}

func (l *Lobby) ListGames() ([]*GameState, error) {
	games, err := l.store.ListGames()
	if err != nil {
		return nil, err
	}

	gameStates := make([]*GameState, 0, len(games))
	for _, game := range games {
		players, err := l.store.GetGamePlayers(game.ID)
		if err != nil {
			return nil, err
		}

		gamePlayers := make([]*Player, len(players))
		for i, p := range players {
			gamePlayers[i] = &Player{
				UserID:        p.UserID,
				Username:      p.Username,
				Order:         p.PlayerOrder,
				IsReady:       p.IsReady,
				IsCurrentTurn: p.IsCurrentTurn,
			}
		}

		gameStates = append(gameStates, &GameState{
			ID:         game.ID,
			Status:     game.Status,
			Players:    gamePlayers,
			MaxPlayers: game.MaxPlayers,
		})
	}

	return gameStates, nil
}

func (l *Lobby) GetGame(gameID int64) (*GameState, error) {
	game, err := l.store.GetGame(gameID)
	if err != nil {
		return nil, err
	}
	if game == nil {
		return nil, nil
	}

	players, err := l.store.GetGamePlayers(gameID)
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
