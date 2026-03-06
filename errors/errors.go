package errors

import "fmt"

// ErrorCode represents a specific error type
type ErrorCode string

const (
	// Game errors
	ErrCodeGameNotFound     ErrorCode = "GAME_NOT_FOUND"
	ErrCodeGameFull         ErrorCode = "GAME_FULL"
	ErrCodeGameStarted      ErrorCode = "GAME_STARTED"
	ErrCodeGameNotStarted   ErrorCode = "GAME_NOT_STARTED"
	ErrCodeNotEnoughPlayers ErrorCode = "NOT_ENOUGH_PLAYERS"
	ErrCodeAlreadyInGame    ErrorCode = "ALREADY_IN_GAME"
	ErrCodeNotInGame        ErrorCode = "NOT_IN_GAME"
	ErrCodeNotYourTurn      ErrorCode = "NOT_YOUR_TURN"
	ErrCodeNotPlayer        ErrorCode = "NOT_PLAYER"
	ErrCodeAlreadyRolled    ErrorCode = "ALREADY_ROLLED"
	ErrCodeMustRoll         ErrorCode = "MUST_ROLL"
	ErrCodePendingAction    ErrorCode = "PENDING_ACTION"
	ErrCodeCannotBuy        ErrorCode = "CANNOT_BUY"
	ErrCodeInsufficientFunds ErrorCode = "INSUFFICIENT_FUNDS"
	ErrCodePlayerBankrupt   ErrorCode = "PLAYER_BANKRUPT"

	// Auth errors
	ErrCodeUnauthorized      ErrorCode = "UNAUTHORIZED"
	ErrCodeInvalidCredentials ErrorCode = "INVALID_CREDENTIALS"
	ErrCodeInvalidUsername   ErrorCode = "INVALID_USERNAME"
	ErrCodeInvalidPassword   ErrorCode = "INVALID_PASSWORD"
	ErrCodeUserExists        ErrorCode = "USER_EXISTS"
	ErrCodeUserNotFound      ErrorCode = "USER_NOT_FOUND"

	// General errors
	ErrCodeInternal    ErrorCode = "INTERNAL_ERROR"
	ErrCodeBadRequest  ErrorCode = "BAD_REQUEST"
	ErrCodeNotFound    ErrorCode = "NOT_FOUND"
	ErrCodeForbidden   ErrorCode = "FORBIDDEN"
)

// AppError represents a user-friendly application error
type AppError struct {
	Code    ErrorCode // Machine-readable code
	Message string    // User-friendly message
	Detail  string    // Internal detail for logging
	Err     error     // Underlying error for unwrapping
}

func (e *AppError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// UserMessage returns the user-friendly message only
func (e *AppError) UserMessage() string {
	return e.Message
}

// New creates a new AppError
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// Newf creates a new AppError with formatted message
func Newf(code ErrorCode, format string, args ...interface{}) *AppError {
	return &AppError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// Wrap wraps an existing error with user-friendly information
func Wrap(err error, code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Detail:  err.Error(),
		Err:     err,
	}
}

// Predefined errors with user-friendly messages

func GameNotFound() *AppError {
	return New(ErrCodeGameNotFound, "Game not found")
}

func GameFull() *AppError {
	return New(ErrCodeGameFull, "This game is full")
}

func GameAlreadyStarted() *AppError {
	return New(ErrCodeGameStarted, "This game has already started")
}

func GameNotStarted() *AppError {
	return New(ErrCodeGameNotStarted, "This game has not started yet")
}

func NotEnoughPlayers() *AppError {
	return New(ErrCodeNotEnoughPlayers, "Not enough players to start the game")
}

func AlreadyInGame() *AppError {
	return New(ErrCodeAlreadyInGame, "You are already in a game")
}

func NotInGame() *AppError {
	return New(ErrCodeNotInGame, "You are not in this game")
}

func NotYourTurn() *AppError {
	return New(ErrCodeNotYourTurn, "It's not your turn")
}

func NotPlayer() *AppError {
	return New(ErrCodeNotPlayer, "You are not a player in this game")
}

func Unauthorized() *AppError {
	return New(ErrCodeUnauthorized, "Please log in to continue")
}

func InvalidCredentials() *AppError {
	return New(ErrCodeInvalidCredentials, "Invalid username or password")
}

func InvalidUsername() *AppError {
	return New(ErrCodeInvalidUsername, "Username must be 3-20 alphanumeric characters")
}

func InvalidPassword() *AppError {
	return New(ErrCodeInvalidPassword, "Password must be at least 8 characters with letters and numbers")
}

func UserExists() *AppError {
	return New(ErrCodeUserExists, "Username already taken")
}

func UserNotFound() *AppError {
	return New(ErrCodeUserNotFound, "User not found")
}

func InternalError(detail string) *AppError {
	return &AppError{
		Code:    ErrCodeInternal,
		Message: "Something went wrong. Please try again later.",
		Detail:  detail,
	}
}

func BadRequest(message string) *AppError {
	return New(ErrCodeBadRequest, message)
}

func AlreadyRolled() *AppError {
	return New(ErrCodeAlreadyRolled, "You have already rolled this turn")
}

func MustRoll() *AppError {
	return New(ErrCodeMustRoll, "You must roll the dice before ending your turn")
}

func PendingAction() *AppError {
	return New(ErrCodePendingAction, "You must complete your current action first")
}

func CannotBuy() *AppError {
	return New(ErrCodeCannotBuy, "You cannot buy this property")
}

func InsufficientFunds() *AppError {
	return New(ErrCodeInsufficientFunds, "You don't have enough money")
}

func PlayerBankrupt() *AppError {
	return New(ErrCodePlayerBankrupt, "This player is bankrupt")
}
