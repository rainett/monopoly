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
	TransferAllPropertiesTx(tx *sql.Tx, gameID, fromUserID, toUserID int64) error
	CountActivePlayersTx(tx *sql.Tx, gameID int64) (int, error)
	GetActivePlayersTx(tx *sql.Tx, gameID int64) ([]*GamePlayer, error)
	// Jail operations
	SetPlayerInJailTx(tx *sql.Tx, gameID, userID int64, inJail bool, jailTurns int) error
	ReleaseFromJailTx(tx *sql.Tx, gameID, userID int64) error
	IncrementJailTurnsTx(tx *sql.Tx, gameID, userID int64) error
	// Mortgage operations
	GetPropertyTx(tx *sql.Tx, gameID int64, position int) (*GameProperty, error)
	SetPropertyMortgagedTx(tx *sql.Tx, gameID int64, position int, mortgaged bool) error
	// Improvement operations
	GetImprovementsTx(tx *sql.Tx, gameID int64, position int) (int, error)
	SetImprovementsTx(tx *sql.Tx, gameID int64, position int, count int) error
	GetAllImprovements(gameID int64) (map[int]int, error)
	GetTotalHousesHotelsTx(tx *sql.Tx, gameID int64) (houses int, hotels int, err error)
	// Card deck operations
	InitializeDecks(gameID int64, chanceOrder, communityOrder []int) error
	DrawCardTx(tx *sql.Tx, gameID int64, deckType string) (cardIndex int, err error)
	// Jail card operations
	GiveJailCardTx(tx *sql.Tx, gameID, userID int64, deckType string) error
	HasJailCard(gameID, userID int64) (bool, string, error)
	UseJailCardTx(tx *sql.Tx, gameID, userID int64) (string, error)
	// Trade operations
	CreateTrade(gameID, fromUserID, toUserID int64, offerJSON string) (int64, error)
	GetTrade(tradeID int64) (*GameTrade, error)
	GetPendingTrades(gameID int64) ([]*GameTrade, error)
	UpdateTradeStatus(tradeID int64, status string) error
	TransferPropertyTx(tx *sql.Tx, gameID int64, position int, newOwnerID int64) error
}

// GameTrade represents a trade in the database
type GameTrade struct {
	ID         int64
	GameID     int64
	FromUserID int64
	ToUserID   int64
	OfferJSON  string
	Status     string
	CreatedAt  string
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
	InJail        bool
	JailTurns     int
}

// GameProperty represents a property owned by a player
type GameProperty struct {
	GameID      int64
	Position    int
	OwnerID     int64
	IsMortgaged bool
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
		       gp.is_bankrupt, gp.has_rolled, gp.pending_action, gp.in_jail, gp.jail_turns
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
		var isReady, isCurrentTurn, hasPlayedTurn, isBankrupt, hasRolled, inJail int
		if err := rows.Scan(&player.GameID, &player.UserID, &player.Username,
			&player.PlayerOrder, &isReady, &isCurrentTurn, &hasPlayedTurn,
			&player.Money, &player.Position, &isBankrupt, &hasRolled,
			&player.PendingAction, &inJail, &player.JailTurns); err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		player.IsReady = intToBool(isReady)
		player.IsCurrentTurn = intToBool(isCurrentTurn)
		player.HasPlayedTurn = intToBool(hasPlayedTurn)
		player.IsBankrupt = intToBool(isBankrupt)
		player.HasRolled = intToBool(hasRolled)
		player.InJail = intToBool(inJail)
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
	var isReady, isCurrentTurn, hasPlayedTurn, isBankrupt, hasRolled, inJail int
	err := tx.QueryRow(`
		SELECT gp.game_id, gp.user_id, u.username, gp.player_order, gp.is_ready,
		       gp.is_current_turn, gp.has_played_turn, gp.money, gp.position,
		       gp.is_bankrupt, gp.has_rolled, gp.pending_action, gp.in_jail, gp.jail_turns
		FROM game_players gp
		JOIN users u ON gp.user_id = u.id
		WHERE gp.game_id = ? AND gp.user_id = ?
	`, gameID, userID).Scan(&player.GameID, &player.UserID, &player.Username,
		&player.PlayerOrder, &isReady, &isCurrentTurn, &hasPlayedTurn,
		&player.Money, &player.Position, &isBankrupt, &hasRolled,
		&player.PendingAction, &inJail, &player.JailTurns)

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
	player.InJail = intToBool(inJail)
	return player, nil
}

func (s *SQLiteGameStore) GetGameProperties(gameID int64) ([]*GameProperty, error) {
	rows, err := s.db.Query(
		"SELECT game_id, position, owner_id, is_mortgaged FROM game_properties WHERE game_id = ?",
		gameID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get game properties: %w", err)
	}
	defer rows.Close()

	var props []*GameProperty
	for rows.Next() {
		p := &GameProperty{}
		var isMortgaged int
		if err := rows.Scan(&p.GameID, &p.Position, &p.OwnerID, &isMortgaged); err != nil {
			return nil, fmt.Errorf("failed to scan property: %w", err)
		}
		p.IsMortgaged = intToBool(isMortgaged)
		props = append(props, p)
	}
	return props, rows.Err()
}

func (s *SQLiteGameStore) GetGamePropertiesTx(tx *sql.Tx, gameID int64) ([]*GameProperty, error) {
	rows, err := tx.Query(
		"SELECT game_id, position, owner_id, is_mortgaged FROM game_properties WHERE game_id = ?",
		gameID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get game properties: %w", err)
	}
	defer rows.Close()

	var props []*GameProperty
	for rows.Next() {
		p := &GameProperty{}
		var isMortgaged int
		if err := rows.Scan(&p.GameID, &p.Position, &p.OwnerID, &isMortgaged); err != nil {
			return nil, fmt.Errorf("failed to scan property: %w", err)
		}
		p.IsMortgaged = intToBool(isMortgaged)
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

func (s *SQLiteGameStore) TransferAllPropertiesTx(tx *sql.Tx, gameID, fromUserID, toUserID int64) error {
	_, err := tx.Exec(
		"UPDATE game_properties SET owner_id = ? WHERE game_id = ? AND owner_id = ?",
		toUserID, gameID, fromUserID,
	)
	if err != nil {
		return fmt.Errorf("failed to transfer properties: %w", err)
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
		       gp.is_bankrupt, gp.has_rolled, gp.pending_action, gp.in_jail, gp.jail_turns
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
		var isReady, isCurrentTurn, hasPlayedTurn, isBankrupt, hasRolled, inJail int
		if err := rows.Scan(&player.GameID, &player.UserID, &player.Username,
			&player.PlayerOrder, &isReady, &isCurrentTurn, &hasPlayedTurn,
			&player.Money, &player.Position, &isBankrupt, &hasRolled,
			&player.PendingAction, &inJail, &player.JailTurns); err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		player.IsReady = intToBool(isReady)
		player.IsCurrentTurn = intToBool(isCurrentTurn)
		player.HasPlayedTurn = intToBool(hasPlayedTurn)
		player.IsBankrupt = intToBool(isBankrupt)
		player.HasRolled = intToBool(hasRolled)
		player.InJail = intToBool(inJail)
		players = append(players, player)
	}
	return players, rows.Err()
}

// Jail operations

func (s *SQLiteGameStore) SetPlayerInJailTx(tx *sql.Tx, gameID, userID int64, inJail bool, jailTurns int) error {
	_, err := tx.Exec(
		"UPDATE game_players SET in_jail = ?, jail_turns = ? WHERE game_id = ? AND user_id = ?",
		boolToInt(inJail), jailTurns, gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to set player jail status: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) ReleaseFromJailTx(tx *sql.Tx, gameID, userID int64) error {
	_, err := tx.Exec(
		"UPDATE game_players SET in_jail = 0, jail_turns = 0 WHERE game_id = ? AND user_id = ?",
		gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to release player from jail: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) IncrementJailTurnsTx(tx *sql.Tx, gameID, userID int64) error {
	_, err := tx.Exec(
		"UPDATE game_players SET jail_turns = jail_turns + 1 WHERE game_id = ? AND user_id = ?",
		gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to increment jail turns: %w", err)
	}
	return nil
}

// Mortgage operations

func (s *SQLiteGameStore) GetPropertyTx(tx *sql.Tx, gameID int64, position int) (*GameProperty, error) {
	p := &GameProperty{}
	var isMortgaged int
	err := tx.QueryRow(
		"SELECT game_id, position, owner_id, is_mortgaged FROM game_properties WHERE game_id = ? AND position = ?",
		gameID, position,
	).Scan(&p.GameID, &p.Position, &p.OwnerID, &isMortgaged)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get property: %w", err)
	}
	p.IsMortgaged = intToBool(isMortgaged)
	return p, nil
}

func (s *SQLiteGameStore) SetPropertyMortgagedTx(tx *sql.Tx, gameID int64, position int, mortgaged bool) error {
	_, err := tx.Exec(
		"UPDATE game_properties SET is_mortgaged = ? WHERE game_id = ? AND position = ?",
		boolToInt(mortgaged), gameID, position,
	)
	if err != nil {
		return fmt.Errorf("failed to set property mortgaged status: %w", err)
	}
	return nil
}

// Improvement operations

func (s *SQLiteGameStore) GetImprovementsTx(tx *sql.Tx, gameID int64, position int) (int, error) {
	var count int
	err := tx.QueryRow(
		"SELECT count FROM game_improvements WHERE game_id = ? AND position = ?",
		gameID, position,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get improvements: %w", err)
	}
	return count, nil
}

func (s *SQLiteGameStore) SetImprovementsTx(tx *sql.Tx, gameID int64, position int, count int) error {
	_, err := tx.Exec(`
		INSERT INTO game_improvements (game_id, position, count) VALUES (?, ?, ?)
		ON CONFLICT(game_id, position) DO UPDATE SET count = ?
	`, gameID, position, count, count)
	if err != nil {
		return fmt.Errorf("failed to set improvements: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) GetAllImprovements(gameID int64) (map[int]int, error) {
	rows, err := s.db.Query(
		"SELECT position, count FROM game_improvements WHERE game_id = ?",
		gameID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get all improvements: %w", err)
	}
	defer rows.Close()

	improvements := make(map[int]int)
	for rows.Next() {
		var pos, count int
		if err := rows.Scan(&pos, &count); err != nil {
			return nil, fmt.Errorf("failed to scan improvement: %w", err)
		}
		improvements[pos] = count
	}
	return improvements, rows.Err()
}

func (s *SQLiteGameStore) GetTotalHousesHotelsTx(tx *sql.Tx, gameID int64) (houses int, hotels int, err error) {
	rows, err := tx.Query(
		"SELECT count FROM game_improvements WHERE game_id = ?",
		gameID,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get total improvements: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var count int
		if err := rows.Scan(&count); err != nil {
			return 0, 0, fmt.Errorf("failed to scan count: %w", err)
		}
		if count == 5 {
			hotels++
		} else {
			houses += count
		}
	}
	return houses, hotels, rows.Err()
}

// Card deck operations

func (s *SQLiteGameStore) InitializeDecks(gameID int64, chanceOrder, communityOrder []int) error {
	chanceJSON := intSliceToJSON(chanceOrder)
	communityJSON := intSliceToJSON(communityOrder)

	_, err := s.db.Exec(`
		INSERT INTO game_card_decks (game_id, deck_type, card_order, next_index)
		VALUES (?, 'chance', ?, 0), (?, 'community', ?, 0)
	`, gameID, chanceJSON, gameID, communityJSON)
	if err != nil {
		return fmt.Errorf("failed to initialize decks: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) DrawCardTx(tx *sql.Tx, gameID int64, deckType string) (int, error) {
	var cardOrder string
	var nextIndex int

	err := tx.QueryRow(
		"SELECT card_order, next_index FROM game_card_decks WHERE game_id = ? AND deck_type = ?",
		gameID, deckType,
	).Scan(&cardOrder, &nextIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to get deck: %w", err)
	}

	order := jsonToIntSlice(cardOrder)
	if len(order) == 0 {
		return 0, fmt.Errorf("empty deck")
	}

	cardIndex := order[nextIndex%len(order)]

	// Update next index
	newIndex := (nextIndex + 1) % len(order)
	_, err = tx.Exec(
		"UPDATE game_card_decks SET next_index = ? WHERE game_id = ? AND deck_type = ?",
		newIndex, gameID, deckType,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to update deck index: %w", err)
	}

	return cardIndex, nil
}

// Jail card operations

func (s *SQLiteGameStore) GiveJailCardTx(tx *sql.Tx, gameID, userID int64, deckType string) error {
	_, err := tx.Exec(
		"INSERT INTO player_jail_cards (game_id, user_id, deck_type) VALUES (?, ?, ?)",
		gameID, userID, deckType,
	)
	if err != nil {
		return fmt.Errorf("failed to give jail card: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) HasJailCard(gameID, userID int64) (bool, string, error) {
	var deckType string
	err := s.db.QueryRow(
		"SELECT deck_type FROM player_jail_cards WHERE game_id = ? AND user_id = ? LIMIT 1",
		gameID, userID,
	).Scan(&deckType)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("failed to check jail card: %w", err)
	}
	return true, deckType, nil
}

func (s *SQLiteGameStore) UseJailCardTx(tx *sql.Tx, gameID, userID int64) (string, error) {
	var deckType string
	err := tx.QueryRow(
		"SELECT deck_type FROM player_jail_cards WHERE game_id = ? AND user_id = ? LIMIT 1",
		gameID, userID,
	).Scan(&deckType)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no jail card to use")
	}
	if err != nil {
		return "", fmt.Errorf("failed to get jail card: %w", err)
	}

	_, err = tx.Exec(
		"DELETE FROM player_jail_cards WHERE game_id = ? AND user_id = ? AND deck_type = ?",
		gameID, userID, deckType,
	)
	if err != nil {
		return "", fmt.Errorf("failed to use jail card: %w", err)
	}

	return deckType, nil
}

// Helper functions for JSON conversion
func intSliceToJSON(slice []int) string {
	if len(slice) == 0 {
		return "[]"
	}
	result := "["
	for i, v := range slice {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("%d", v)
	}
	result += "]"
	return result
}

func jsonToIntSlice(s string) []int {
	if s == "" || s == "[]" {
		return nil
	}
	// Simple parsing for "[1,2,3]" format
	s = s[1 : len(s)-1] // Remove brackets
	if s == "" {
		return nil
	}
	parts := make([]int, 0)
	var num int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		} else if c == ',' {
			parts = append(parts, num)
			num = 0
		}
	}
	parts = append(parts, num)
	return parts
}

// Trade operations

func (s *SQLiteGameStore) CreateTrade(gameID, fromUserID, toUserID int64, offerJSON string) (int64, error) {
	result, err := s.db.Exec(`
		INSERT INTO game_trades (game_id, from_user_id, to_user_id, offer_json, status)
		VALUES (?, ?, ?, ?, 'pending')
	`, gameID, fromUserID, toUserID, offerJSON)
	if err != nil {
		return 0, fmt.Errorf("failed to create trade: %w", err)
	}
	return result.LastInsertId()
}

func (s *SQLiteGameStore) GetTrade(tradeID int64) (*GameTrade, error) {
	trade := &GameTrade{}
	err := s.db.QueryRow(`
		SELECT id, game_id, from_user_id, to_user_id, offer_json, status, created_at
		FROM game_trades WHERE id = ?
	`, tradeID).Scan(&trade.ID, &trade.GameID, &trade.FromUserID, &trade.ToUserID,
		&trade.OfferJSON, &trade.Status, &trade.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trade: %w", err)
	}
	return trade, nil
}

func (s *SQLiteGameStore) GetPendingTrades(gameID int64) ([]*GameTrade, error) {
	rows, err := s.db.Query(`
		SELECT id, game_id, from_user_id, to_user_id, offer_json, status, created_at
		FROM game_trades WHERE game_id = ? AND status = 'pending'
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending trades: %w", err)
	}
	defer rows.Close()

	var trades []*GameTrade
	for rows.Next() {
		trade := &GameTrade{}
		if err := rows.Scan(&trade.ID, &trade.GameID, &trade.FromUserID, &trade.ToUserID,
			&trade.OfferJSON, &trade.Status, &trade.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan trade: %w", err)
		}
		trades = append(trades, trade)
	}
	return trades, rows.Err()
}

func (s *SQLiteGameStore) UpdateTradeStatus(tradeID int64, status string) error {
	_, err := s.db.Exec(
		"UPDATE game_trades SET status = ? WHERE id = ?",
		status, tradeID,
	)
	if err != nil {
		return fmt.Errorf("failed to update trade status: %w", err)
	}
	return nil
}

func (s *SQLiteGameStore) TransferPropertyTx(tx *sql.Tx, gameID int64, position int, newOwnerID int64) error {
	_, err := tx.Exec(
		"UPDATE game_properties SET owner_id = ? WHERE game_id = ? AND position = ?",
		newOwnerID, gameID, position,
	)
	if err != nil {
		return fmt.Errorf("failed to transfer property: %w", err)
	}
	return nil
}
