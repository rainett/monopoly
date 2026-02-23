package game

const (
	StatusWaiting    = "waiting"
	StatusInProgress = "in_progress"
	StatusFinished   = "finished"
)

type Player struct {
	UserID        int64  `json:"userId"`
	Username      string `json:"username"`
	Order         int    `json:"order"`
	IsReady       bool   `json:"isReady"`
	IsCurrentTurn bool   `json:"isCurrentTurn"`
}

type GameState struct {
	ID              int64     `json:"id"`
	Status          string    `json:"status"`
	Players         []*Player `json:"players"`
	CurrentPlayerID int64     `json:"currentPlayerId"`
	MaxPlayers      int       `json:"maxPlayers"`
}

type Event struct {
	Type    string      `json:"type"`
	GameID  int64       `json:"gameId"`
	Payload interface{} `json:"payload"`
}

type PlayerJoinedPayload struct {
	Player *Player `json:"player"`
}

type PlayerReadyPayload struct {
	UserID  int64 `json:"userId"`
	IsReady bool  `json:"isReady"`
}

type GameStartedPayload struct {
	CurrentPlayerID int64 `json:"currentPlayerId"`
}

type TurnChangedPayload struct {
	PreviousPlayerID int64 `json:"previousPlayerId"`
	CurrentPlayerID  int64 `json:"currentPlayerId"`
}
