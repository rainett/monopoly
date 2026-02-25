package auth

import (
	"errors"
	"fmt"
	"monopoly/store"
	"regexp"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidUsername    = errors.New("username must be alphanumeric and 3-20 characters")
	ErrInvalidPassword    = errors.New("password must be at least 8 characters and contain both letters and numbers")
	ErrUserExists         = errors.New("username already exists")
	ErrInvalidCredentials = errors.New("invalid username or password")
)

type Service struct {
	store   store.Store
	session *SessionManager
}

func NewService(store store.Store, sessionManager *SessionManager) *Service {
	return &Service{
		store:   store,
		session: sessionManager,
	}
}

func (s *Service) Register(username, password string) error {
	// Sanitize username to prevent XSS
	username = SanitizeUsername(username)

	if err := validateUsername(username); err != nil {
		return err
	}

	if err := validatePassword(password); err != nil {
		return err
	}

	existingUser, err := s.store.GetUserByUsername(username)
	if err != nil {
		return fmt.Errorf("failed to check existing user: %w", err)
	}
	if existingUser != nil {
		return ErrUserExists
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = s.store.CreateUser(username, string(passwordHash))
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (s *Service) Login(username, password string) (string, error) {
	// Sanitize username to prevent XSS
	username = SanitizeUsername(username)

	user, err := s.store.GetUserByUsername(username)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	sessionID, err := s.session.CreateSession(user.ID)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	return sessionID, nil
}

func (s *Service) Logout(sessionID string) {
	s.session.DeleteSession(sessionID)
}

func (s *Service) ValidateSession(sessionID string) (int64, bool) {
	return s.session.GetUserID(sessionID)
}

func (s *Service) GetSessionManager() *SessionManager {
	return s.session
}

func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 20 {
		return ErrInvalidUsername
	}
	matched, _ := regexp.MatchString("^[a-zA-Z0-9]+$", username)
	if !matched {
		return ErrInvalidUsername
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return ErrInvalidPassword
	}

	hasLetter := false
	hasNumber := false

	for _, char := range password {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') {
			hasLetter = true
		}
		if char >= '0' && char <= '9' {
			hasNumber = true
		}
	}

	if !hasLetter || !hasNumber {
		return ErrInvalidPassword
	}

	return nil
}
