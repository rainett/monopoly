package store

import (
	"database/sql"
	"fmt"
)

// GameStore handles in-game operations (turns, ready status, game state)
type GameStore interface {
	GetGame(gameID int64) (*Game, error)
	GetGamePlayers(gameID int64) ([]*GamePlayer, error)
	JoinGame(gameID, userID int64, playerOrder int) error // Legacy method for WebSocket game view
	UpdatePlayerReady(gameID, userID int64, isReady bool) error
	UpdateGameStatus(gameID int64, status string) error
	UpdateCurrentTurn(gameID, userID int64) error
	GetCurrentTurnPlayer(gameID int64) (*GamePlayer, error)
	MarkPlayerTurnComplete(gameID, userID int64) error
	AllPlayersCompletedTurn(gameID int64) (bool, error)
}

// Game represents a game entity
type Game struct {
	ID         int64
	Status     string
	CreatedAt  string
	MaxPlayers int
}

// GamePlayer represents a player in a game
type GamePlayer struct {
	GameID        int64
	UserID        int64
	Username      string
	PlayerOrder   int
	IsReady       bool
	IsCurrentTurn bool
	HasPlayedTurn bool
}

// SQLiteGameStore implements GameStore for SQLite
type SQLiteGameStore struct {
	db *sql.DB
}

// NewGameStore creates a new GameStore
func NewGameStore(db *sql.DB) *SQLiteGameStore {
	return &SQLiteGameStore{db: db}
}

func (s *SQLiteGameStore) GetGame(gameID int64) (*Game, error) {
	game := &Game{}
	err := s.db.QueryRow(
		"SELECT id, status, created_at, max_players FROM games WHERE id = ?",
		gameID,
	).Scan(&game.ID, &game.Status, &game.CreatedAt, &game.MaxPlayers)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get game: %w", err)
	}
	return game, nil
}

func (s *SQLiteGameStore) GetGamePlayers(gameID int64) ([]*GamePlayer, error) {
	rows, err := s.db.Query(`
		SELECT gp.game_id, gp.user_id, u.username, gp.player_order, gp.is_ready, gp.is_current_turn, gp.has_played_turn
		FROM game_players gp
		JOIN users u ON gp.user_id = u.id
		WHERE gp.game_id = ?
		ORDER BY gp.player_order
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game players: %w", err)
	}
	defer rows.Close()

	var players []*GamePlayer
	for rows.Next() {
		player := &GamePlayer{}
		var isReady, isCurrentTurn, hasPlayedTurn int
		if err := rows.Scan(&player.GameID, &player.UserID, &player.Username, &player.PlayerOrder, &isReady, &isCurrentTurn, &hasPlayedTurn); err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		player.IsReady = intToBool(isReady)
		player.IsCurrentTurn = intToBool(isCurrentTurn)
		player.HasPlayedTurn = intToBool(hasPlayedTurn)
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate players: %w", err)
	}
	return players, nil
}

// JoinGame is a legacy method for WebSocket game view compatibility
// TODO: Remove once WebSocket game view is refactored to use lobby
func (s *SQLiteGameStore) JoinGame(gameID, userID int64, playerOrder int) error {
	_, err := s.db.Exec(
		"INSERT INTO game_players (game_id, user_id, player_order, is_ready, is_current_turn) VALUES (?, ?, ?, 0, 0)",
		gameID, userID, playerOrder,
	)
	if err != nil {
		return fmt.Errorf("failed to join game: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) UpdatePlayerReady(gameID, userID int64, isReady bool) error {
	_, err := s.db.Exec(
		"UPDATE game_players SET is_ready = ? WHERE game_id = ? AND user_id = ?",
		boolToInt(isReady), gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update player ready: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) UpdateGameStatus(gameID int64, status string) error {
	_, err := s.db.Exec(
		"UPDATE games SET status = ? WHERE id = ?",
		status, gameID,
	)
	if err != nil {
		return fmt.Errorf("failed to update game status: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) UpdateCurrentTurn(gameID, userID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear all current turns
	if _, err := tx.Exec("UPDATE game_players SET is_current_turn = 0 WHERE game_id = ?", gameID); err != nil {
		return fmt.Errorf("failed to clear current turns: %w", err)
	}

	// Set new current turn
	if _, err := tx.Exec(
		"UPDATE game_players SET is_current_turn = 1 WHERE game_id = ? AND user_id = ?",
		gameID, userID,
	); err != nil {
		return fmt.Errorf("failed to set current turn: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) GetCurrentTurnPlayer(gameID int64) (*GamePlayer, error) {
	player := &GamePlayer{}
	var isReady, isCurrentTurn int
	err := s.db.QueryRow(`
		SELECT gp.game_id, gp.user_id, u.username, gp.player_order, gp.is_ready, gp.is_current_turn
		FROM game_players gp
		JOIN users u ON gp.user_id = u.id
		WHERE gp.game_id = ? AND gp.is_current_turn = 1
	`, gameID).Scan(&player.GameID, &player.UserID, &player.Username, &player.PlayerOrder, &isReady, &isCurrentTurn)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get current turn player: %w", err)
	}
	player.IsReady = intToBool(isReady)
	player.IsCurrentTurn = intToBool(isCurrentTurn)
	return player, nil
}

func (s *SQLiteGameStore) MarkPlayerTurnComplete(gameID, userID int64) error {
	_, err := s.db.Exec(
		"UPDATE game_players SET has_played_turn = 1 WHERE game_id = ? AND user_id = ?",
		gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to mark player turn complete: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) AllPlayersCompletedTurn(gameID int64) (bool, error) {
	var totalPlayers, playersCompleted int
	err := s.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(has_played_turn) as completed
		FROM game_players
		WHERE game_id = ?
	`, gameID).Scan(&totalPlayers, &playersCompleted)

	if err != nil {
		return false, fmt.Errorf("failed to check if all players completed turn: %w", err)
	}

	return totalPlayers > 0 && totalPlayers == playersCompleted, nil
}
