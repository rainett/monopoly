package store

import (
	"database/sql"
	"errors"
	"fmt"
)

type AuthStore interface {
	GetUserByUsername(username string) (*User, error)
	GetUserByID(userID int64) (*User, error)
	CreateUser(username, passwordHash string) (int64, error)
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    string
}

type SQLiteAuthStore struct {
	db *sql.DB
}

func NewAuthStore(db *sql.DB) *SQLiteAuthStore {
	return &SQLiteAuthStore{db: db}
}

func (s *SQLiteAuthStore) GetUserByUsername(username string) (*User, error) {
	user := &User{}
	err := s.db.QueryRow(`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`,
		username).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}
	return user, nil
}

func (s *SQLiteAuthStore) GetUserByID(userID int64) (*User, error) {
	user := &User{}
	err := s.db.QueryRow(`SELECT id, username, password_hash, created_at FROM users WHERE id = ?`,
		userID).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}
	return user, nil
}

func (s *SQLiteAuthStore) CreateUser(username, passwordHash string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO users (username, password_hash) VALUES (?, ?)",
		username, passwordHash,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create user: %w", err)
	}
	return result.LastInsertId()
}
