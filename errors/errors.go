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
	ErrCodeNotInJail            ErrorCode = "NOT_IN_JAIL"
	ErrCodeAlreadyInJail        ErrorCode = "ALREADY_IN_JAIL"
	ErrCodePropertyNotOwned     ErrorCode = "PROPERTY_NOT_OWNED"
	ErrCodePropertyAlreadyMortgaged ErrorCode = "PROPERTY_ALREADY_MORTGAGED"
	ErrCodePropertyNotMortgaged ErrorCode = "PROPERTY_NOT_MORTGAGED"
	ErrCodeNoMonopoly           ErrorCode = "NO_MONOPOLY"
	ErrCodeMaxImprovements      ErrorCode = "MAX_IMPROVEMENTS"
	ErrCodeNoImprovements       ErrorCode = "NO_IMPROVEMENTS"
	ErrCodeUnevenBuild          ErrorCode = "UNEVEN_BUILD"
	ErrCodeHouseShortage        ErrorCode = "HOUSE_SHORTAGE"
	ErrCodeHotelShortage        ErrorCode = "HOTEL_SHORTAGE"
	ErrCodeAuctionInProgress    ErrorCode = "AUCTION_IN_PROGRESS"
	ErrCodeNoAuction            ErrorCode = "NO_AUCTION"
	ErrCodeNotYourBid           ErrorCode = "NOT_YOUR_BID"
	ErrCodeBidTooLow            ErrorCode = "BID_TOO_LOW"

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

func NotInJail() *AppError {
	return New(ErrCodeNotInJail, "You are not in jail")
}

func AlreadyInJail() *AppError {
	return New(ErrCodeAlreadyInJail, "You are already in jail")
}

func PropertyNotOwned() *AppError {
	return New(ErrCodePropertyNotOwned, "You don't own this property")
}

func PropertyAlreadyMortgaged() *AppError {
	return New(ErrCodePropertyAlreadyMortgaged, "This property is already mortgaged")
}

func PropertyNotMortgaged() *AppError {
	return New(ErrCodePropertyNotMortgaged, "This property is not mortgaged")
}

func NoMonopoly() *AppError {
	return New(ErrCodeNoMonopoly, "You must own all properties in this color group to build")
}

func MaxImprovements() *AppError {
	return New(ErrCodeMaxImprovements, "This property already has a hotel")
}

func NoImprovements() *AppError {
	return New(ErrCodeNoImprovements, "This property has no houses to sell")
}

func UnevenBuild() *AppError {
	return New(ErrCodeUnevenBuild, "You must build evenly across all properties in a color group")
}

func HouseShortage() *AppError {
	return New(ErrCodeHouseShortage, "No houses available (max 32 houses in game)")
}

func HotelShortage() *AppError {
	return New(ErrCodeHotelShortage, "No hotels available (max 12 hotels in game)")
}

func AuctionInProgress() *AppError {
	return New(ErrCodeAuctionInProgress, "An auction is already in progress")
}

func NoAuction() *AppError {
	return New(ErrCodeNoAuction, "No auction is in progress")
}

func NotYourBid() *AppError {
	return New(ErrCodeNotYourBid, "It's not your turn to bid")
}

func BidTooLow() *AppError {
	return New(ErrCodeBidTooLow, "Bid must be higher than current bid")
}
