package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"log"
	"net/http"
	"time"
)

const (
	sessionDuration        = 7 * 24 * time.Hour // 7 days
	sessionCleanupInterval = 1 * time.Hour
	sessionIDByteLength    = 32
)

type Session struct {
	UserID    int64
	ExpiresAt time.Time
}

type SessionManager struct {
	db *sql.DB
}

func NewSessionManager(db *sql.DB) *SessionManager {
	sm := &SessionManager{
		db: db,
	}
	go sm.cleanupExpiredSessions()
	return sm
}

func (sm *SessionManager) CreateSession(userID int64) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(sessionDuration)

	_, err = sm.db.Exec(`
		INSERT INTO sessions (session_id, user_id, expires_at)
		VALUES (?, ?, ?)
	`, sessionID, userID, expiresAt)

	if err != nil {
		return "", err
	}

	return sessionID, nil
}

func (sm *SessionManager) GetUserID(sessionID string) (int64, bool) {
	var userID int64
	var expiresAt time.Time

	err := sm.db.QueryRow(`
		SELECT user_id, expires_at
		FROM sessions
		WHERE session_id = ?
	`, sessionID).Scan(&userID, &expiresAt)

	if err == sql.ErrNoRows {
		return 0, false
	}

	if err != nil {
		log.Printf("Error getting session: %v", err)
		return 0, false
	}

	// Check if session is expired
	if time.Now().After(expiresAt) {
		// Delete expired session
		sm.DeleteSession(sessionID)
		return 0, false
	}

	return userID, true
}

func (sm *SessionManager) DeleteSession(sessionID string) {
	_, err := sm.db.Exec(`
		DELETE FROM sessions
		WHERE session_id = ?
	`, sessionID)

	if err != nil {
		log.Printf("Error deleting session: %v", err)
	}
}

func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, sessionID string) {
	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Secure: true, // Enable in production with HTTPS
	}
	http.SetCookie(w, cookie)
}

func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
}

func GetSessionFromRequest(r *http.Request) string {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (sm *SessionManager) cleanupExpiredSessions() {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		result, err := sm.db.Exec(`
			DELETE FROM sessions
			WHERE expires_at < ?
		`, time.Now())

		if err != nil {
			log.Printf("Error cleaning up expired sessions: %v", err)
		} else {
			if rows, err := result.RowsAffected(); err == nil && rows > 0 {
				log.Printf("Cleaned up %d expired sessions", rows)
			}
		}
	}
}

func generateSessionID() (string, error) {
	bytes := make([]byte, sessionIDByteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
