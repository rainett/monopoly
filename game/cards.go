package game

import "math/rand"

type CardType string

const (
	CardTypeCollectMoney    CardType = "collect_money"
	CardTypePayMoney        CardType = "pay_money"
	CardTypeMoveTo          CardType = "move_to"
	CardTypeMoveBack        CardType = "move_back"
	CardTypeGoToJail        CardType = "go_to_jail"
	CardTypeGetOutOfJail    CardType = "get_out_of_jail"
	CardTypeRepairs         CardType = "repairs"
	CardTypePayEachPlayer   CardType = "pay_each_player"
	CardTypeCollectFromEach CardType = "collect_from_each"
	CardTypeAdvanceToNearest CardType = "advance_to_nearest"
)

type Card struct {
	ID          int      `json:"id"`
	Type        CardType `json:"type"`
	Text        string   `json:"text"`
	Value       int      `json:"value,omitempty"`       // Amount for money cards
	Value2      int      `json:"value2,omitempty"`      // Second value (e.g., hotel repair cost)
	Destination int      `json:"destination,omitempty"` // Position for move cards
	NearestType string   `json:"nearestType,omitempty"` // "railroad" or "utility" for advance_to_nearest
}

// ChanceCards contains all Chance card definitions
var ChanceCards = []Card{
	{1, CardTypeMoveTo, "Advance to GO", 200, 0, 0, ""},
	{2, CardTypeMoveTo, "Advance to Illinois Ave.", 0, 0, 24, ""},
	{3, CardTypeMoveTo, "Advance to St. Charles Place", 0, 0, 11, ""},
	{4, CardTypeAdvanceToNearest, "Advance to nearest Utility. If unowned, you may buy it. If owned, throw dice and pay owner 10x amount.", 0, 0, 0, "utility"},
	{5, CardTypeAdvanceToNearest, "Advance to nearest Railroad. Pay owner twice normal rent. If unowned, you may buy it.", 0, 0, 0, "railroad"},
	{6, CardTypeCollectMoney, "Bank pays you dividend of $50", 50, 0, 0, ""},
	{7, CardTypeGetOutOfJail, "Get Out of Jail Free", 0, 0, 0, ""},
	{8, CardTypeMoveBack, "Go Back 3 Spaces", 3, 0, 0, ""},
	{9, CardTypeGoToJail, "Go to Jail. Do not pass GO, do not collect $200", 0, 0, 0, ""},
	{10, CardTypeRepairs, "Make general repairs on all your property: $25 per house, $100 per hotel", 25, 100, 0, ""},
	{11, CardTypePayMoney, "Pay poor tax of $15", 15, 0, 0, ""},
	{12, CardTypeMoveTo, "Take a trip to Reading Railroad", 0, 0, 5, ""},
	{13, CardTypeMoveTo, "Take a walk on the Boardwalk", 0, 0, 39, ""},
	{14, CardTypePayEachPlayer, "You have been elected Chairman of the Board. Pay each player $50", 50, 0, 0, ""},
	{15, CardTypeCollectMoney, "Your building and loan matures. Collect $150", 150, 0, 0, ""},
	{16, CardTypeCollectMoney, "You have won a crossword competition. Collect $100", 100, 0, 0, ""},
}

// CommunityChestCards contains all Community Chest card definitions
var CommunityChestCards = []Card{
	{1, CardTypeMoveTo, "Advance to GO", 200, 0, 0, ""},
	{2, CardTypeCollectMoney, "Bank error in your favor. Collect $200", 200, 0, 0, ""},
	{3, CardTypePayMoney, "Doctor's fee. Pay $50", 50, 0, 0, ""},
	{4, CardTypeCollectMoney, "From sale of stock you get $50", 50, 0, 0, ""},
	{5, CardTypeGetOutOfJail, "Get Out of Jail Free", 0, 0, 0, ""},
	{6, CardTypeGoToJail, "Go to Jail. Do not pass GO, do not collect $200", 0, 0, 0, ""},
	{7, CardTypeCollectFromEach, "Grand Opera Night. Collect $50 from every player", 50, 0, 0, ""},
	{8, CardTypeCollectMoney, "Holiday fund matures. Receive $100", 100, 0, 0, ""},
	{9, CardTypeCollectMoney, "Income tax refund. Collect $20", 20, 0, 0, ""},
	{10, CardTypeCollectFromEach, "It is your birthday. Collect $10 from every player", 10, 0, 0, ""},
	{11, CardTypeCollectMoney, "Life insurance matures. Collect $100", 100, 0, 0, ""},
	{12, CardTypePayMoney, "Hospital fees. Pay $100", 100, 0, 0, ""},
	{13, CardTypePayMoney, "School fees. Pay $50", 50, 0, 0, ""},
	{14, CardTypeCollectMoney, "Receive $25 consultancy fee", 25, 0, 0, ""},
	{15, CardTypeRepairs, "You are assessed for street repairs: $40 per house, $115 per hotel", 40, 115, 0, ""},
	{16, CardTypeCollectMoney, "You have won second prize in a beauty contest. Collect $10", 10, 0, 0, ""},
}

// ShuffleDeck returns a shuffled order of card indices
func ShuffleDeck(numCards int) []int {
	order := make([]int, numCards)
	for i := range order {
		order[i] = i
	}
	rand.Shuffle(len(order), func(i, j int) {
		order[i], order[j] = order[j], order[i]
	})
	return order
}
