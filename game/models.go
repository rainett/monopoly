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
	Money         int    `json:"money"`
	Position      int    `json:"position"`
	IsBankrupt    bool   `json:"isBankrupt"`
	HasRolled     bool   `json:"hasRolled"`
	PendingAction string `json:"pendingAction"`
}

type GameState struct {
	ID              int64            `json:"id"`
	Status          string           `json:"status"`
	Players         []*Player        `json:"players"`
	CurrentPlayerID int64            `json:"currentPlayerId"`
	MaxPlayers      int              `json:"maxPlayers"`
	Properties      map[int]int64    `json:"properties"`
	Board           [40]BoardSpace   `json:"board"`
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

type GameFinishedPayload struct {
	Players  []*Player `json:"players"`
	WinnerID int64     `json:"winnerId"`
}

type DiceRolledPayload struct {
	UserID    int64  `json:"userId"`
	Die1      int    `json:"die1"`
	Die2      int    `json:"die2"`
	Total     int    `json:"total"`
	OldPos    int    `json:"oldPos"`
	NewPos    int    `json:"newPos"`
	PassedGo  bool   `json:"passedGo"`
	SpaceName string `json:"spaceName"`
	SpaceType string `json:"spaceType"`
}

type BuyPromptPayload struct {
	UserID   int64  `json:"userId"`
	Position int    `json:"position"`
	Name     string `json:"name"`
	Price    int    `json:"price"`
}

type PropertyBoughtPayload struct {
	UserID   int64  `json:"userId"`
	Position int    `json:"position"`
	Name     string `json:"name"`
	Price    int    `json:"price"`
	NewMoney int    `json:"newMoney"`
}

type PropertyPassedPayload struct {
	UserID   int64  `json:"userId"`
	Position int    `json:"position"`
	Name     string `json:"name"`
}

type RentPaidPayload struct {
	PayerID    int64  `json:"payerId"`
	OwnerID    int64  `json:"ownerId"`
	Position   int    `json:"position"`
	Name       string `json:"name"`
	Amount     int    `json:"amount"`
	PayerMoney int    `json:"payerMoney"`
	OwnerMoney int    `json:"ownerMoney"`
}

type TaxPaidPayload struct {
	UserID   int64  `json:"userId"`
	Position int    `json:"position"`
	Amount   int    `json:"amount"`
	NewMoney int    `json:"newMoney"`
}

type PlayerBankruptPayload struct {
	UserID     int64  `json:"userId"`
	Username   string `json:"username"`
	Reason     string `json:"reason"`
	CreditorID int64  `json:"creditorId,omitempty"`
}

type GoToJailPayload struct {
	UserID int64 `json:"userId"`
	OldPos int   `json:"oldPos"`
}
