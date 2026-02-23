package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store interface {
	CreateUser(username, passwordHash string) (int64, error)
	GetUserByUsername(username string) (*User, error)
	GetUserByID(userID int64) (*User, error)
	CreateGame(maxPlayers int) (int64, error)
	JoinGame(gameID, userID int64, playerOrder int) error
	ListGames() ([]*Game, error)
	GetGame(gameID int64) (*Game, error)
	GetGamePlayers(gameID int64) ([]*GamePlayer, error)
	UpdatePlayerReady(gameID, userID int64, isReady bool) error
	UpdateGameStatus(gameID int64, status string) error
	UpdateCurrentTurn(gameID, userID int64) error
	GetCurrentTurnPlayer(gameID int64) (*GamePlayer, error)
	Close() error
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    string
}

type Game struct {
	ID         int64
	Status     string
	CreatedAt  string
	MaxPlayers int
}

type GamePlayer struct {
	GameID        int64
	UserID        int64
	Username      string
	PlayerOrder   int
	IsReady       bool
	IsCurrentTurn bool
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) CreateUser(username, passwordHash string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO users (username, password_hash) VALUES (?, ?)",
		username, passwordHash,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create user: %w", err)
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) GetUserByUsername(username string) (*User, error) {
	user := &User{}
	err := s.db.QueryRow(
		"SELECT id, username, password_hash, created_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

func (s *SQLiteStore) GetUserByID(userID int64) (*User, error) {
	user := &User{}
	err := s.db.QueryRow(
		"SELECT id, username, password_hash, created_at FROM users WHERE id = ?",
		userID,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

func (s *SQLiteStore) CreateGame(maxPlayers int) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO games (status, max_players) VALUES ('waiting', ?)",
		maxPlayers,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create game: %w", err)
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) JoinGame(gameID, userID int64, playerOrder int) error {
	_, err := s.db.Exec(
		"INSERT INTO game_players (game_id, user_id, player_order) VALUES (?, ?, ?)",
		gameID, userID, playerOrder,
	)
	if err != nil {
		return fmt.Errorf("failed to join game: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListGames() ([]*Game, error) {
	rows, err := s.db.Query(
		"SELECT id, status, created_at, max_players FROM games WHERE status != 'finished' ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list games: %w", err)
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		game := &Game{}
		if err := rows.Scan(&game.ID, &game.Status, &game.CreatedAt, &game.MaxPlayers); err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		games = append(games, game)
	}
	return games, nil
}

func (s *SQLiteStore) GetGame(gameID int64) (*Game, error) {
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

func (s *SQLiteStore) GetGamePlayers(gameID int64) ([]*GamePlayer, error) {
	rows, err := s.db.Query(`
		SELECT gp.game_id, gp.user_id, u.username, gp.player_order, gp.is_ready, gp.is_current_turn
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
		var isReady, isCurrentTurn int
		if err := rows.Scan(&player.GameID, &player.UserID, &player.Username, &player.PlayerOrder, &isReady, &isCurrentTurn); err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		player.IsReady = isReady == 1
		player.IsCurrentTurn = isCurrentTurn == 1
		players = append(players, player)
	}
	return players, nil
}

func (s *SQLiteStore) UpdatePlayerReady(gameID, userID int64, isReady bool) error {
	readyVal := 0
	if isReady {
		readyVal = 1
	}
	_, err := s.db.Exec(
		"UPDATE game_players SET is_ready = ? WHERE game_id = ? AND user_id = ?",
		readyVal, gameID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update player ready: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateGameStatus(gameID int64, status string) error {
	_, err := s.db.Exec(
		"UPDATE games SET status = ? WHERE id = ?",
		status, gameID,
	)
	if err != nil {
		return fmt.Errorf("failed to update game status: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateCurrentTurn(gameID, userID int64) error {
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

func (s *SQLiteStore) GetCurrentTurnPlayer(gameID int64) (*GamePlayer, error) {
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
	player.IsReady = isReady == 1
	player.IsCurrentTurn = isCurrentTurn == 1
	return player, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
