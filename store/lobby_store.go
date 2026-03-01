package store

import (
	"database/sql"
	"errors"
	"fmt"
)

type LobbyStore interface {
	ListGames(userID int64) ([]*LobbyGameDTO, error)
	CreateGame(maxPlayers int) (int64, error)
	JoinGame(gameID, userID int64, username string) error
	LeaveGame(gameID, userID int64) error
	GetUserCurrentGame(userID int64) (*LobbyGameDTO, error)
	IsUserInGame(userID int64) (bool, int64, error)
}

// LobbyGameDTO is the simplified DTO for lobby game list
type LobbyGameDTO struct {
	ID         int64            `json:"id"`
	Status     string           `json:"status"`
	MaxPlayers int              `json:"maxPlayers"`
	Players    []LobbyPlayerDTO `json:"players"`
	IsJoined   bool             `json:"isJoined"` // true if current user is in this game
}

// LobbyPlayerDTO contains minimal player info for lobby
type LobbyPlayerDTO struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}

type SQLiteLobbyStore struct {
	db *sql.DB
}

func NewSQLiteLobbyStore(db *sql.DB) *SQLiteLobbyStore {
	return &SQLiteLobbyStore{db: db}
}

func (s *SQLiteLobbyStore) ListGames(userID int64) ([]*LobbyGameDTO, error) {
	// Get all active games
	rows, err := s.db.Query(`
		SELECT id, status, max_players
		FROM games
		WHERE status != 'finished'
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, wrapDBError("list games", err)
	}
	defer rows.Close()

	// Build games map for efficient lookup
	gamesMap := make(map[int64]*LobbyGameDTO)
	var gameIDs []int64
	for rows.Next() {
		game := &LobbyGameDTO{Players: []LobbyPlayerDTO{}}
		if err := rows.Scan(&game.ID, &game.Status, &game.MaxPlayers); err != nil {
			return nil, wrapDBError("scan game row", err)
		}
		gamesMap[game.ID] = game
		gameIDs = append(gameIDs, game.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate game rows: %w", err)
	}

	// If no games, return empty list
	if len(gameIDs) == 0 {
		return []*LobbyGameDTO{}, nil
	}

	// Get all players for all games in a single query
	playerRows, err := s.db.Query(`
		SELECT gp.game_id, gp.user_id, u.username
		FROM game_players gp
		JOIN users u ON gp.user_id = u.id
		WHERE gp.game_id IN (SELECT id FROM games WHERE status != 'finished')
		ORDER BY gp.game_id, gp.player_order
	`)
	if err != nil {
		return nil, wrapDBError("query game players", err)
	}
	defer playerRows.Close()

	// Group players by game
	for playerRows.Next() {
		var gameID int64
		var player LobbyPlayerDTO
		if err := playerRows.Scan(&gameID, &player.UserID, &player.Username); err != nil {
			return nil, wrapDBError("scan player row", err)
		}

		if game, exists := gamesMap[gameID]; exists {
			game.Players = append(game.Players, player)
			if player.UserID == userID {
				game.IsJoined = true
			}
		}
	}

	if err := playerRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate player rows: %w", err)
	}

	// Convert map to ordered slice (maintaining DESC order from gameIDs)
	games := make([]*LobbyGameDTO, 0, len(gameIDs))
	for _, gameID := range gameIDs {
		games = append(games, gamesMap[gameID])
	}

	return games, nil
}

func (s *SQLiteLobbyStore) CreateGame(maxPlayers int) (int64, error) {
	result, err := s.db.Exec(`INSERT INTO games (status, max_players) VALUES ('waiting', ?)`, maxPlayers)
	if err != nil {
		return 0, fmt.Errorf("failed to create game: %w", err)
	}
	return result.LastInsertId()
}

func (s *SQLiteLobbyStore) JoinGame(gameID, userID int64, username string) error {
	// Check if user is already in a game
	isInGame, existingGameID, err := s.IsUserInGame(userID)
	if err != nil {
		return fmt.Errorf("failed to check user game status: %w", err)
	}
	if isInGame && existingGameID != gameID {
		return errors.New("user already in another game")
	}
	if isInGame && existingGameID == gameID {
		return nil // Already in this game, no-op
	}

	// Check if game exists and is waiting
	var status string
	var maxPlayers, currentPlayers int
	err = s.db.QueryRow(`
		SELECT g.status, g.max_players, COUNT(gp.user_id) as player_count
		FROM games g
		LEFT JOIN game_players gp ON g.id = gp.game_id
		WHERE g.id = ?
		GROUP BY g.id
	`, gameID).Scan(&status, &maxPlayers, &currentPlayers)
	if err == sql.ErrNoRows {
		return errors.New("game not found")
	}
	if err != nil {
		return fmt.Errorf("failed to query game: %w", err)
	}

	if status != "waiting" {
		return errors.New("game already started")
	}
	if currentPlayers >= maxPlayers {
		return errors.New("game is full")
	}

	// Get next player order
	var nextOrder int
	err = s.db.QueryRow(`
		SELECT COALESCE(MAX(player_order), 0) + 1
		FROM game_players
		WHERE game_id = ?
	`, gameID).Scan(&nextOrder)
	if err != nil {
		return fmt.Errorf("failed to get next player order: %w", err)
	}

	// Add player to game
	_, err = s.db.Exec(`
		INSERT INTO game_players (game_id, user_id, player_order, is_ready, is_current_turn)
		VALUES (?, ?, ?, 0, 0)
	`, gameID, userID, nextOrder)
	if err != nil {
		return fmt.Errorf("failed to add player to game: %w", err)
	}

	return nil
}

func (s *SQLiteLobbyStore) LeaveGame(gameID, userID int64) error {
	// Remove player from game
	result, err := s.db.Exec(`
		DELETE FROM game_players
		WHERE game_id = ? AND user_id = ?
	`, gameID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove player from game: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return errors.New("user not in game")
	}

	// Check if game is now empty
	var playerCount int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM game_players WHERE game_id = ?
	`, gameID).Scan(&playerCount)
	if err != nil {
		return fmt.Errorf("failed to count players: %w", err)
	}

	// If game is empty and still waiting, delete it
	if playerCount == 0 {
		_, err = s.db.Exec(`
			DELETE FROM games WHERE id = ? AND status = 'waiting'
		`, gameID)
		if err != nil {
			return fmt.Errorf("failed to delete empty game: %w", err)
		}
	}

	return nil
}

func (s *SQLiteLobbyStore) GetUserCurrentGame(userID int64) (*LobbyGameDTO, error) {
	// Get game details and all players in a single query
	rows, err := s.db.Query(`
		SELECT g.id, g.status, g.max_players, gp.user_id, u.username
		FROM game_players gp_user
		JOIN games g ON gp_user.game_id = g.id
		JOIN game_players gp ON gp.game_id = g.id
		JOIN users u ON gp.user_id = u.id
		WHERE gp_user.user_id = ?
		ORDER BY gp.player_order
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user game: %w", err)
	}
	defer rows.Close()

	var game *LobbyGameDTO
	for rows.Next() {
		var gameID int64
		var status string
		var maxPlayers int
		var player LobbyPlayerDTO

		if err := rows.Scan(&gameID, &status, &maxPlayers, &player.UserID, &player.Username); err != nil {
			return nil, fmt.Errorf("failed to scan game and player: %w", err)
		}

		// Initialize game on first row
		if game == nil {
			game = &LobbyGameDTO{
				ID:         gameID,
				Status:     status,
				MaxPlayers: maxPlayers,
				IsJoined:   true,
				Players:    []LobbyPlayerDTO{},
			}
		}

		game.Players = append(game.Players, player)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate game rows: %w", err)
	}

	return game, nil
}

func (s *SQLiteLobbyStore) IsUserInGame(userID int64) (bool, int64, error) {
	var gameID int64
	err := s.db.QueryRow(`
		SELECT game_id
		FROM game_players
		WHERE user_id = ?
		LIMIT 1
	`, userID).Scan(&gameID)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to check user game: %w", err)
	}
	return true, gameID, nil
}
