package game

import (
	"monopoly/store"
)

const (
	minPlayersPerGame = 2
	maxPlayersPerGame = 8
)

type Lobby struct {
	store store.LobbyStore
}

func NewLobby(store store.LobbyStore) *Lobby {
	return &Lobby{store: store}
}

// CreateGame creates a new game and automatically joins the creator
func (l *Lobby) CreateGame(maxPlayers int, userID int64, username string) (*store.LobbyGameDTO, error) {
	if maxPlayers < minPlayersPerGame {
		maxPlayers = minPlayersPerGame
	}
	if maxPlayers > maxPlayersPerGame {
		maxPlayers = maxPlayersPerGame
	}

	gameID, err := l.store.CreateGame(maxPlayers)
	if err != nil {
		return nil, err
	}

	// Automatically join the creator
	err = l.store.JoinGame(gameID, userID, username)
	if err != nil {
		return nil, err
	}

	// Return the created game with the creator as a player
	return &store.LobbyGameDTO{
		ID:         gameID,
		Status:     "waiting",
		MaxPlayers: maxPlayers,
		Players: []store.LobbyPlayerDTO{
			{
				UserID:   userID,
				Username: username,
			},
		},
		IsJoined: true,
	}, nil
}

func (l *Lobby) ListGames(userID int64) ([]*store.LobbyGameDTO, error) {
	games, err := l.store.ListGames(userID)
	if err != nil {
		return nil, err
	}
	return games, nil
}

func (l *Lobby) JoinGame(gameID, userID int64, username string) error {
	return l.store.JoinGame(gameID, userID, username)
}

func (l *Lobby) LeaveGame(gameID, userID int64) error {
	return l.store.LeaveGame(gameID, userID)
}

func (l *Lobby) GetUserCurrentGame(userID int64) (*store.LobbyGameDTO, error) {
	return l.store.GetUserCurrentGame(userID)
}

func (l *Lobby) GetGameWithPlayers(gameID, userID int64) (*store.LobbyGameDTO, error) {
	return l.store.GetGameWithPlayers(gameID, userID)
}
