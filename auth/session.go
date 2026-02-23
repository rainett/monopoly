package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
)

type SessionManager struct {
	sessions map[string]int64
	mu       sync.RWMutex
	secret   string
}

func NewSessionManager(secret string) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]int64),
		secret:   secret,
	}
}

func (sm *SessionManager) CreateSession(userID int64) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", err
	}

	sm.mu.Lock()
	sm.sessions[sessionID] = userID
	sm.mu.Unlock()

	return sessionID, nil
}

func (sm *SessionManager) GetUserID(sessionID string) (int64, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	userID, exists := sm.sessions[sessionID]
	return userID, exists
}

func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mu.Lock()
	delete(sm.sessions, sessionID)
	sm.mu.Unlock()
}

func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, sessionID string) {
	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60, // 7 days
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

func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
