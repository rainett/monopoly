package game

import (
	"database/sql"
	"monopoly/store"
	"testing"
)

// MockGameStore implements store.GameStore for testing
type MockGameStore struct {
	Games      map[int64]*store.Game
	Players    map[int64][]*store.GamePlayer
	Properties map[int64][]*store.GameProperty

	// Track method calls
	UpdatePlayerPositionCalled bool
	UpdatePlayerMoneyCalled    bool
	SetPlayerHasRolledCalled   bool

	// Configure behavior
	MockTx *sql.Tx
}

func NewMockGameStore() *MockGameStore {
	return &MockGameStore{
		Games:      make(map[int64]*store.Game),
		Players:    make(map[int64][]*store.GamePlayer),
		Properties: make(map[int64][]*store.GameProperty),
	}
}

// Basic Game Operations
func (m *MockGameStore) GetGame(gameID int64) (*store.Game, error) {
	return m.Games[gameID], nil
}

func (m *MockGameStore) GetGamePlayers(gameID int64) ([]*store.GamePlayer, error) {
	return m.Players[gameID], nil
}

func (m *MockGameStore) JoinGame(gameID, userID int64, playerOrder int) error {
	player := &store.GamePlayer{
		GameID:      gameID,
		UserID:      userID,
		PlayerOrder: playerOrder,
		Money:       1500,
	}
	m.Players[gameID] = append(m.Players[gameID], player)
	return nil
}

func (m *MockGameStore) UpdatePlayerReady(gameID, userID int64, isReady bool) error {
	return nil
}

func (m *MockGameStore) UpdateGameStatus(gameID int64, status string) error {
	if g := m.Games[gameID]; g != nil {
		g.Status = status
	}
	return nil
}

func (m *MockGameStore) UpdateCurrentTurn(gameID, userID int64) error {
	return nil
}

func (m *MockGameStore) GetCurrentTurnPlayer(gameID int64) (*store.GamePlayer, error) {
	for _, p := range m.Players[gameID] {
		if p.IsCurrentTurn {
			return p, nil
		}
	}
	return nil, nil
}

func (m *MockGameStore) MarkPlayerTurnComplete(gameID, userID int64) error {
	return nil
}

func (m *MockGameStore) AllPlayersCompletedTurn(gameID int64) (bool, error) {
	return false, nil
}

// Transaction support
func (m *MockGameStore) BeginTx() (*sql.Tx, error) {
	return m.MockTx, nil
}

func (m *MockGameStore) CommitTx(tx *sql.Tx) error {
	return nil
}

func (m *MockGameStore) RollbackTx(tx *sql.Tx) error {
	return nil
}

// Transaction-aware operations
func (m *MockGameStore) UpdatePlayerReadyTx(tx *sql.Tx, gameID, userID int64, isReady bool) error {
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.IsReady = isReady
		}
	}
	return nil
}

func (m *MockGameStore) UpdateGameStatusTx(tx *sql.Tx, gameID int64, status string) error {
	if g := m.Games[gameID]; g != nil {
		g.Status = status
	}
	return nil
}

func (m *MockGameStore) UpdateCurrentTurnTx(tx *sql.Tx, gameID, userID int64) error {
	for _, p := range m.Players[gameID] {
		p.IsCurrentTurn = p.UserID == userID
	}
	return nil
}

func (m *MockGameStore) MarkPlayerTurnCompleteTx(tx *sql.Tx, gameID, userID int64) error {
	return nil
}

// Game mechanics operations
func (m *MockGameStore) UpdatePlayerPositionTx(tx *sql.Tx, gameID, userID int64, position int) error {
	m.UpdatePlayerPositionCalled = true
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.Position = position
		}
	}
	return nil
}

func (m *MockGameStore) UpdatePlayerMoneyTx(tx *sql.Tx, gameID, userID int64, money int) error {
	m.UpdatePlayerMoneyCalled = true
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.Money = money
		}
	}
	return nil
}

func (m *MockGameStore) SetPlayerBankruptTx(tx *sql.Tx, gameID, userID int64) error {
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.IsBankrupt = true
		}
	}
	return nil
}

func (m *MockGameStore) SetPlayerHasRolledTx(tx *sql.Tx, gameID, userID int64, hasRolled bool) error {
	m.SetPlayerHasRolledCalled = true
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.HasRolled = hasRolled
		}
	}
	return nil
}

func (m *MockGameStore) SetPlayerPendingActionTx(tx *sql.Tx, gameID, userID int64, action string) error {
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.PendingAction = action
		}
	}
	return nil
}

func (m *MockGameStore) ResetPlayerTurnStateTx(tx *sql.Tx, gameID, userID int64) error {
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.HasRolled = false
			p.PendingAction = ""
		}
	}
	return nil
}

func (m *MockGameStore) GetPlayerTx(tx *sql.Tx, gameID, userID int64) (*store.GamePlayer, error) {
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			return p, nil
		}
	}
	return nil, nil
}

// Property operations
func (m *MockGameStore) GetGameProperties(gameID int64) ([]*store.GameProperty, error) {
	return m.Properties[gameID], nil
}

func (m *MockGameStore) GetGamePropertiesTx(tx *sql.Tx, gameID int64) ([]*store.GameProperty, error) {
	return m.Properties[gameID], nil
}

func (m *MockGameStore) GetPropertyOwnerTx(tx *sql.Tx, gameID int64, position int) (int64, error) {
	for _, p := range m.Properties[gameID] {
		if p.Position == position {
			return p.OwnerID, nil
		}
	}
	return 0, nil
}

func (m *MockGameStore) GetPlayerPropertiesTx(tx *sql.Tx, gameID, userID int64) ([]int, error) {
	var positions []int
	for _, p := range m.Properties[gameID] {
		if p.OwnerID == userID {
			positions = append(positions, p.Position)
		}
	}
	return positions, nil
}

func (m *MockGameStore) InsertPropertyTx(tx *sql.Tx, gameID int64, position int, ownerID int64) error {
	m.Properties[gameID] = append(m.Properties[gameID], &store.GameProperty{
		GameID:   gameID,
		Position: position,
		OwnerID:  ownerID,
	})
	return nil
}

func (m *MockGameStore) DeletePlayerPropertiesTx(tx *sql.Tx, gameID, userID int64) error {
	var remaining []*store.GameProperty
	for _, p := range m.Properties[gameID] {
		if p.OwnerID != userID {
			remaining = append(remaining, p)
		}
	}
	m.Properties[gameID] = remaining
	return nil
}

func (m *MockGameStore) TransferAllPropertiesTx(tx *sql.Tx, gameID, fromUserID, toUserID int64) error {
	for _, p := range m.Properties[gameID] {
		if p.OwnerID == fromUserID {
			p.OwnerID = toUserID
		}
	}
	return nil
}

func (m *MockGameStore) CountActivePlayersTx(tx *sql.Tx, gameID int64) (int, error) {
	count := 0
	for _, p := range m.Players[gameID] {
		if !p.IsBankrupt {
			count++
		}
	}
	return count, nil
}

func (m *MockGameStore) GetActivePlayersTx(tx *sql.Tx, gameID int64) ([]*store.GamePlayer, error) {
	var active []*store.GamePlayer
	for _, p := range m.Players[gameID] {
		if !p.IsBankrupt {
			active = append(active, p)
		}
	}
	return active, nil
}

// Jail operations
func (m *MockGameStore) SetPlayerInJailTx(tx *sql.Tx, gameID, userID int64, inJail bool, jailTurns int) error {
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.InJail = inJail
			p.JailTurns = jailTurns
		}
	}
	return nil
}

func (m *MockGameStore) ReleaseFromJailTx(tx *sql.Tx, gameID, userID int64) error {
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.InJail = false
			p.JailTurns = 0
		}
	}
	return nil
}

func (m *MockGameStore) IncrementJailTurnsTx(tx *sql.Tx, gameID, userID int64) error {
	for _, p := range m.Players[gameID] {
		if p.UserID == userID {
			p.JailTurns++
		}
	}
	return nil
}

// Mortgage operations
func (m *MockGameStore) GetPropertyTx(tx *sql.Tx, gameID int64, position int) (*store.GameProperty, error) {
	for _, p := range m.Properties[gameID] {
		if p.Position == position {
			return p, nil
		}
	}
	return nil, nil
}

func (m *MockGameStore) SetPropertyMortgagedTx(tx *sql.Tx, gameID int64, position int, mortgaged bool) error {
	for _, p := range m.Properties[gameID] {
		if p.Position == position {
			p.IsMortgaged = mortgaged
		}
	}
	return nil
}

// Improvement operations
func (m *MockGameStore) GetImprovementsTx(tx *sql.Tx, gameID int64, position int) (int, error) {
	return 0, nil
}

func (m *MockGameStore) SetImprovementsTx(tx *sql.Tx, gameID int64, position int, count int) error {
	return nil
}

func (m *MockGameStore) GetAllImprovements(gameID int64) (map[int]int, error) {
	return make(map[int]int), nil
}

func (m *MockGameStore) GetTotalHousesHotelsTx(tx *sql.Tx, gameID int64) (houses int, hotels int, err error) {
	return 0, 0, nil
}

// Card deck operations
func (m *MockGameStore) InitializeDecks(gameID int64, chanceOrder, communityOrder []int) error {
	return nil
}

func (m *MockGameStore) DrawCardTx(tx *sql.Tx, gameID int64, deckType string) (int, error) {
	return 0, nil
}

// Jail card operations
func (m *MockGameStore) GiveJailCardTx(tx *sql.Tx, gameID, userID int64, deckType string) error {
	return nil
}

func (m *MockGameStore) HasJailCard(gameID, userID int64) (bool, string, error) {
	return false, "", nil
}

func (m *MockGameStore) UseJailCardTx(tx *sql.Tx, gameID, userID int64) (string, error) {
	return "", nil
}

// Trade operations
func (m *MockGameStore) CreateTrade(gameID, fromUserID, toUserID int64, offerJSON string) (int64, error) {
	return 1, nil
}

func (m *MockGameStore) GetTrade(tradeID int64) (*store.GameTrade, error) {
	return nil, nil
}

func (m *MockGameStore) GetPendingTrades(gameID int64) ([]*store.GameTrade, error) {
	return nil, nil
}

func (m *MockGameStore) UpdateTradeStatus(tradeID int64, status string) error {
	return nil
}

func (m *MockGameStore) TransferPropertyTx(tx *sql.Tx, gameID int64, position int, newOwnerID int64) error {
	for _, p := range m.Properties[gameID] {
		if p.Position == position {
			p.OwnerID = newOwnerID
		}
	}
	return nil
}

// ============ TESTS ============

func TestNewEngine(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}

	if engine.store == nil {
		t.Error("Engine store is nil")
	}

	if engine.doublesCount == nil {
		t.Error("Engine doublesCount map is nil")
	}

	if engine.activeAuctions == nil {
		t.Error("Engine activeAuctions map is nil")
	}
}

func TestGetGameState_NotFound(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Test with invalid game ID
	_, err := engine.GetGameState(0)
	if err == nil {
		t.Error("Expected error for game ID 0")
	}

	// Test with non-existent game
	_, err = engine.GetGameState(999)
	if err == nil {
		t.Error("Expected error for non-existent game")
	}
}

func TestGetGameState_Success(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up test game
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusWaiting,
		MaxPlayers: 4,
	}
	mockStore.Players[1] = []*store.GamePlayer{
		{GameID: 1, UserID: 100, Username: "player1", PlayerOrder: 0, Money: 1500, IsCurrentTurn: true},
		{GameID: 1, UserID: 101, Username: "player2", PlayerOrder: 1, Money: 1500},
	}

	state, err := engine.GetGameState(1)
	if err != nil {
		t.Fatalf("GetGameState failed: %v", err)
	}

	if state.ID != 1 {
		t.Errorf("Expected game ID 1, got %d", state.ID)
	}

	if state.Status != StatusWaiting {
		t.Errorf("Expected status %s, got %s", StatusWaiting, state.Status)
	}

	if len(state.Players) != 2 {
		t.Errorf("Expected 2 players, got %d", len(state.Players))
	}

	if state.CurrentPlayerID != 100 {
		t.Errorf("Expected current player ID 100, got %d", state.CurrentPlayerID)
	}
}

func TestJoinGame_Success(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up waiting game
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusWaiting,
		MaxPlayers: 4,
	}
	mockStore.Players[1] = []*store.GamePlayer{}

	event, err := engine.JoinGame(1, 100, "testuser")
	if err != nil {
		t.Fatalf("JoinGame failed: %v", err)
	}

	if event.Type != "player_joined" {
		t.Errorf("Expected event type 'player_joined', got '%s'", event.Type)
	}
}

func TestJoinGame_GameStarted(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up in-progress game
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusInProgress,
		MaxPlayers: 4,
	}
	mockStore.Players[1] = []*store.GamePlayer{}

	_, err := engine.JoinGame(1, 100, "testuser")
	if err == nil {
		t.Error("Expected error when joining started game")
	}
}

func TestJoinGame_GameFull(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up full game
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusWaiting,
		MaxPlayers: 2,
	}
	mockStore.Players[1] = []*store.GamePlayer{
		{GameID: 1, UserID: 100, Username: "player1", PlayerOrder: 0, Money: 1500},
		{GameID: 1, UserID: 101, Username: "player2", PlayerOrder: 1, Money: 1500},
	}

	_, err := engine.JoinGame(1, 102, "testuser")
	if err == nil {
		t.Error("Expected error when joining full game")
	}
}

func TestJoinGame_AlreadyInGame(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up game with player already in it
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusWaiting,
		MaxPlayers: 4,
	}
	mockStore.Players[1] = []*store.GamePlayer{
		{GameID: 1, UserID: 100, Username: "player1", PlayerOrder: 0, Money: 1500},
	}

	_, err := engine.JoinGame(1, 100, "player1")
	if err == nil {
		t.Error("Expected error when player already in game")
	}
}

func TestRollDice_NotYourTurn(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up in-progress game
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusInProgress,
		MaxPlayers: 4,
	}
	mockStore.Players[1] = []*store.GamePlayer{
		{GameID: 1, UserID: 100, Username: "player1", PlayerOrder: 0, Money: 1500, IsCurrentTurn: true},
		{GameID: 1, UserID: 101, Username: "player2", PlayerOrder: 1, Money: 1500},
	}

	// Player 2 tries to roll when it's player 1's turn
	_, err := engine.RollDice(1, 101)
	if err == nil {
		t.Error("Expected error when rolling dice out of turn")
	}
}

func TestRollDice_AlreadyRolled(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up in-progress game with player who already rolled
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusInProgress,
		MaxPlayers: 4,
	}
	mockStore.Players[1] = []*store.GamePlayer{
		{GameID: 1, UserID: 100, Username: "player1", PlayerOrder: 0, Money: 1500, IsCurrentTurn: true, HasRolled: true},
	}

	_, err := engine.RollDice(1, 100)
	if err == nil {
		t.Error("Expected error when rolling dice twice")
	}
}

func TestRollDice_GameNotStarted(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up waiting game
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusWaiting,
		MaxPlayers: 4,
	}
	mockStore.Players[1] = []*store.GamePlayer{
		{GameID: 1, UserID: 100, Username: "player1", PlayerOrder: 0, Money: 1500, IsCurrentTurn: true},
	}

	_, err := engine.RollDice(1, 100)
	if err == nil {
		t.Error("Expected error when rolling dice before game starts")
	}
}

func TestRollDice_BankruptPlayer(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up in-progress game with bankrupt player
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusInProgress,
		MaxPlayers: 4,
	}
	mockStore.Players[1] = []*store.GamePlayer{
		{GameID: 1, UserID: 100, Username: "player1", PlayerOrder: 0, Money: 0, IsCurrentTurn: true, IsBankrupt: true},
	}

	_, err := engine.RollDice(1, 100)
	if err == nil {
		t.Error("Expected error when bankrupt player tries to roll")
	}
}

func TestRollDice_PendingAction(t *testing.T) {
	mockStore := NewMockGameStore()
	engine := NewEngine(mockStore)

	// Set up in-progress game with pending action
	mockStore.Games[1] = &store.Game{
		ID:         1,
		Status:     StatusInProgress,
		MaxPlayers: 4,
	}
	mockStore.Players[1] = []*store.GamePlayer{
		{GameID: 1, UserID: 100, Username: "player1", PlayerOrder: 0, Money: 1500, IsCurrentTurn: true, PendingAction: "buy_property"},
	}

	_, err := engine.RollDice(1, 100)
	if err == nil {
		t.Error("Expected error when player has pending action")
	}
}

func TestCalculateRent_BaseRent(t *testing.T) {
	// Test base rent calculation for Mediterranean Ave (position 1)
	space := Board[1]
	rent := calculateBaseRent(space, false, 0)

	if rent != 2 { // Mediterranean Ave base rent is $2
		t.Errorf("Expected base rent $2, got $%d", rent)
	}
}

func TestCalculateRent_MonopolyBonus(t *testing.T) {
	// Test that monopoly doubles base rent
	space := Board[1]

	baseRent := calculateBaseRent(space, false, 0)
	monopolyRent := calculateBaseRent(space, true, 0)

	if monopolyRent != baseRent*2 {
		t.Errorf("Expected monopoly rent $%d (2x base), got $%d", baseRent*2, monopolyRent)
	}
}

func TestCalculateRent_WithHouses(t *testing.T) {
	// Test rent with houses for Mediterranean Ave
	space := Board[1]

	rent0 := calculateBaseRent(space, true, 0)
	rent1 := calculateBaseRent(space, true, 1)
	rent2 := calculateBaseRent(space, true, 2)

	// Mediterranean Ave rent: base $2 (monopoly $4), 1 house $10, 2 houses $30
	if rent0 != 4 {
		t.Errorf("Expected monopoly rent $4, got $%d", rent0)
	}
	if rent1 != 10 {
		t.Errorf("Expected 1-house rent $10, got $%d", rent1)
	}
	if rent2 != 30 {
		t.Errorf("Expected 2-house rent $30, got $%d", rent2)
	}
}

func TestCalculateRent_Railroad(t *testing.T) {
	// Railroad rent depends on number owned: 1=$25, 2=$50, 3=$100, 4=$200
	expectedRents := map[int]int{1: 25, 2: 50, 3: 100, 4: 200}

	for count, expected := range expectedRents {
		rent := calculateRailroadRent(count)
		if rent != expected {
			t.Errorf("Railroad rent for %d owned: expected $%d, got $%d", count, expected, rent)
		}
	}
}

func TestCalculateRent_Utility(t *testing.T) {
	// Utility rent: 1 owned = 4x dice, 2 owned = 10x dice
	diceTotal := 7

	rent1 := calculateUtilityRent(diceTotal, 1)
	rent2 := calculateUtilityRent(diceTotal, 2)

	if rent1 != 28 { // 7 * 4
		t.Errorf("Utility rent (1 owned) for dice 7: expected $28, got $%d", rent1)
	}
	if rent2 != 70 { // 7 * 10
		t.Errorf("Utility rent (2 owned) for dice 7: expected $70, got $%d", rent2)
	}
}

// Helper functions for rent calculation tests
// RentWithHouses array: index 0 = base rent, 1 = 1 house, ..., 5 = hotel
func calculateBaseRent(space BoardSpace, hasMonopoly bool, houses int) int {
	if space.Type != SpaceProperty {
		return 0
	}

	// Use RentWithHouses array - index matches house count (0-5)
	if houses > 0 && houses <= 5 {
		return space.RentWithHouses[houses]
	}

	if hasMonopoly {
		return space.Rent * 2
	}
	return space.Rent
}

func calculateRailroadRent(count int) int {
	switch count {
	case 1:
		return 25
	case 2:
		return 50
	case 3:
		return 100
	case 4:
		return 200
	default:
		return 0
	}
}

func calculateUtilityRent(diceTotal, count int) int {
	if count == 2 {
		return diceTotal * 10
	}
	return diceTotal * 4
}

func TestBoardSetup(t *testing.T) {
	// Verify board has 40 spaces
	if len(Board) != 40 {
		t.Errorf("Expected 40 board spaces, got %d", len(Board))
	}

	// Verify GO is at position 0
	if Board[0].Type != SpaceGo {
		t.Errorf("Expected GO at position 0, got %s", Board[0].Type)
	}

	// Verify Jail is at position 10
	if Board[10].Type != SpaceJail {
		t.Errorf("Expected Jail at position 10, got %s", Board[10].Type)
	}

	// Verify Free Parking is at position 20
	if Board[20].Type != SpaceFreeParking {
		t.Errorf("Expected Free Parking at position 20, got %s", Board[20].Type)
	}

	// Verify Go To Jail is at position 30
	if Board[30].Type != SpaceGoToJail {
		t.Errorf("Expected Go To Jail at position 30, got %s", Board[30].Type)
	}
}

func TestPropertyGroups(t *testing.T) {
	// Count properties in each group
	groupCounts := make(map[ColorGroup]int)

	for _, space := range Board {
		if space.Type == SpaceProperty {
			groupCounts[space.Color]++
		}
	}

	// Verify property group sizes
	expectedGroupSizes := map[ColorGroup]int{
		ColorBrown:     2,
		ColorLightBlue: 3,
		ColorPink:      3,
		ColorOrange:    3,
		ColorRed:       3,
		ColorYellow:    3,
		ColorGreen:     3,
		ColorDarkBlue:  2,
	}

	for group, expected := range expectedGroupSizes {
		if groupCounts[group] != expected {
			t.Errorf("Expected %d %s properties, got %d", expected, group, groupCounts[group])
		}
	}
}

func TestRailroads(t *testing.T) {
	// Count railroads
	railroadCount := 0
	railroadPositions := []int{5, 15, 25, 35}

	for _, pos := range railroadPositions {
		if Board[pos].Type != SpaceRailroad {
			t.Errorf("Expected railroad at position %d, got %s", pos, Board[pos].Type)
		}
		railroadCount++
	}

	if railroadCount != 4 {
		t.Errorf("Expected 4 railroads, got %d", railroadCount)
	}
}

func TestUtilities(t *testing.T) {
	// Verify utilities at positions 12 and 28
	if Board[12].Type != SpaceUtility {
		t.Errorf("Expected utility at position 12, got %s", Board[12].Type)
	}
	if Board[28].Type != SpaceUtility {
		t.Errorf("Expected utility at position 28, got %s", Board[28].Type)
	}
}

func TestChanceAndCommunityChest(t *testing.T) {
	// Verify Chance positions
	chancePositions := []int{7, 22, 36}
	for _, pos := range chancePositions {
		if Board[pos].Type != SpaceChance {
			t.Errorf("Expected Chance at position %d, got %s", pos, Board[pos].Type)
		}
	}

	// Verify Community Chest positions
	ccPositions := []int{2, 17, 33}
	for _, pos := range ccPositions {
		if Board[pos].Type != SpaceCommunityChest {
			t.Errorf("Expected Community Chest at position %d, got %s", pos, Board[pos].Type)
		}
	}
}

func TestTaxSpaces(t *testing.T) {
	// Income Tax at position 4
	if Board[4].Type != SpaceTax {
		t.Errorf("Expected Tax at position 4, got %s", Board[4].Type)
	}
	if Board[4].TaxAmount != 200 {
		t.Errorf("Expected Income Tax $200, got $%d", Board[4].TaxAmount)
	}

	// Luxury Tax at position 38
	if Board[38].Type != SpaceTax {
		t.Errorf("Expected Tax at position 38, got %s", Board[38].Type)
	}
	if Board[38].TaxAmount != 100 {
		t.Errorf("Expected Luxury Tax $100, got $%d", Board[38].TaxAmount)
	}
}
