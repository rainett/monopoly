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
	// Transaction support
	BeginTx() (*sql.Tx, error)
	CommitTx(tx *sql.Tx) error
	RollbackTx(tx *sql.Tx) error
	// Transaction-aware operations
	UpdatePlayerReadyTx(tx *sql.Tx, gameID, userID int64, isReady bool) error
	UpdateGameStatusTx(tx *sql.Tx, gameID int64, status string) error
	UpdateCurrentTurnTx(tx *sql.Tx, gameID, userID int64) error
	MarkPlayerTurnCompleteTx(tx *sql.Tx, gameID, userID int64) error
	// Game mechanics operations
	UpdatePlayerPositionTx(tx *sql.Tx, gameID, userID int64, position int) error
	UpdatePlayerMoneyTx(tx *sql.Tx, gameID, userID int64, money int) error
	SetPlayerBankruptTx(tx *sql.Tx, gameID, userID int64) error
	SetPlayerHasRolledTx(tx *sql.Tx, gameID, userID int64, hasRolled bool) error
	SetPlayerPendingActionTx(tx *sql.Tx, gameID, userID int64, action string) error
	ResetPlayerTurnStateTx(tx *sql.Tx, gameID, userID int64) error
	GetPlayerTx(tx *sql.Tx, gameID, userID int64) (*GamePlayer, error)
	GetGameProperties(gameID int64) ([]*GameProperty, error)
	GetGamePropertiesTx(tx *sql.Tx, gameID int64) ([]*GameProperty, error)
	GetPropertyOwnerTx(tx *sql.Tx, gameID int64, position int) (int64, error)
	GetPlayerPropertiesTx(tx *sql.Tx, gameID, userID int64) ([]int, error)
	InsertPropertyTx(tx *sql.Tx, gameID int64, position int, ownerID int64) error
	DeletePlayerPropertiesTx(tx *sql.Tx, gameID, userID int64) error
	CountActivePlayersTx(tx *sql.Tx, gameID int64) (int, error)
	GetActivePlayersTx(tx *sql.Tx, gameID int64) ([]*GamePlayer, error)
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
	Money         int
	Position      int
	IsBankrupt    bool
	HasRolled     bool
	PendingAction string
}

// GameProperty represents a property owned by a player
type GameProperty struct {
	GameID   int64
	Position int
	OwnerID  int64
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
		SELECT gp.game_id, gp.user_id, u.username, gp.player_order, gp.is_ready,
		       gp.is_current_turn, gp.has_played_turn, gp.money, gp.position,
		       gp.is_bankrupt, gp.has_rolled, gp.pending_action
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
		var isReady, isCurrentTurn, hasPlayedTurn, isBankrupt, hasRolled int
		if err := rows.Scan(&player.GameID, &player.UserID, &player.Username,
			&player.PlayerOrder, &isReady, &isCurrentTurn, &hasPlayedTurn,
			&player.Money, &player.Position, &isBankrupt, &hasRolled,
			&player.PendingAction); err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		player.IsReady = intToBool(isReady)
		player.IsCurrentTurn = intToBool(isCurrentTurn)
		player.HasPlayedTurn = intToBool(hasPlayedTurn)
		player.IsBankrupt = intToBool(isBankrupt)
		player.HasRolled = intToBool(hasRolled)
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate players: %w", err)
	}
	return players, nil
}

// JoinGame is a legacy method for WebSocket game view compatibility
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

	if _, err := tx.Exec("UPDATE game_players SET is_current_turn = 0 WHERE game_id = ?", gameID); err != nil {
		return fmt.Errorf("failed to clear current turns: %w", err)
	}

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

// Transaction support methods

func (s *SQLiteGameStore) BeginTx() (*sql.Tx, error) {
	return s.db.Begin()
}

func (s *SQLiteGameStore) CommitTx(tx *sql.Tx) error {
	return tx.Commit()
}

func (s *SQLiteGameStore) RollbackTx(tx *sql.Tx) error {
	return tx.Rollback()
}

func (s *SQLiteGameStore) UpdatePlayerReadyTx(tx *sql.Tx, gameID, userID int64, isReady bool) error {
	_, err := tx.Exec(
		"UPDATE game_players SET is_ready = ? WHERE game_id = ? AND user_id = ?",
		boolToInt(isReady), gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update player ready: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) UpdateGameStatusTx(tx *sql.Tx, gameID int64, status string) error {
	_, err := tx.Exec(
		"UPDATE games SET status = ? WHERE id = ?",
		status, gameID,
	)
	if err != nil {
		return fmt.Errorf("failed to update game status: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) UpdateCurrentTurnTx(tx *sql.Tx, gameID, userID int64) error {
	if _, err := tx.Exec("UPDATE game_players SET is_current_turn = 0 WHERE game_id = ?", gameID); err != nil {
		return fmt.Errorf("failed to clear current turns: %w", err)
	}

	if _, err := tx.Exec(
		"UPDATE game_players SET is_current_turn = 1 WHERE game_id = ? AND user_id = ?",
		gameID, userID,
	); err != nil {
		return fmt.Errorf("failed to set current turn: %w", err)
	}

	return nil
}

func (s *SQLiteGameStore) MarkPlayerTurnCompleteTx(tx *sql.Tx, gameID, userID int64) error {
	_, err := tx.Exec(
		"UPDATE game_players SET has_played_turn = 1 WHERE game_id = ? AND user_id = ?",
		gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to mark player turn complete: %w", err)
	}
	return nil
}

// Game mechanics store methods

func (s *SQLiteGameStore) UpdatePlayerPositionTx(tx *sql.Tx, gameID, userID int64, position int) error {
	_, err := tx.Exec(
		"UPDATE game_players SET position = ? WHERE game_id = ? AND user_id = ?",
		position, gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update player position: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) UpdatePlayerMoneyTx(tx *sql.Tx, gameID, userID int64, money int) error {
	_, err := tx.Exec(
		"UPDATE game_players SET money = ? WHERE game_id = ? AND user_id = ?",
		money, gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update player money: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) SetPlayerBankruptTx(tx *sql.Tx, gameID, userID int64) error {
	_, err := tx.Exec(
		"UPDATE game_players SET is_bankrupt = 1, money = 0 WHERE game_id = ? AND user_id = ?",
		gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to set player bankrupt: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) SetPlayerHasRolledTx(tx *sql.Tx, gameID, userID int64, hasRolled bool) error {
	_, err := tx.Exec(
		"UPDATE game_players SET has_rolled = ? WHERE game_id = ? AND user_id = ?",
		boolToInt(hasRolled), gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to set player has_rolled: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) SetPlayerPendingActionTx(tx *sql.Tx, gameID, userID int64, action string) error {
	_, err := tx.Exec(
		"UPDATE game_players SET pending_action = ? WHERE game_id = ? AND user_id = ?",
		action, gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to set player pending_action: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) ResetPlayerTurnStateTx(tx *sql.Tx, gameID, userID int64) error {
	_, err := tx.Exec(
		"UPDATE game_players SET has_rolled = 0, pending_action = '' WHERE game_id = ? AND user_id = ?",
		gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to reset player turn state: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) GetPlayerTx(tx *sql.Tx, gameID, userID int64) (*GamePlayer, error) {
	player := &GamePlayer{}
	var isReady, isCurrentTurn, hasPlayedTurn, isBankrupt, hasRolled int
	err := tx.QueryRow(`
		SELECT gp.game_id, gp.user_id, u.username, gp.player_order, gp.is_ready,
		       gp.is_current_turn, gp.has_played_turn, gp.money, gp.position,
		       gp.is_bankrupt, gp.has_rolled, gp.pending_action
		FROM game_players gp
		JOIN users u ON gp.user_id = u.id
		WHERE gp.game_id = ? AND gp.user_id = ?
	`, gameID, userID).Scan(&player.GameID, &player.UserID, &player.Username,
		&player.PlayerOrder, &isReady, &isCurrentTurn, &hasPlayedTurn,
		&player.Money, &player.Position, &isBankrupt, &hasRolled,
		&player.PendingAction)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get player: %w", err)
	}
	player.IsReady = intToBool(isReady)
	player.IsCurrentTurn = intToBool(isCurrentTurn)
	player.HasPlayedTurn = intToBool(hasPlayedTurn)
	player.IsBankrupt = intToBool(isBankrupt)
	player.HasRolled = intToBool(hasRolled)
	return player, nil
}

func (s *SQLiteGameStore) GetGameProperties(gameID int64) ([]*GameProperty, error) {
	rows, err := s.db.Query(
		"SELECT game_id, position, owner_id FROM game_properties WHERE game_id = ?",
		gameID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get game properties: %w", err)
	}
	defer rows.Close()

	var props []*GameProperty
	for rows.Next() {
		p := &GameProperty{}
		if err := rows.Scan(&p.GameID, &p.Position, &p.OwnerID); err != nil {
			return nil, fmt.Errorf("failed to scan property: %w", err)
		}
		props = append(props, p)
	}
	return props, rows.Err()
}

func (s *SQLiteGameStore) GetGamePropertiesTx(tx *sql.Tx, gameID int64) ([]*GameProperty, error) {
	rows, err := tx.Query(
		"SELECT game_id, position, owner_id FROM game_properties WHERE game_id = ?",
		gameID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get game properties: %w", err)
	}
	defer rows.Close()

	var props []*GameProperty
	for rows.Next() {
		p := &GameProperty{}
		if err := rows.Scan(&p.GameID, &p.Position, &p.OwnerID); err != nil {
			return nil, fmt.Errorf("failed to scan property: %w", err)
		}
		props = append(props, p)
	}
	return props, rows.Err()
}

func (s *SQLiteGameStore) GetPropertyOwnerTx(tx *sql.Tx, gameID int64, position int) (int64, error) {
	var ownerID int64
	err := tx.QueryRow(
		"SELECT owner_id FROM game_properties WHERE game_id = ? AND position = ?",
		gameID, position,
	).Scan(&ownerID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get property owner: %w", err)
	}
	return ownerID, nil
}

func (s *SQLiteGameStore) GetPlayerPropertiesTx(tx *sql.Tx, gameID, userID int64) ([]int, error) {
	rows, err := tx.Query(
		"SELECT position FROM game_properties WHERE game_id = ? AND owner_id = ?",
		gameID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get player properties: %w", err)
	}
	defer rows.Close()

	var positions []int
	for rows.Next() {
		var pos int
		if err := rows.Scan(&pos); err != nil {
			return nil, fmt.Errorf("failed to scan position: %w", err)
		}
		positions = append(positions, pos)
	}
	return positions, rows.Err()
}

func (s *SQLiteGameStore) InsertPropertyTx(tx *sql.Tx, gameID int64, position int, ownerID int64) error {
	_, err := tx.Exec(
		"INSERT INTO game_properties (game_id, position, owner_id) VALUES (?, ?, ?)",
		gameID, position, ownerID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert property: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) DeletePlayerPropertiesTx(tx *sql.Tx, gameID, userID int64) error {
	_, err := tx.Exec(
		"DELETE FROM game_properties WHERE game_id = ? AND owner_id = ?",
		gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete player properties: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) CountActivePlayersTx(tx *sql.Tx, gameID int64) (int, error) {
	var count int
	err := tx.QueryRow(
		"SELECT COUNT(*) FROM game_players WHERE game_id = ? AND is_bankrupt = 0",
		gameID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active players: %w", err)
	}
	return count, nil
}

func (s *SQLiteGameStore) GetActivePlayersTx(tx *sql.Tx, gameID int64) ([]*GamePlayer, error) {
	rows, err := tx.Query(`
		SELECT gp.game_id, gp.user_id, u.username, gp.player_order, gp.is_ready,
		       gp.is_current_turn, gp.has_played_turn, gp.money, gp.position,
		       gp.is_bankrupt, gp.has_rolled, gp.pending_action
		FROM game_players gp
		JOIN users u ON gp.user_id = u.id
		WHERE gp.game_id = ? AND gp.is_bankrupt = 0
		ORDER BY gp.player_order
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active players: %w", err)
	}
	defer rows.Close()

	var players []*GamePlayer
	for rows.Next() {
		player := &GamePlayer{}
		var isReady, isCurrentTurn, hasPlayedTurn, isBankrupt, hasRolled int
		if err := rows.Scan(&player.GameID, &player.UserID, &player.Username,
			&player.PlayerOrder, &isReady, &isCurrentTurn, &hasPlayedTurn,
			&player.Money, &player.Position, &isBankrupt, &hasRolled,
			&player.PendingAction); err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		player.IsReady = intToBool(isReady)
		player.IsCurrentTurn = intToBool(isCurrentTurn)
		player.HasPlayedTurn = intToBool(hasPlayedTurn)
		player.IsBankrupt = intToBool(isBankrupt)
		player.HasRolled = intToBool(hasRolled)
		players = append(players, player)
	}
	return players, rows.Err()
}
