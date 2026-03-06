package auth

import (
	"fmt"
	"monopoly/errors"
	"monopoly/store"
	"regexp"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	store   store.AuthStore
	session *SessionManager
}

func NewService(store store.AuthStore, sessionManager *SessionManager) *Service {
	return &Service{
		store:   store,
		session: sessionManager,
	}
}

func (s *Service) Register(username, password string) error {
	username = SanitizeString(username)
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
		return errors.UserExists()
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
	username = SanitizeString(username)

	user, err := s.store.GetUserByUsername(username)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return "", errors.InvalidCredentials()
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", errors.InvalidCredentials()
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
		return errors.InvalidUsername()
	}
	matched, _ := regexp.MatchString("^[a-zA-Z0-9]+$", username)
	if !matched {
		return errors.InvalidUsername()
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.InvalidPassword()
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
		return errors.InvalidPassword()
	}

	return nil
}
