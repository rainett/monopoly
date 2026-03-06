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
	InJail        bool   `json:"inJail"`
	JailTurns     int    `json:"jailTurns"`
}

type GameState struct {
	ID                  int64            `json:"id"`
	Status              string           `json:"status"`
	Players             []*Player        `json:"players"`
	CurrentPlayerID     int64            `json:"currentPlayerId"`
	MaxPlayers          int              `json:"maxPlayers"`
	Properties          map[int]int64    `json:"properties"`
	MortgagedProperties map[int]bool     `json:"mortgagedProperties"`
	Improvements        map[int]int      `json:"improvements"` // position -> house count (1-4 houses, 5 = hotel)
	Board               [40]BoardSpace   `json:"board"`
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
	UserID       int64  `json:"userId"`
	Die1         int    `json:"die1"`
	Die2         int    `json:"die2"`
	Total        int    `json:"total"`
	OldPos       int    `json:"oldPos"`
	NewPos       int    `json:"newPos"`
	PassedGo     bool   `json:"passedGo"`
	SpaceName    string `json:"spaceName"`
	SpaceType    string `json:"spaceType"`
	IsDoubles    bool   `json:"isDoubles"`
	DoublesCount int    `json:"doublesCount"`
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
	UserID int64  `json:"userId"`
	OldPos int    `json:"oldPos"`
	Reason string `json:"reason"` // "landed" or "three_doubles"
}

type JailEscapePayload struct {
	UserID    int64  `json:"userId"`
	Method    string `json:"method"` // "doubles", "bail", "card"
	NewMoney  int    `json:"newMoney,omitempty"`
}

type JailRollFailedPayload struct {
	UserID     int64 `json:"userId"`
	Die1       int   `json:"die1"`
	Die2       int   `json:"die2"`
	JailTurns  int   `json:"jailTurns"`
	ForcedBail bool  `json:"forcedBail"` // True if this was the 3rd failed roll
	NewMoney   int   `json:"newMoney,omitempty"`
}

type PropertyMortgagedPayload struct {
	UserID   int64  `json:"userId"`
	Position int    `json:"position"`
	Name     string `json:"name"`
	Amount   int    `json:"amount"`   // Amount received (half of price)
	NewMoney int    `json:"newMoney"`
}

type PropertyUnmortgagedPayload struct {
	UserID   int64  `json:"userId"`
	Position int    `json:"position"`
	Name     string `json:"name"`
	Amount   int    `json:"amount"`   // Amount paid (110% of mortgage value)
	NewMoney int    `json:"newMoney"`
}

type HouseBuiltPayload struct {
	UserID      int64  `json:"userId"`
	Position    int    `json:"position"`
	Name        string `json:"name"`
	HouseCount  int    `json:"houseCount"`  // 1-4 for houses, 5 for hotel
	Cost        int    `json:"cost"`
	NewMoney    int    `json:"newMoney"`
}

type HouseSoldPayload struct {
	UserID     int64  `json:"userId"`
	Position   int    `json:"position"`
	Name       string `json:"name"`
	HouseCount int    `json:"houseCount"`
	Refund     int    `json:"refund"`
	NewMoney   int    `json:"newMoney"`
}

type CardDrawnPayload struct {
	UserID     int64  `json:"userId"`
	DeckType   string `json:"deckType"` // "chance" or "community"
	CardText   string `json:"cardText"`
	CardType   string `json:"cardType"`
	Effect     string `json:"effect,omitempty"` // Description of what happened
	NewMoney   int    `json:"newMoney,omitempty"`
	NewPos     int    `json:"newPos,omitempty"`
}

// TradeOffer represents what each side offers in a trade
type TradeOffer struct {
	OfferedMoney       int   `json:"offeredMoney"`
	RequestedMoney     int   `json:"requestedMoney"`
	OfferedProperties  []int `json:"offeredProperties"`  // Positions of properties offered
	RequestedProperties []int `json:"requestedProperties"` // Positions of properties requested
}

// Trade represents a pending trade between players
type Trade struct {
	ID         int64      `json:"id"`
	GameID     int64      `json:"gameId"`
	FromUserID int64      `json:"fromUserId"`
	ToUserID   int64      `json:"toUserId"`
	Offer      TradeOffer `json:"offer"`
	Status     string     `json:"status"` // "pending", "accepted", "declined", "cancelled"
}

type TradeProposedPayload struct {
	Trade        *Trade `json:"trade"`
	FromUsername string `json:"fromUsername"`
	ToUsername   string `json:"toUsername"`
}

type TradeResponsePayload struct {
	TradeID      int64  `json:"tradeId"`
	FromUserID   int64  `json:"fromUserId"`
	ToUserID     int64  `json:"toUserId"`
	Status       string `json:"status"`
	FromUsername string `json:"fromUsername"`
	ToUsername   string `json:"toUsername"`
}

// Auction represents an active property auction
type Auction struct {
	GameID          int64   `json:"gameId"`
	Position        int     `json:"position"`
	PropertyName    string  `json:"propertyName"`
	HighestBid      int     `json:"highestBid"`
	HighestBidderID int64   `json:"highestBidderId"`
	BidderOrder     []int64 `json:"bidderOrder"`     // Order of bidding (round-robin)
	CurrentBidder   int     `json:"currentBidderIdx"` // Index in BidderOrder of whose turn it is
	PassedBidders   map[int64]bool `json:"-"`        // Players who have passed (exited auction)
}

type AuctionStartedPayload struct {
	Position       int    `json:"position"`
	PropertyName   string `json:"propertyName"`
	StartingBid    int    `json:"startingBid"`
	BidderOrder    []int64 `json:"bidderOrder"`
	CurrentBidder  int64  `json:"currentBidderId"`
}

type AuctionBidPayload struct {
	Position       int    `json:"position"`
	BidderID       int64  `json:"bidderId"`
	BidderName     string `json:"bidderName"`
	BidAmount      int    `json:"bidAmount"`
	NextBidderID   int64  `json:"nextBidderId"`
}

type AuctionPassedPayload struct {
	Position       int    `json:"position"`
	PasserID       int64  `json:"passerId"`
	PasserName     string `json:"passerName"`
	NextBidderID   int64  `json:"nextBidderId"`
	RemainingCount int    `json:"remainingCount"`
}

type AuctionEndedPayload struct {
	Position     int    `json:"position"`
	PropertyName string `json:"propertyName"`
	WinnerID     int64  `json:"winnerId"`
	WinnerName   string `json:"winnerName"`
	FinalBid     int    `json:"finalBid"`
	NoWinner     bool   `json:"noWinner"` // True if everyone passed
}
