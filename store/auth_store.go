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
	// Friends
	SearchUsers(query string, excludeUserID int64, limit int) ([]*User, error)
	SendFriendRequest(fromUserID, toUserID int64) error
	AcceptFriendRequest(userID, friendID int64) error
	DeclineFriendRequest(userID, friendID int64) error
	GetFriends(userID int64) ([]*User, error)
	GetPendingRequests(userID int64) ([]*FriendRequest, error)
	AreFriends(userID1, userID2 int64) (bool, error)
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    string
}

type FriendRequest struct {
	FromUserID   int64
	FromUsername string
	ToUserID     int64
	ToUsername   string
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

// Friends methods

func (s *SQLiteAuthStore) SearchUsers(query string, excludeUserID int64, limit int) ([]*User, error) {
	rows, err := s.db.Query(`
		SELECT id, username, created_at FROM users
		WHERE username LIKE ? AND id != ?
		ORDER BY username
		LIMIT ?
	`, "%"+query+"%", excludeUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		if err := rows.Scan(&user.ID, &user.Username, &user.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *SQLiteAuthStore) SendFriendRequest(fromUserID, toUserID int64) error {
	// Check if friendship already exists
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM friendships
		WHERE (user_id_1 = ? AND user_id_2 = ?) OR (user_id_1 = ? AND user_id_2 = ?)
	`, fromUserID, toUserID, toUserID, fromUserID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check existing friendship: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("friendship or request already exists")
	}

	_, err = s.db.Exec(`
		INSERT INTO friendships (user_id_1, user_id_2, status) VALUES (?, ?, 'pending')
	`, fromUserID, toUserID)
	if err != nil {
		return fmt.Errorf("failed to send friend request: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) AcceptFriendRequest(userID, friendID int64) error {
	result, err := s.db.Exec(`
		UPDATE friendships SET status = 'accepted'
		WHERE user_id_1 = ? AND user_id_2 = ? AND status = 'pending'
	`, friendID, userID)
	if err != nil {
		return fmt.Errorf("failed to accept friend request: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no pending friend request found")
	}
	return nil
}

func (s *SQLiteAuthStore) DeclineFriendRequest(userID, friendID int64) error {
	result, err := s.db.Exec(`
		DELETE FROM friendships
		WHERE user_id_1 = ? AND user_id_2 = ? AND status = 'pending'
	`, friendID, userID)
	if err != nil {
		return fmt.Errorf("failed to decline friend request: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no pending friend request found")
	}
	return nil
}

func (s *SQLiteAuthStore) GetFriends(userID int64) ([]*User, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.username, u.created_at
		FROM users u
		JOIN friendships f ON (
			(f.user_id_1 = ? AND f.user_id_2 = u.id) OR
			(f.user_id_2 = ? AND f.user_id_1 = u.id)
		)
		WHERE f.status = 'accepted'
		ORDER BY u.username
	`, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get friends: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		if err := rows.Scan(&user.ID, &user.Username, &user.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan friend: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *SQLiteAuthStore) GetPendingRequests(userID int64) ([]*FriendRequest, error) {
	rows, err := s.db.Query(`
		SELECT f.user_id_1, u1.username, f.user_id_2, u2.username, f.created_at
		FROM friendships f
		JOIN users u1 ON f.user_id_1 = u1.id
		JOIN users u2 ON f.user_id_2 = u2.id
		WHERE f.user_id_2 = ? AND f.status = 'pending'
		ORDER BY f.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending requests: %w", err)
	}
	defer rows.Close()

	var requests []*FriendRequest
	for rows.Next() {
		req := &FriendRequest{}
		if err := rows.Scan(&req.FromUserID, &req.FromUsername, &req.ToUserID, &req.ToUsername, &req.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan request: %w", err)
		}
		requests = append(requests, req)
	}
	return requests, rows.Err()
}

func (s *SQLiteAuthStore) AreFriends(userID1, userID2 int64) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM friendships
		WHERE ((user_id_1 = ? AND user_id_2 = ?) OR (user_id_1 = ? AND user_id_2 = ?))
		AND status = 'accepted'
	`, userID1, userID2, userID2, userID1).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check friendship: %w", err)
	}
	return count > 0, nil
}
