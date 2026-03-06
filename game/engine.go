package game

import (
	"database/sql"
	"encoding/json"
	"math/rand"
	"monopoly/errors"
	"monopoly/store"
)

type Engine struct {
	store          store.GameStore
	doublesCount   map[int64]int      // gameID -> count of consecutive doubles this turn
	activeAuctions map[int64]*Auction // gameID -> active auction (nil if no auction in progress)
}

func NewEngine(store store.GameStore) *Engine {
	return &Engine{
		store:          store,
		doublesCount:   make(map[int64]int),
		activeAuctions: make(map[int64]*Auction),
	}
}

func (e *Engine) GetGameState(gameID int64) (*GameState, error) {
	if gameID <= 0 {
		return nil, errors.GameNotFound()
	}

	game, err := e.store.GetGame(gameID)
	if err != nil {
		return nil, err
	}
	if game == nil {
		return nil, errors.GameNotFound()
	}

	players, err := e.store.GetGamePlayers(gameID)
	if err != nil {
		return nil, err
	}

	gamePlayers := make([]*Player, len(players))
	var currentPlayerID int64
	for i, p := range players {
		gamePlayers[i] = &Player{
			UserID:        p.UserID,
			Username:      p.Username,
			Order:         p.PlayerOrder,
			IsReady:       p.IsReady,
			IsCurrentTurn: p.IsCurrentTurn,
			Money:         p.Money,
			Position:      p.Position,
			IsBankrupt:    p.IsBankrupt,
			HasRolled:     p.HasRolled,
			PendingAction: p.PendingAction,
			InJail:        p.InJail,
			JailTurns:     p.JailTurns,
		}
		if p.IsCurrentTurn {
			currentPlayerID = p.UserID
		}
	}

	props, err := e.store.GetGameProperties(gameID)
	if err != nil {
		return nil, err
	}
	properties := make(map[int]int64)
	mortgagedProperties := make(map[int]bool)
	for _, p := range props {
		properties[p.Position] = p.OwnerID
		if p.IsMortgaged {
			mortgagedProperties[p.Position] = true
		}
	}

	improvements, err := e.store.GetAllImprovements(gameID)
	if err != nil {
		return nil, err
	}

	return &GameState{
		ID:                  game.ID,
		Status:              game.Status,
		Players:             gamePlayers,
		CurrentPlayerID:     currentPlayerID,
		MaxPlayers:          game.MaxPlayers,
		Properties:          properties,
		MortgagedProperties: mortgagedProperties,
		Improvements:        improvements,
		Board:               Board,
	}, nil
}

func (e *Engine) JoinGame(gameID, userID int64, username string) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusWaiting {
		return nil, errors.GameAlreadyStarted()
	}

	if len(state.Players) >= state.MaxPlayers {
		return nil, errors.GameFull()
	}

	for _, p := range state.Players {
		if p.UserID == userID {
			return nil, errors.AlreadyInGame()
		}
	}

	playerOrder := len(state.Players)
	if err := e.store.JoinGame(gameID, userID, playerOrder); err != nil {
		return nil, err
	}

	newPlayer := &Player{
		UserID:   userID,
		Username: username,
		Order:    playerOrder,
		IsReady:  false,
		Money:    1500,
	}

	return &Event{
		Type:   "player_joined",
		GameID: gameID,
		Payload: PlayerJoinedPayload{
			Player: newPlayer,
		},
	}, nil
}

func (e *Engine) SetReady(gameID, userID int64, isReady bool) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusWaiting {
		return nil, errors.GameAlreadyStarted()
	}

	found := false
	for _, p := range state.Players {
		if p.UserID == userID {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.NotInGame()
	}

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	if err := e.store.UpdatePlayerReadyTx(tx, gameID, userID, isReady); err != nil {
		return nil, err
	}

	state, err = e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if len(state.Players) >= 2 {
		allReady := true
		for _, p := range state.Players {
			if !p.IsReady {
				allReady = false
				break
			}
		}

		if allReady {
			if err := e.store.UpdateGameStatusTx(tx, gameID, StatusInProgress); err != nil {
				return nil, err
			}

			firstPlayer := state.Players[0]
			if err := e.store.UpdateCurrentTurnTx(tx, gameID, firstPlayer.UserID); err != nil {
				return nil, err
			}

			if err := e.store.CommitTx(tx); err != nil {
				return nil, err
			}

			// Initialize card decks
			chanceOrder := ShuffleDeck(len(ChanceCards))
			communityOrder := ShuffleDeck(len(CommunityChestCards))
			if err := e.store.InitializeDecks(gameID, chanceOrder, communityOrder); err != nil {
				// Non-fatal, game can continue without cards
			}

			return &Event{
				Type:   "game_started",
				GameID: gameID,
				Payload: GameStartedPayload{
					CurrentPlayerID: firstPlayer.UserID,
				},
			}, nil
		}
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return &Event{
		Type:   "player_ready",
		GameID: gameID,
		Payload: PlayerReadyPayload{
			UserID:  userID,
			IsReady: isReady,
		},
	}, nil
}

func (e *Engine) StartGameIfFull(gameID int64) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusWaiting || len(state.Players) < state.MaxPlayers {
		return nil, nil
	}

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	if err := e.store.UpdateGameStatusTx(tx, gameID, StatusInProgress); err != nil {
		return nil, err
	}

	firstPlayer := state.Players[0]
	if err := e.store.UpdateCurrentTurnTx(tx, gameID, firstPlayer.UserID); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	// Initialize card decks
	chanceOrder := ShuffleDeck(len(ChanceCards))
	communityOrder := ShuffleDeck(len(CommunityChestCards))
	if err := e.store.InitializeDecks(gameID, chanceOrder, communityOrder); err != nil {
		// Non-fatal, game can continue without cards
	}

	return &Event{
		Type:   "game_started",
		GameID: gameID,
		Payload: GameStartedPayload{
			CurrentPlayerID: firstPlayer.UserID,
		},
	}, nil
}

func (e *Engine) RollDice(gameID, userID int64) ([]*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	if state.CurrentPlayerID != userID {
		return nil, errors.NotYourTurn()
	}

	var currentPlayer *Player
	for _, p := range state.Players {
		if p.UserID == userID {
			currentPlayer = p
			break
		}
	}

	if currentPlayer == nil {
		return nil, errors.NotInGame()
	}

	if currentPlayer.IsBankrupt {
		return nil, errors.PlayerBankrupt()
	}

	if currentPlayer.HasRolled {
		return nil, errors.AlreadyRolled()
	}

	if currentPlayer.PendingAction != "" {
		return nil, errors.PendingAction()
	}

	die1 := rand.Intn(6) + 1
	die2 := rand.Intn(6) + 1
	total := die1 + die2
	isDoubles := die1 == die2

	// Handle jail logic first
	if currentPlayer.InJail {
		return e.rollDiceInJail(gameID, userID, currentPlayer, die1, die2, total, isDoubles)
	}

	// Track consecutive doubles (only when not in jail)
	if isDoubles {
		e.doublesCount[gameID]++
	} else {
		e.doublesCount[gameID] = 0
	}
	doublesCount := e.doublesCount[gameID]

	// Three doubles in a row - go to jail
	if doublesCount >= 3 {
		e.doublesCount[gameID] = 0 // Reset for next turn

		tx, err := e.store.BeginTx()
		if err != nil {
			return nil, err
		}
		defer e.store.RollbackTx(tx)

		if err := e.store.UpdatePlayerPositionTx(tx, gameID, userID, 10); err != nil {
			return nil, err
		}
		if err := e.store.SetPlayerInJailTx(tx, gameID, userID, true, 0); err != nil {
			return nil, err
		}
		if err := e.store.SetPlayerHasRolledTx(tx, gameID, userID, true); err != nil {
			return nil, err
		}

		if err := e.store.CommitTx(tx); err != nil {
			return nil, err
		}

		return []*Event{
			{
				Type:   "dice_rolled",
				GameID: gameID,
				Payload: DiceRolledPayload{
					UserID:       userID,
					Die1:         die1,
					Die2:         die2,
					Total:        total,
					OldPos:       currentPlayer.Position,
					NewPos:       10,
					PassedGo:     false,
					SpaceName:    "Jail",
					SpaceType:    string(SpaceJail),
					IsDoubles:    true,
					DoublesCount: doublesCount,
				},
			},
			{
				Type:   "go_to_jail",
				GameID: gameID,
				Payload: GoToJailPayload{
					UserID: userID,
					OldPos: currentPlayer.Position,
					Reason: "three_doubles",
				},
			},
		}, nil
	}

	oldPos := currentPlayer.Position
	newPos := (oldPos + total) % 40
	passedGo := newPos < oldPos

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	player, err := e.store.GetPlayerTx(tx, gameID, userID)
	if err != nil {
		return nil, err
	}
	if player == nil {
		return nil, errors.NotInGame()
	}

	if err := e.store.UpdatePlayerPositionTx(tx, gameID, userID, newPos); err != nil {
		return nil, err
	}

	// If doubles, player can roll again (don't mark as rolled)
	// But if there's a pending action from landing, they must complete it first
	if err := e.store.SetPlayerHasRolledTx(tx, gameID, userID, !isDoubles); err != nil {
		return nil, err
	}

	currentMoney := player.Money
	if passedGo {
		currentMoney += 200
		if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, currentMoney); err != nil {
			return nil, err
		}
	}

	space := Board[newPos]

	var events []*Event
	events = append(events, &Event{
		Type:   "dice_rolled",
		GameID: gameID,
		Payload: DiceRolledPayload{
			UserID:       userID,
			Die1:         die1,
			Die2:         die2,
			Total:        total,
			OldPos:       oldPos,
			NewPos:       newPos,
			PassedGo:     passedGo,
			SpaceName:    space.Name,
			SpaceType:    string(space.Type),
			IsDoubles:    isDoubles,
			DoublesCount: doublesCount,
		},
	})

	resolutionEvents, err := e.resolveSpaceLanding(tx, gameID, userID, player.Username, currentMoney, space, total, 1.0)
	if err != nil {
		return nil, err
	}
	events = append(events, resolutionEvents...)

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return events, nil
}

// rollDiceInJail handles dice rolling when a player is in jail
func (e *Engine) rollDiceInJail(gameID, userID int64, player *Player, die1, die2, total int, isDoubles bool) ([]*Event, error) {
	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	dbPlayer, err := e.store.GetPlayerTx(tx, gameID, userID)
	if err != nil {
		return nil, err
	}
	if dbPlayer == nil {
		return nil, errors.NotInGame()
	}

	var events []*Event

	if isDoubles {
		// Escaped jail by rolling doubles! Move normally
		if err := e.store.ReleaseFromJailTx(tx, gameID, userID); err != nil {
			return nil, err
		}

		oldPos := player.Position
		newPos := (oldPos + total) % 40
		passedGo := newPos < oldPos

		if err := e.store.UpdatePlayerPositionTx(tx, gameID, userID, newPos); err != nil {
			return nil, err
		}
		if err := e.store.SetPlayerHasRolledTx(tx, gameID, userID, true); err != nil {
			return nil, err
		}

		currentMoney := dbPlayer.Money
		if passedGo {
			currentMoney += 200
			if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, currentMoney); err != nil {
				return nil, err
			}
		}

		space := Board[newPos]

		events = append(events, &Event{
			Type:   "jail_escape",
			GameID: gameID,
			Payload: JailEscapePayload{
				UserID: userID,
				Method: "doubles",
			},
		})

		events = append(events, &Event{
			Type:   "dice_rolled",
			GameID: gameID,
			Payload: DiceRolledPayload{
				UserID:       userID,
				Die1:         die1,
				Die2:         die2,
				Total:        total,
				OldPos:       oldPos,
				NewPos:       newPos,
				PassedGo:     passedGo,
				SpaceName:    space.Name,
				SpaceType:    string(space.Type),
				IsDoubles:    true,
				DoublesCount: 0, // Reset doubles count after leaving jail
			},
		})

		// Resolve landing
		resolutionEvents, err := e.resolveSpaceLanding(tx, gameID, userID, dbPlayer.Username, currentMoney, space, total, 1.0)
		if err != nil {
			return nil, err
		}
		events = append(events, resolutionEvents...)
	} else {
		// Failed to escape
		newJailTurns := dbPlayer.JailTurns + 1

		if newJailTurns >= 3 {
			// 3rd failed roll - forced to pay $50 bail
			bailAmount := 50
			if dbPlayer.Money < bailAmount {
				// Bankrupt from jail bail
				bankruptEvents, err := e.handleBankruptcyTx(tx, gameID, userID, dbPlayer.Username, "jail_bail", 0)
				if err != nil {
					return nil, err
				}
				if err := e.store.CommitTx(tx); err != nil {
					return nil, err
				}

				events = append(events, &Event{
					Type:   "jail_roll_failed",
					GameID: gameID,
					Payload: JailRollFailedPayload{
						UserID:     userID,
						Die1:       die1,
						Die2:       die2,
						JailTurns:  newJailTurns,
						ForcedBail: true,
						NewMoney:   0,
					},
				})
				events = append(events, bankruptEvents...)
				return events, nil
			}

			newMoney := dbPlayer.Money - bailAmount
			if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
				return nil, err
			}
			if err := e.store.ReleaseFromJailTx(tx, gameID, userID); err != nil {
				return nil, err
			}

			// Move normally after forced bail
			oldPos := player.Position
			newPos := (oldPos + total) % 40
			passedGo := newPos < oldPos

			if err := e.store.UpdatePlayerPositionTx(tx, gameID, userID, newPos); err != nil {
				return nil, err
			}
			if err := e.store.SetPlayerHasRolledTx(tx, gameID, userID, true); err != nil {
				return nil, err
			}

			currentMoney := newMoney
			if passedGo {
				currentMoney += 200
				if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, currentMoney); err != nil {
					return nil, err
				}
			}

			space := Board[newPos]

			events = append(events, &Event{
				Type:   "jail_roll_failed",
				GameID: gameID,
				Payload: JailRollFailedPayload{
					UserID:     userID,
					Die1:       die1,
					Die2:       die2,
					JailTurns:  newJailTurns,
					ForcedBail: true,
					NewMoney:   currentMoney,
				},
			})

			events = append(events, &Event{
				Type:   "jail_escape",
				GameID: gameID,
				Payload: JailEscapePayload{
					UserID:   userID,
					Method:   "bail",
					NewMoney: currentMoney,
				},
			})

			events = append(events, &Event{
				Type:   "dice_rolled",
				GameID: gameID,
				Payload: DiceRolledPayload{
					UserID:       userID,
					Die1:         die1,
					Die2:         die2,
					Total:        total,
					OldPos:       oldPos,
					NewPos:       newPos,
					PassedGo:     passedGo,
					SpaceName:    space.Name,
					SpaceType:    string(space.Type),
					IsDoubles:    false,
					DoublesCount: 0,
				},
			})

			// Resolve landing
			resolutionEvents, err := e.resolveSpaceLanding(tx, gameID, userID, dbPlayer.Username, currentMoney, space, total, 1.0)
			if err != nil {
				return nil, err
			}
			events = append(events, resolutionEvents...)
		} else {
			// Still in jail, just increment turns
			if err := e.store.IncrementJailTurnsTx(tx, gameID, userID); err != nil {
				return nil, err
			}
			if err := e.store.SetPlayerHasRolledTx(tx, gameID, userID, true); err != nil {
				return nil, err
			}

			events = append(events, &Event{
				Type:   "jail_roll_failed",
				GameID: gameID,
				Payload: JailRollFailedPayload{
					UserID:     userID,
					Die1:       die1,
					Die2:       die2,
					JailTurns:  newJailTurns,
					ForcedBail: false,
				},
			})
		}
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return events, nil
}

// UseJailFreeCard allows a player to use a Get Out of Jail Free card
func (e *Engine) UseJailFreeCard(gameID, userID int64) ([]*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	if state.CurrentPlayerID != userID {
		return nil, errors.NotYourTurn()
	}

	var currentPlayer *Player
	for _, p := range state.Players {
		if p.UserID == userID {
			currentPlayer = p
			break
		}
	}

	if currentPlayer == nil {
		return nil, errors.NotInGame()
	}

	if !currentPlayer.InJail {
		return nil, errors.NotInJail()
	}

	if currentPlayer.HasRolled {
		return nil, errors.AlreadyRolled()
	}

	// Check if player has a jail card
	hasCard, _, err := e.store.HasJailCard(gameID, userID)
	if err != nil {
		return nil, err
	}
	if !hasCard {
		return nil, errors.BadRequest("You don't have a Get Out of Jail Free card")
	}

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	// Use the card
	deckType, err := e.store.UseJailCardTx(tx, gameID, userID)
	if err != nil {
		return nil, err
	}

	// Release from jail
	if err := e.store.ReleaseFromJailTx(tx, gameID, userID); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return []*Event{
		{
			Type:   "jail_escape",
			GameID: gameID,
			Payload: JailEscapePayload{
				UserID: userID,
				Method: "card",
			},
		},
		{
			Type:   "card_used",
			GameID: gameID,
			Payload: map[string]interface{}{
				"userId":   userID,
				"cardType": "get_out_of_jail",
				"deckType": deckType,
			},
		},
	}, nil
}

// PayJailBail allows a player to pay $50 to get out of jail before rolling
func (e *Engine) PayJailBail(gameID, userID int64) ([]*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	if state.CurrentPlayerID != userID {
		return nil, errors.NotYourTurn()
	}

	var currentPlayer *Player
	for _, p := range state.Players {
		if p.UserID == userID {
			currentPlayer = p
			break
		}
	}

	if currentPlayer == nil {
		return nil, errors.NotInGame()
	}

	if !currentPlayer.InJail {
		return nil, errors.NotInJail()
	}

	if currentPlayer.HasRolled {
		return nil, errors.AlreadyRolled()
	}

	bailAmount := 50
	if currentPlayer.Money < bailAmount {
		return nil, errors.InsufficientFunds()
	}

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	newMoney := currentPlayer.Money - bailAmount
	if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
		return nil, err
	}
	if err := e.store.ReleaseFromJailTx(tx, gameID, userID); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return []*Event{
		{
			Type:   "jail_escape",
			GameID: gameID,
			Payload: JailEscapePayload{
				UserID:   userID,
				Method:   "bail",
				NewMoney: newMoney,
			},
		},
	}, nil
}

// MortgageProperty allows a player to mortgage a property they own
func (e *Engine) MortgageProperty(gameID, userID int64, position int) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	// Verify ownership
	ownerID, ok := state.Properties[position]
	if !ok || ownerID != userID {
		return nil, errors.PropertyNotOwned()
	}

	// Check if already mortgaged
	if state.MortgagedProperties[position] {
		return nil, errors.PropertyAlreadyMortgaged()
	}

	space := Board[position]
	mortgageValue := space.Price / 2

	var player *Player
	for _, p := range state.Players {
		if p.UserID == userID {
			player = p
			break
		}
	}
	if player == nil {
		return nil, errors.NotInGame()
	}

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	if err := e.store.SetPropertyMortgagedTx(tx, gameID, position, true); err != nil {
		return nil, err
	}

	newMoney := player.Money + mortgageValue
	if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return &Event{
		Type:   "property_mortgaged",
		GameID: gameID,
		Payload: PropertyMortgagedPayload{
			UserID:   userID,
			Position: position,
			Name:     space.Name,
			Amount:   mortgageValue,
			NewMoney: newMoney,
		},
	}, nil
}

// UnmortgageProperty allows a player to unmortgage a property by paying 110% of mortgage value
func (e *Engine) UnmortgageProperty(gameID, userID int64, position int) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	// Verify ownership
	ownerID, ok := state.Properties[position]
	if !ok || ownerID != userID {
		return nil, errors.PropertyNotOwned()
	}

	// Check if mortgaged
	if !state.MortgagedProperties[position] {
		return nil, errors.PropertyNotMortgaged()
	}

	space := Board[position]
	mortgageValue := space.Price / 2
	unmortgageCost := mortgageValue + (mortgageValue / 10) // 110% of mortgage value

	var player *Player
	for _, p := range state.Players {
		if p.UserID == userID {
			player = p
			break
		}
	}
	if player == nil {
		return nil, errors.NotInGame()
	}

	if player.Money < unmortgageCost {
		return nil, errors.InsufficientFunds()
	}

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	if err := e.store.SetPropertyMortgagedTx(tx, gameID, position, false); err != nil {
		return nil, err
	}

	newMoney := player.Money - unmortgageCost
	if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return &Event{
		Type:   "property_unmortgaged",
		GameID: gameID,
		Payload: PropertyUnmortgagedPayload{
			UserID:   userID,
			Position: position,
			Name:     space.Name,
			Amount:   unmortgageCost,
			NewMoney: newMoney,
		},
	}, nil
}

// BuyHouse allows a player to buy a house on a property they own
func (e *Engine) BuyHouse(gameID, userID int64, position int) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	// Verify ownership
	ownerID, ok := state.Properties[position]
	if !ok || ownerID != userID {
		return nil, errors.PropertyNotOwned()
	}

	space := Board[position]
	if space.Type != SpaceProperty {
		return nil, errors.BadRequest("Can only build on properties")
	}

	// Check if mortgaged
	if state.MortgagedProperties[position] {
		return nil, errors.PropertyAlreadyMortgaged()
	}

	// Check if player has monopoly
	colorCount := 0
	colorPositions := []int{}
	for pos, owner := range state.Properties {
		if owner == userID && Board[pos].Color == space.Color {
			colorCount++
			colorPositions = append(colorPositions, pos)
		}
	}
	if colorCount < space.GroupSize {
		return nil, errors.NoMonopoly()
	}

	// Check current improvements
	currentImpr := state.Improvements[position]
	if currentImpr >= 5 {
		return nil, errors.MaxImprovements()
	}

	// Check even build rule - new count can't be more than 1 above any other property in group
	for _, pos := range colorPositions {
		if pos != position {
			otherImpr := state.Improvements[pos]
			if currentImpr >= otherImpr+1 {
				return nil, errors.UnevenBuild()
			}
		}
	}

	// Check house/hotel supply (max 32 houses, 12 hotels)
	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	totalHouses, totalHotels, err := e.store.GetTotalHousesHotelsTx(tx, gameID)
	if err != nil {
		return nil, err
	}

	if currentImpr == 4 {
		// Building a hotel
		if totalHotels >= 12 {
			return nil, errors.HotelShortage()
		}
	} else {
		// Building a house
		if totalHouses >= 32 {
			return nil, errors.HouseShortage()
		}
	}

	// Check funds
	var player *Player
	for _, p := range state.Players {
		if p.UserID == userID {
			player = p
			break
		}
	}
	if player == nil {
		return nil, errors.NotInGame()
	}

	if player.Money < space.HouseCost {
		return nil, errors.InsufficientFunds()
	}

	// Build the house
	newImpr := currentImpr + 1
	if err := e.store.SetImprovementsTx(tx, gameID, position, newImpr); err != nil {
		return nil, err
	}

	newMoney := player.Money - space.HouseCost
	if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	eventType := "house_built"
	if newImpr == 5 {
		eventType = "hotel_built"
	}

	return &Event{
		Type:   eventType,
		GameID: gameID,
		Payload: HouseBuiltPayload{
			UserID:     userID,
			Position:   position,
			Name:       space.Name,
			HouseCount: newImpr,
			Cost:       space.HouseCost,
			NewMoney:   newMoney,
		},
	}, nil
}

// SellHouse allows a player to sell a house from a property they own
func (e *Engine) SellHouse(gameID, userID int64, position int) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	// Verify ownership
	ownerID, ok := state.Properties[position]
	if !ok || ownerID != userID {
		return nil, errors.PropertyNotOwned()
	}

	space := Board[position]
	if space.Type != SpaceProperty {
		return nil, errors.BadRequest("Can only sell houses from properties")
	}

	// Check current improvements
	currentImpr := state.Improvements[position]
	if currentImpr <= 0 {
		return nil, errors.NoImprovements()
	}

	// Check even build rule - can't sell if it would make this more than 1 below others
	colorPositions := []int{}
	for pos, owner := range state.Properties {
		if owner == userID && Board[pos].Color == space.Color {
			colorPositions = append(colorPositions, pos)
		}
	}

	for _, pos := range colorPositions {
		if pos != position {
			otherImpr := state.Improvements[pos]
			if currentImpr <= otherImpr-1 {
				return nil, errors.UnevenBuild()
			}
		}
	}

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	// If selling a hotel, check if 4 houses are available
	if currentImpr == 5 {
		houses, _, err := e.store.GetTotalHousesHotelsTx(tx, gameID)
		if err != nil {
			return nil, err
		}
		availableHouses := 32 - houses
		if availableHouses < 4 {
			return nil, errors.HouseShortage()
		}
	}

	// Sell the house (get half value back)
	newImpr := currentImpr - 1
	if err := e.store.SetImprovementsTx(tx, gameID, position, newImpr); err != nil {
		return nil, err
	}

	refund := space.HouseCost / 2
	var player *Player
	for _, p := range state.Players {
		if p.UserID == userID {
			player = p
			break
		}
	}
	if player == nil {
		return nil, errors.NotInGame()
	}

	newMoney := player.Money + refund
	if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return &Event{
		Type:   "house_sold",
		GameID: gameID,
		Payload: HouseSoldPayload{
			UserID:     userID,
			Position:   position,
			Name:       space.Name,
			HouseCount: newImpr,
			Refund:     refund,
			NewMoney:   newMoney,
		},
	}, nil
}

func (e *Engine) resolveSpaceLanding(tx *sql.Tx, gameID, userID int64, username string, currentMoney int, space BoardSpace, diceTotal int, rentMultiplier float64) ([]*Event, error) {
	var events []*Event

	switch space.Type {
	case SpaceProperty, SpaceRailroad, SpaceUtility:
		ownerID, err := e.store.GetPropertyOwnerTx(tx, gameID, space.Position)
		if err != nil {
			return nil, err
		}

		if ownerID == 0 {
			// Unowned - prompt to buy
			if currentMoney >= space.Price {
				if err := e.store.SetPlayerPendingActionTx(tx, gameID, userID, "buy_or_pass"); err != nil {
					return nil, err
				}
				events = append(events, &Event{
					Type:   "buy_prompt",
					GameID: gameID,
					Payload: BuyPromptPayload{
						UserID:   userID,
						Position: space.Position,
						Name:     space.Name,
						Price:    space.Price,
					},
				})
			}
			// If can't afford, nothing happens (no auction in MVP)
		} else if ownerID != userID {
			// Owned by someone else - check if mortgaged first
			prop, err := e.store.GetPropertyTx(tx, gameID, space.Position)
			if err != nil {
				return nil, err
			}
			if prop != nil && prop.IsMortgaged {
				break // No rent on mortgaged properties
			}

			// Pay rent
			ownerProps, err := e.store.GetPlayerPropertiesTx(tx, gameID, ownerID)
			if err != nil {
				return nil, err
			}

			// Get improvements on this property
			improvements, err := e.store.GetImprovementsTx(tx, gameID, space.Position)
			if err != nil {
				return nil, err
			}

			rent := int(float64(CalculateRent(space, ownerProps, diceTotal, improvements)) * rentMultiplier)

			// Check if owner is bankrupt (shouldn't be, but safe check)
			owner, err := e.store.GetPlayerTx(tx, gameID, ownerID)
			if err != nil {
				return nil, err
			}
			if owner != nil && owner.IsBankrupt {
				break // Don't pay rent to bankrupt player
			}

			if currentMoney >= rent {
				// Can afford rent
				payerNewMoney := currentMoney - rent
				ownerNewMoney := owner.Money + rent

				if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, payerNewMoney); err != nil {
					return nil, err
				}
				if err := e.store.UpdatePlayerMoneyTx(tx, gameID, ownerID, ownerNewMoney); err != nil {
					return nil, err
				}

				events = append(events, &Event{
					Type:   "rent_paid",
					GameID: gameID,
					Payload: RentPaidPayload{
						PayerID:    userID,
						OwnerID:    ownerID,
						Position:   space.Position,
						Name:       space.Name,
						Amount:     rent,
						PayerMoney: payerNewMoney,
						OwnerMoney: ownerNewMoney,
					},
				})
			} else {
				// Can't afford rent - bankruptcy
				// Give whatever money they have to the owner
				ownerNewMoney := owner.Money + currentMoney
				if err := e.store.UpdatePlayerMoneyTx(tx, gameID, ownerID, ownerNewMoney); err != nil {
					return nil, err
				}

				bankruptEvents, err := e.handleBankruptcyTx(tx, gameID, userID, username, "rent", ownerID)
				if err != nil {
					return nil, err
				}
				events = append(events, bankruptEvents...)
			}
		}
		// If owned by self, nothing happens

	case SpaceTax:
		if currentMoney >= space.TaxAmount {
			newMoney := currentMoney - space.TaxAmount
			if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
				return nil, err
			}
			events = append(events, &Event{
				Type:   "tax_paid",
				GameID: gameID,
				Payload: TaxPaidPayload{
					UserID:   userID,
					Position: space.Position,
					Amount:   space.TaxAmount,
					NewMoney: newMoney,
				},
			})
		} else {
			// Can't afford tax - bankruptcy
			bankruptEvents, err := e.handleBankruptcyTx(tx, gameID, userID, username, "tax", 0)
			if err != nil {
				return nil, err
			}
			events = append(events, bankruptEvents...)
		}

	case SpaceGoToJail:
		if err := e.store.UpdatePlayerPositionTx(tx, gameID, userID, 10); err != nil {
			return nil, err
		}
		if err := e.store.SetPlayerInJailTx(tx, gameID, userID, true, 0); err != nil {
			return nil, err
		}
		events = append(events, &Event{
			Type:   "go_to_jail",
			GameID: gameID,
			Payload: GoToJailPayload{
				UserID: userID,
				OldPos: space.Position,
				Reason: "landed",
			},
		})

	case SpaceChance:
		cardEvents, err := e.drawAndExecuteCard(tx, gameID, userID, username, currentMoney, "chance", space.Position, diceTotal)
		if err != nil {
			return nil, err
		}
		events = append(events, cardEvents...)

	case SpaceCommunityChest:
		cardEvents, err := e.drawAndExecuteCard(tx, gameID, userID, username, currentMoney, "community", space.Position, diceTotal)
		if err != nil {
			return nil, err
		}
		events = append(events, cardEvents...)

	// Go, Jail (visiting), Free Parking - no-op
	}

	return events, nil
}

func (e *Engine) drawAndExecuteCard(tx *sql.Tx, gameID, userID int64, username string, currentMoney int, deckType string, currentPos int, diceTotal int) ([]*Event, error) {
	var events []*Event

	// Draw a card
	cardIndex, err := e.store.DrawCardTx(tx, gameID, deckType)
	if err != nil {
		// If deck not initialized, skip card effect
		return events, nil
	}

	var card Card
	if deckType == "chance" {
		if cardIndex < 0 || cardIndex >= len(ChanceCards) {
			return events, nil
		}
		card = ChanceCards[cardIndex]
	} else {
		if cardIndex < 0 || cardIndex >= len(CommunityChestCards) {
			return events, nil
		}
		card = CommunityChestCards[cardIndex]
	}

	var effect string
	var newMoney int = currentMoney
	var newPos int = currentPos
	var cardRentMultiplier float64 = 1.0 // Special rent multiplier for advance to nearest cards

	switch card.Type {
	case CardTypeCollectMoney:
		newMoney = currentMoney + card.Value
		if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
			return nil, err
		}
		effect = "Collected $" + itoa(card.Value)

	case CardTypePayMoney:
		if currentMoney >= card.Value {
			newMoney = currentMoney - card.Value
			if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
				return nil, err
			}
			effect = "Paid $" + itoa(card.Value)
		} else {
			// Bankruptcy
			bankruptEvents, err := e.handleBankruptcyTx(tx, gameID, userID, username, "card", 0)
			if err != nil {
				return nil, err
			}
			events = append(events, &Event{
				Type:   "card_drawn",
				GameID: gameID,
				Payload: CardDrawnPayload{
					UserID:   userID,
					DeckType: deckType,
					CardText: card.Text,
					CardType: string(card.Type),
					Effect:   "Couldn't pay - bankruptcy!",
					NewMoney: 0,
				},
			})
			events = append(events, bankruptEvents...)
			return events, nil
		}

	case CardTypeMoveTo:
		newPos = card.Destination
		passedGo := newPos < currentPos && card.Value > 0 // Some MoveTo cards give $200 for passing GO
		if passedGo {
			newMoney += 200
			if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
				return nil, err
			}
		}
		if err := e.store.UpdatePlayerPositionTx(tx, gameID, userID, newPos); err != nil {
			return nil, err
		}
		effect = "Moved to " + Board[newPos].Name

	case CardTypeMoveBack:
		newPos = (currentPos - card.Value + 40) % 40
		if err := e.store.UpdatePlayerPositionTx(tx, gameID, userID, newPos); err != nil {
			return nil, err
		}
		effect = "Moved back " + itoa(card.Value) + " spaces to " + Board[newPos].Name

	case CardTypeGoToJail:
		newPos = 10
		if err := e.store.UpdatePlayerPositionTx(tx, gameID, userID, 10); err != nil {
			return nil, err
		}
		if err := e.store.SetPlayerInJailTx(tx, gameID, userID, true, 0); err != nil {
			return nil, err
		}
		effect = "Sent to Jail!"
		events = append(events, &Event{
			Type:   "card_drawn",
			GameID: gameID,
			Payload: CardDrawnPayload{
				UserID:   userID,
				DeckType: deckType,
				CardText: card.Text,
				CardType: string(card.Type),
				Effect:   effect,
				NewMoney: newMoney,
				NewPos:   newPos,
			},
		})
		events = append(events, &Event{
			Type:   "go_to_jail",
			GameID: gameID,
			Payload: GoToJailPayload{
				UserID: userID,
				OldPos: currentPos,
				Reason: "card",
			},
		})
		return events, nil

	case CardTypeGetOutOfJail:
		if err := e.store.GiveJailCardTx(tx, gameID, userID, deckType); err != nil {
			return nil, err
		}
		effect = "Received Get Out of Jail Free card"

	case CardTypeRepairs:
		// Calculate repair costs
		houses, hotels, err := e.store.GetTotalHousesHotelsTx(tx, gameID)
		if err != nil {
			return nil, err
		}
		// Only count player's own improvements
		props, err := e.store.GetPlayerPropertiesTx(tx, gameID, userID)
		if err != nil {
			return nil, err
		}
		playerHouses, playerHotels := 0, 0
		for _, pos := range props {
			impr, _ := e.store.GetImprovementsTx(tx, gameID, pos)
			if impr == 5 {
				playerHotels++
			} else {
				playerHouses += impr
			}
		}
		_ = houses // unused, we only care about player's
		_ = hotels
		cost := (playerHouses * card.Value) + (playerHotels * card.Value2)
		if currentMoney >= cost {
			newMoney = currentMoney - cost
			if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
				return nil, err
			}
			effect = "Paid $" + itoa(cost) + " for repairs"
		} else {
			// Bankruptcy
			bankruptEvents, err := e.handleBankruptcyTx(tx, gameID, userID, username, "card", 0)
			if err != nil {
				return nil, err
			}
			events = append(events, &Event{
				Type:   "card_drawn",
				GameID: gameID,
				Payload: CardDrawnPayload{
					UserID:   userID,
					DeckType: deckType,
					CardText: card.Text,
					CardType: string(card.Type),
					Effect:   "Couldn't pay $" + itoa(cost) + " - bankruptcy!",
					NewMoney: 0,
				},
			})
			events = append(events, bankruptEvents...)
			return events, nil
		}

	case CardTypePayEachPlayer, CardTypeCollectFromEach:
		// Get all active players
		activePlayers, err := e.store.GetActivePlayersTx(tx, gameID)
		if err != nil {
			return nil, err
		}
		otherCount := 0
		for _, p := range activePlayers {
			if p.UserID != userID {
				otherCount++
			}
		}
		totalAmount := card.Value * otherCount

		if card.Type == CardTypePayEachPlayer {
			if currentMoney >= totalAmount {
				newMoney = currentMoney - totalAmount
				if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
					return nil, err
				}
				// Give money to each player
				for _, p := range activePlayers {
					if p.UserID != userID {
						pMoney := p.Money + card.Value
						if err := e.store.UpdatePlayerMoneyTx(tx, gameID, p.UserID, pMoney); err != nil {
							return nil, err
						}
					}
				}
				effect = "Paid $" + itoa(card.Value) + " to each player (total: $" + itoa(totalAmount) + ")"
			} else {
				// Bankruptcy
				bankruptEvents, err := e.handleBankruptcyTx(tx, gameID, userID, username, "card", 0)
				if err != nil {
					return nil, err
				}
				events = append(events, &Event{
					Type:   "card_drawn",
					GameID: gameID,
					Payload: CardDrawnPayload{
						UserID:   userID,
						DeckType: deckType,
						CardText: card.Text,
						CardType: string(card.Type),
						Effect:   "Couldn't pay - bankruptcy!",
						NewMoney: 0,
					},
				})
				events = append(events, bankruptEvents...)
				return events, nil
			}
		} else {
			// Collect from each player
			collected := 0
			for _, p := range activePlayers {
				if p.UserID != userID {
					toCollect := card.Value
					if p.Money < toCollect {
						toCollect = p.Money // Take what they have
					}
					pMoney := p.Money - toCollect
					if err := e.store.UpdatePlayerMoneyTx(tx, gameID, p.UserID, pMoney); err != nil {
						return nil, err
					}
					collected += toCollect
				}
			}
			newMoney = currentMoney + collected
			if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
				return nil, err
			}
			effect = "Collected $" + itoa(collected) + " from all players"
		}

	case CardTypeAdvanceToNearest:
		// Find nearest railroad or utility
		if card.NearestType == "railroad" {
			railroads := []int{5, 15, 25, 35}
			minDist := 40
			nearest := railroads[0]
			for _, rr := range railroads {
				dist := (rr - currentPos + 40) % 40
				if dist == 0 {
					dist = 40
				}
				if dist < minDist {
					minDist = dist
					nearest = rr
				}
			}
			newPos = nearest
			cardRentMultiplier = 2.0 // "Pay owner twice normal rent" per card text
		} else if card.NearestType == "utility" {
			// Electric Company at 12, Water Works at 28
			if (12-currentPos+40)%40 < (28-currentPos+40)%40 {
				newPos = 12
			} else {
				newPos = 28
			}
			cardRentMultiplier = 2.5 // "Pay 10x dice" instead of normal 4x = 2.5x multiplier
		}
		passedGo := newPos < currentPos
		if passedGo {
			newMoney += 200
			if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
				return nil, err
			}
		}
		if err := e.store.UpdatePlayerPositionTx(tx, gameID, userID, newPos); err != nil {
			return nil, err
		}
		effect = "Advanced to " + Board[newPos].Name
	}

	events = append(events, &Event{
		Type:   "card_drawn",
		GameID: gameID,
		Payload: CardDrawnPayload{
			UserID:   userID,
			DeckType: deckType,
			CardText: card.Text,
			CardType: string(card.Type),
			Effect:   effect,
			NewMoney: newMoney,
			NewPos:   newPos,
		},
	})

	// If player moved to a new space, resolve that landing
	if newPos != currentPos && card.Type != CardTypeGoToJail {
		landingSpace := Board[newPos]
		landingEvents, err := e.resolveSpaceLanding(tx, gameID, userID, username, newMoney, landingSpace, diceTotal, cardRentMultiplier)
		if err != nil {
			return nil, err
		}
		events = append(events, landingEvents...)
	}

	return events, nil
}

// Helper function for int to string conversion
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	if negative {
		result = "-" + result
	}
	return result
}

func (e *Engine) handleBankruptcyTx(tx *sql.Tx, gameID, userID int64, username, reason string, creditorID int64) ([]*Event, error) {
	var events []*Event

	// Mark player bankrupt
	if err := e.store.SetPlayerBankruptTx(tx, gameID, userID); err != nil {
		return nil, err
	}

	// Handle properties based on creditor
	if creditorID != 0 {
		// Transfer all properties to the creditor (mortgaged properties transfer as-is)
		if err := e.store.TransferAllPropertiesTx(tx, gameID, userID, creditorID); err != nil {
			return nil, err
		}
	} else {
		// Bankrupt to bank - properties become unowned
		if err := e.store.DeletePlayerPropertiesTx(tx, gameID, userID); err != nil {
			return nil, err
		}
	}

	events = append(events, &Event{
		Type:   "player_bankrupt",
		GameID: gameID,
		Payload: PlayerBankruptPayload{
			UserID:     userID,
			Username:   username,
			Reason:     reason,
			CreditorID: creditorID,
		},
	})

	// Check if only 1 active player remains
	activeCount, err := e.store.CountActivePlayersTx(tx, gameID)
	if err != nil {
		return nil, err
	}

	if activeCount <= 1 {
		if err := e.store.UpdateGameStatusTx(tx, gameID, StatusFinished); err != nil {
			return nil, err
		}

		activePlayers, err := e.store.GetActivePlayersTx(tx, gameID)
		if err != nil {
			return nil, err
		}

		// Get all players for final state
		allPlayers, err := e.store.GetGamePlayers(gameID)
		if err != nil {
			return nil, err
		}

		var winnerID int64
		if len(activePlayers) > 0 {
			winnerID = activePlayers[0].UserID
		}

		finalPlayers := make([]*Player, len(allPlayers))
		for i, p := range allPlayers {
			finalPlayers[i] = &Player{
				UserID:     p.UserID,
				Username:   p.Username,
				Order:      p.PlayerOrder,
				Money:      p.Money,
				Position:   p.Position,
				IsBankrupt: p.IsBankrupt,
			}
		}

		events = append(events, &Event{
			Type:   "game_finished",
			GameID: gameID,
			Payload: GameFinishedPayload{
				Players:  finalPlayers,
				WinnerID: winnerID,
			},
		})
	}

	return events, nil
}

func (e *Engine) BuyProperty(gameID, userID int64) (*Event, error) {
	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	player, err := e.store.GetPlayerTx(tx, gameID, userID)
	if err != nil {
		return nil, err
	}
	if player == nil {
		return nil, errors.NotInGame()
	}

	if player.PendingAction != "buy_or_pass" {
		return nil, errors.CannotBuy()
	}

	space := Board[player.Position]
	if player.Money < space.Price {
		return nil, errors.InsufficientFunds()
	}

	newMoney := player.Money - space.Price
	if err := e.store.UpdatePlayerMoneyTx(tx, gameID, userID, newMoney); err != nil {
		return nil, err
	}

	if err := e.store.InsertPropertyTx(tx, gameID, player.Position, userID); err != nil {
		return nil, err
	}

	if err := e.store.SetPlayerPendingActionTx(tx, gameID, userID, ""); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return &Event{
		Type:   "property_bought",
		GameID: gameID,
		Payload: PropertyBoughtPayload{
			UserID:   userID,
			Position: player.Position,
			Name:     space.Name,
			Price:    space.Price,
			NewMoney: newMoney,
		},
	}, nil
}

func (e *Engine) PassProperty(gameID, userID int64) ([]*Event, error) {
	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	player, err := e.store.GetPlayerTx(tx, gameID, userID)
	if err != nil {
		return nil, err
	}
	if player == nil {
		return nil, errors.NotInGame()
	}

	if player.PendingAction != "buy_or_pass" {
		return nil, errors.CannotBuy()
	}

	// Check if an auction is already in progress
	if e.activeAuctions[gameID] != nil {
		return nil, errors.AuctionInProgress()
	}

	// Set pending action to auction (player who passed needs to wait for auction)
	if err := e.store.SetPlayerPendingActionTx(tx, gameID, userID, "auction"); err != nil {
		return nil, err
	}

	// Get all active players for auction bidder order
	activePlayers, err := e.store.GetActivePlayersTx(tx, gameID)
	if err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	space := Board[player.Position]
	events := []*Event{
		{
			Type:   "property_passed",
			GameID: gameID,
			Payload: PropertyPassedPayload{
				UserID:   userID,
				Position: player.Position,
				Name:     space.Name,
			},
		},
	}

	// Build bidder order starting from player who passed, going in player order
	bidderOrder := make([]int64, 0, len(activePlayers))
	passedIdx := -1
	for i, p := range activePlayers {
		if p.UserID == userID {
			passedIdx = i
			break
		}
	}

	// Start from player after the one who passed
	for i := 0; i < len(activePlayers); i++ {
		idx := (passedIdx + 1 + i) % len(activePlayers)
		bidderOrder = append(bidderOrder, activePlayers[idx].UserID)
	}

	// Create auction
	auction := &Auction{
		GameID:          gameID,
		Position:        player.Position,
		PropertyName:    space.Name,
		HighestBid:      0,
		HighestBidderID: 0,
		BidderOrder:     bidderOrder,
		CurrentBidder:   0,
		PassedBidders:   make(map[int64]bool),
	}
	e.activeAuctions[gameID] = auction

	// Emit auction_started event
	events = append(events, &Event{
		Type:   "auction_started",
		GameID: gameID,
		Payload: AuctionStartedPayload{
			Position:      player.Position,
			PropertyName:  space.Name,
			StartingBid:   1,
			BidderOrder:   bidderOrder,
			CurrentBidder: bidderOrder[0],
		},
	})

	return events, nil
}

// PlaceBid allows a player to place a bid in the current auction
func (e *Engine) PlaceBid(gameID, userID int64, amount int) ([]*Event, error) {
	auction := e.activeAuctions[gameID]
	if auction == nil {
		return nil, errors.NoAuction()
	}

	// Check if it's this player's turn to bid
	if auction.BidderOrder[auction.CurrentBidder] != userID {
		return nil, errors.NotYourBid()
	}

	// Check if bid is valid
	if amount <= auction.HighestBid {
		return nil, errors.BidTooLow()
	}

	// Check if player has enough money
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	var bidderMoney int
	var bidderName string
	for _, p := range state.Players {
		if p.UserID == userID {
			bidderMoney = p.Money
			bidderName = p.Username
			break
		}
	}

	if amount > bidderMoney {
		return nil, errors.InsufficientFunds()
	}

	// Record bid
	auction.HighestBid = amount
	auction.HighestBidderID = userID

	// Move to next bidder
	nextBidderID := e.advanceAuctionBidder(auction)

	return []*Event{
		{
			Type:   "auction_bid",
			GameID: gameID,
			Payload: AuctionBidPayload{
				Position:     auction.Position,
				BidderID:     userID,
				BidderName:   bidderName,
				BidAmount:    amount,
				NextBidderID: nextBidderID,
			},
		},
	}, nil
}

// PassAuction allows a player to pass (exit) the current auction
func (e *Engine) PassAuction(gameID, userID int64) ([]*Event, error) {
	auction := e.activeAuctions[gameID]
	if auction == nil {
		return nil, errors.NoAuction()
	}

	// Check if it's this player's turn to bid
	if auction.BidderOrder[auction.CurrentBidder] != userID {
		return nil, errors.NotYourBid()
	}

	// Mark player as passed
	auction.PassedBidders[userID] = true

	// Get player name
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	var passerName string
	for _, p := range state.Players {
		if p.UserID == userID {
			passerName = p.Username
			break
		}
	}

	// Count remaining bidders
	remaining := 0
	for _, bidderID := range auction.BidderOrder {
		if !auction.PassedBidders[bidderID] {
			remaining++
		}
	}

	var events []*Event

	// Check if auction should end (all but one passed, or everyone passed)
	if remaining <= 1 {
		// End auction
		return e.endAuction(gameID, auction, state)
	}

	// Move to next bidder
	nextBidderID := e.advanceAuctionBidder(auction)

	events = append(events, &Event{
		Type:   "auction_passed",
		GameID: gameID,
		Payload: AuctionPassedPayload{
			Position:       auction.Position,
			PasserID:       userID,
			PasserName:     passerName,
			NextBidderID:   nextBidderID,
			RemainingCount: remaining,
		},
	})

	return events, nil
}

// advanceAuctionBidder moves to the next active bidder in the auction
func (e *Engine) advanceAuctionBidder(auction *Auction) int64 {
	numBidders := len(auction.BidderOrder)
	for i := 1; i <= numBidders; i++ {
		nextIdx := (auction.CurrentBidder + i) % numBidders
		nextBidder := auction.BidderOrder[nextIdx]
		if !auction.PassedBidders[nextBidder] {
			auction.CurrentBidder = nextIdx
			return nextBidder
		}
	}
	return 0 // No valid bidder found (should not happen)
}

// endAuction finalizes the auction and transfers property to winner
func (e *Engine) endAuction(gameID int64, auction *Auction, state *GameState) ([]*Event, error) {
	var events []*Event

	// Clear the original player's pending action
	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	// Find player with auction pending action and clear it
	for _, p := range state.Players {
		if p.PendingAction == "auction" {
			if err := e.store.SetPlayerPendingActionTx(tx, gameID, p.UserID, ""); err != nil {
				return nil, err
			}
			break
		}
	}

	if auction.HighestBidderID != 0 && auction.HighestBid > 0 {
		// Winner exists - transfer property and deduct money
		winner, err := e.store.GetPlayerTx(tx, gameID, auction.HighestBidderID)
		if err != nil {
			return nil, err
		}

		// Deduct bid amount
		newMoney := winner.Money - auction.HighestBid
		if err := e.store.UpdatePlayerMoneyTx(tx, gameID, auction.HighestBidderID, newMoney); err != nil {
			return nil, err
		}

		// Transfer property
		if err := e.store.InsertPropertyTx(tx, gameID, auction.Position, auction.HighestBidderID); err != nil {
			return nil, err
		}

		if err := e.store.CommitTx(tx); err != nil {
			return nil, err
		}

		events = append(events, &Event{
			Type:   "auction_ended",
			GameID: gameID,
			Payload: AuctionEndedPayload{
				Position:     auction.Position,
				PropertyName: auction.PropertyName,
				WinnerID:     auction.HighestBidderID,
				WinnerName:   winner.Username,
				FinalBid:     auction.HighestBid,
				NoWinner:     false,
			},
		})
	} else {
		// No winner - everyone passed
		if err := e.store.CommitTx(tx); err != nil {
			return nil, err
		}

		events = append(events, &Event{
			Type:   "auction_ended",
			GameID: gameID,
			Payload: AuctionEndedPayload{
				Position:     auction.Position,
				PropertyName: auction.PropertyName,
				WinnerID:     0,
				WinnerName:   "",
				FinalBid:     0,
				NoWinner:     true,
			},
		})
	}

	// Remove auction
	delete(e.activeAuctions, gameID)

	return events, nil
}

// GetActiveAuction returns the active auction for a game, if any
func (e *Engine) GetActiveAuction(gameID int64) *Auction {
	return e.activeAuctions[gameID]
}

func (e *Engine) EndTurn(gameID, userID int64) (*Event, error) {
	return e.endTurnInternal(gameID, userID, false)
}

func (e *Engine) ForceEndTurn(gameID, userID int64) (*Event, error) {
	return e.endTurnInternal(gameID, userID, true)
}

// EliminatePlayerForTimeouts removes a player from the game due to consecutive timeouts
func (e *Engine) EliminatePlayerForTimeouts(gameID, userID int64) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	var player *Player
	for _, p := range state.Players {
		if p.UserID == userID {
			player = p
			break
		}
	}

	if player == nil {
		return nil, errors.NotInGame()
	}

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	// Mark player as bankrupt
	if err := e.store.SetPlayerBankruptTx(tx, gameID, userID); err != nil {
		return nil, err
	}

	// Release all their properties
	if err := e.store.DeletePlayerPropertiesTx(tx, gameID, userID); err != nil {
		return nil, err
	}

	// Clear pending action
	if err := e.store.SetPlayerPendingActionTx(tx, gameID, userID, ""); err != nil {
		return nil, err
	}

	// Check if game should end
	activePlayers, err := e.store.GetActivePlayersTx(tx, gameID)
	if err != nil {
		return nil, err
	}

	if len(activePlayers) <= 1 {
		// Game over - one player left
		if err := e.store.UpdateGameStatusTx(tx, gameID, StatusFinished); err != nil {
			return nil, err
		}

		if err := e.store.CommitTx(tx); err != nil {
			return nil, err
		}

		var winnerID int64
		if len(activePlayers) == 1 {
			winnerID = activePlayers[0].UserID
		}

		// Get final player states
		finalPlayers, _ := e.store.GetGamePlayers(gameID)
		resultPlayers := make([]*Player, len(finalPlayers))
		for i, p := range finalPlayers {
			resultPlayers[i] = &Player{
				UserID:     p.UserID,
				Username:   p.Username,
				Money:      p.Money,
				IsBankrupt: p.IsBankrupt,
			}
		}

		return &Event{
			Type:   "game_finished",
			GameID: gameID,
			Payload: GameFinishedPayload{
				Players:  resultPlayers,
				WinnerID: winnerID,
			},
		}, nil
	}

	// Find next player
	currentIdx := -1
	for i, p := range activePlayers {
		if p.UserID == userID {
			currentIdx = i
			break
		}
	}

	// Player was eliminated, so find next from remaining active players
	var nextPlayer *store.GamePlayer
	if currentIdx == -1 {
		// Player already not in active list, pick first active
		nextPlayer = activePlayers[0]
	} else {
		nextIdx := (currentIdx + 1) % len(activePlayers)
		nextPlayer = activePlayers[nextIdx]
	}

	// Reset doubles count
	e.doublesCount[gameID] = 0

	if err := e.store.ResetPlayerTurnStateTx(tx, gameID, nextPlayer.UserID); err != nil {
		return nil, err
	}

	if err := e.store.UpdateCurrentTurnTx(tx, gameID, nextPlayer.UserID); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return &Event{
		Type:   "turn_changed",
		GameID: gameID,
		Payload: TurnChangedPayload{
			PreviousPlayerID: userID,
			CurrentPlayerID:  nextPlayer.UserID,
		},
	}, nil
}

func (e *Engine) endTurnInternal(gameID, userID int64, force bool) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	if state.CurrentPlayerID != userID {
		return nil, errors.NotYourTurn()
	}

	// Reset doubles count when turn ends
	e.doublesCount[gameID] = 0

	var currentPlayer *Player
	for _, p := range state.Players {
		if p.UserID == userID {
			currentPlayer = p
			break
		}
	}

	if currentPlayer == nil {
		return nil, errors.NotInGame()
	}

	if !force {
		if !currentPlayer.HasRolled {
			return nil, errors.MustRoll()
		}
		if currentPlayer.PendingAction != "" {
			return nil, errors.PendingAction()
		}
	}

	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	if force {
		if err := e.store.SetPlayerPendingActionTx(tx, gameID, userID, ""); err != nil {
			return nil, err
		}
	}

	activePlayers, err := e.store.GetActivePlayersTx(tx, gameID)
	if err != nil {
		return nil, err
	}

	currentIdx := -1
	for i, p := range activePlayers {
		if p.UserID == userID {
			currentIdx = i
			break
		}
	}

	// If current player not found among active (bankrupt during their turn), pick first active
	if currentIdx == -1 {
		if len(activePlayers) == 0 {
			return nil, errors.GameNotStarted()
		}
		currentIdx = len(activePlayers) - 1 // So next will be index 0
	}

	nextIdx := (currentIdx + 1) % len(activePlayers)
	nextPlayer := activePlayers[nextIdx]

	if err := e.store.ResetPlayerTurnStateTx(tx, gameID, nextPlayer.UserID); err != nil {
		return nil, err
	}

	if err := e.store.UpdateCurrentTurnTx(tx, gameID, nextPlayer.UserID); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return &Event{
		Type:   "turn_changed",
		GameID: gameID,
		Payload: TurnChangedPayload{
			PreviousPlayerID: userID,
			CurrentPlayerID:  nextPlayer.UserID,
		},
	}, nil
}

// ProposeTrade creates a new trade offer
func (e *Engine) ProposeTrade(gameID, fromUserID, toUserID int64, offer TradeOffer) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	// Verify both players are in the game and not bankrupt
	var fromPlayer, toPlayer *Player
	for _, p := range state.Players {
		if p.UserID == fromUserID {
			fromPlayer = p
		}
		if p.UserID == toUserID {
			toPlayer = p
		}
	}

	if fromPlayer == nil || toPlayer == nil {
		return nil, errors.NotInGame()
	}

	if fromPlayer.IsBankrupt || toPlayer.IsBankrupt {
		return nil, errors.PlayerBankrupt()
	}

	// Verify fromPlayer owns offered properties and they're not mortgaged/improved
	for _, pos := range offer.OfferedProperties {
		ownerID, ok := state.Properties[pos]
		if !ok || ownerID != fromUserID {
			return nil, errors.PropertyNotOwned()
		}
		if state.MortgagedProperties[pos] {
			return nil, errors.BadRequest("Cannot trade mortgaged properties")
		}
		if state.Improvements[pos] > 0 {
			return nil, errors.BadRequest("Cannot trade properties with houses/hotels")
		}
	}

	// Verify toPlayer owns requested properties and they're not mortgaged/improved
	for _, pos := range offer.RequestedProperties {
		ownerID, ok := state.Properties[pos]
		if !ok || ownerID != toUserID {
			return nil, errors.BadRequest("Other player doesn't own requested property")
		}
		if state.MortgagedProperties[pos] {
			return nil, errors.BadRequest("Cannot trade mortgaged properties")
		}
		if state.Improvements[pos] > 0 {
			return nil, errors.BadRequest("Cannot trade properties with houses/hotels")
		}
	}

	// Verify fromPlayer has enough money
	if fromPlayer.Money < offer.OfferedMoney {
		return nil, errors.InsufficientFunds()
	}

	// Marshal offer to JSON
	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return nil, err
	}

	// Create the trade
	tradeID, err := e.store.CreateTrade(gameID, fromUserID, toUserID, string(offerJSON))
	if err != nil {
		return nil, err
	}

	trade := &Trade{
		ID:         tradeID,
		GameID:     gameID,
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Offer:      offer,
		Status:     "pending",
	}

	return &Event{
		Type:   "trade_proposed",
		GameID: gameID,
		Payload: TradeProposedPayload{
			Trade:        trade,
			FromUsername: fromPlayer.Username,
			ToUsername:   toPlayer.Username,
		},
	}, nil
}

// AcceptTrade accepts a pending trade
func (e *Engine) AcceptTrade(gameID, userID, tradeID int64) ([]*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	// Get the trade
	dbTrade, err := e.store.GetTrade(tradeID)
	if err != nil {
		return nil, err
	}
	if dbTrade == nil {
		return nil, errors.BadRequest("Trade not found")
	}

	if dbTrade.Status != "pending" {
		return nil, errors.BadRequest("Trade is no longer pending")
	}

	if dbTrade.ToUserID != userID {
		return nil, errors.BadRequest("You are not the recipient of this trade")
	}

	// Parse the offer
	var offer TradeOffer
	if err := json.Unmarshal([]byte(dbTrade.OfferJSON), &offer); err != nil {
		return nil, err
	}

	// Re-validate the trade (properties and money)
	var fromPlayer, toPlayer *Player
	for _, p := range state.Players {
		if p.UserID == dbTrade.FromUserID {
			fromPlayer = p
		}
		if p.UserID == dbTrade.ToUserID {
			toPlayer = p
		}
	}

	if fromPlayer == nil || toPlayer == nil || fromPlayer.IsBankrupt || toPlayer.IsBankrupt {
		if err := e.store.UpdateTradeStatus(tradeID, "cancelled"); err != nil {
			return nil, err
		}
		return nil, errors.BadRequest("Trade is no longer valid")
	}

	// Check money
	if fromPlayer.Money < offer.OfferedMoney || toPlayer.Money < offer.RequestedMoney {
		if err := e.store.UpdateTradeStatus(tradeID, "cancelled"); err != nil {
			return nil, err
		}
		return nil, errors.InsufficientFunds()
	}

	// Execute the trade
	tx, err := e.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer e.store.RollbackTx(tx)

	// Transfer money
	newFromMoney := fromPlayer.Money - offer.OfferedMoney + offer.RequestedMoney
	newToMoney := toPlayer.Money - offer.RequestedMoney + offer.OfferedMoney

	if err := e.store.UpdatePlayerMoneyTx(tx, gameID, dbTrade.FromUserID, newFromMoney); err != nil {
		return nil, err
	}
	if err := e.store.UpdatePlayerMoneyTx(tx, gameID, dbTrade.ToUserID, newToMoney); err != nil {
		return nil, err
	}

	// Transfer properties
	for _, pos := range offer.OfferedProperties {
		if err := e.store.TransferPropertyTx(tx, gameID, pos, dbTrade.ToUserID); err != nil {
			return nil, err
		}
	}
	for _, pos := range offer.RequestedProperties {
		if err := e.store.TransferPropertyTx(tx, gameID, pos, dbTrade.FromUserID); err != nil {
			return nil, err
		}
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	// Update trade status
	if err := e.store.UpdateTradeStatus(tradeID, "accepted"); err != nil {
		return nil, err
	}

	return []*Event{
		{
			Type:   "trade_accepted",
			GameID: gameID,
			Payload: TradeResponsePayload{
				TradeID:      tradeID,
				FromUserID:   dbTrade.FromUserID,
				ToUserID:     dbTrade.ToUserID,
				Status:       "accepted",
				FromUsername: fromPlayer.Username,
				ToUsername:   toPlayer.Username,
			},
		},
	}, nil
}

// DeclineTrade declines a pending trade
func (e *Engine) DeclineTrade(gameID, userID, tradeID int64) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	// Get the trade
	dbTrade, err := e.store.GetTrade(tradeID)
	if err != nil {
		return nil, err
	}
	if dbTrade == nil {
		return nil, errors.BadRequest("Trade not found")
	}

	if dbTrade.Status != "pending" {
		return nil, errors.BadRequest("Trade is no longer pending")
	}

	if dbTrade.ToUserID != userID {
		return nil, errors.BadRequest("You are not the recipient of this trade")
	}

	// Update trade status
	if err := e.store.UpdateTradeStatus(tradeID, "declined"); err != nil {
		return nil, err
	}

	var fromUsername, toUsername string
	for _, p := range state.Players {
		if p.UserID == dbTrade.FromUserID {
			fromUsername = p.Username
		}
		if p.UserID == dbTrade.ToUserID {
			toUsername = p.Username
		}
	}

	return &Event{
		Type:   "trade_declined",
		GameID: gameID,
		Payload: TradeResponsePayload{
			TradeID:      tradeID,
			FromUserID:   dbTrade.FromUserID,
			ToUserID:     dbTrade.ToUserID,
			Status:       "declined",
			FromUsername: fromUsername,
			ToUsername:   toUsername,
		},
	}, nil
}

// CancelTrade cancels a pending trade (by the proposer)
func (e *Engine) CancelTrade(gameID, userID, tradeID int64) (*Event, error) {
	state, err := e.GetGameState(gameID)
	if err != nil {
		return nil, err
	}

	if state.Status != StatusInProgress {
		return nil, errors.GameNotStarted()
	}

	// Get the trade
	dbTrade, err := e.store.GetTrade(tradeID)
	if err != nil {
		return nil, err
	}
	if dbTrade == nil {
		return nil, errors.BadRequest("Trade not found")
	}

	if dbTrade.Status != "pending" {
		return nil, errors.BadRequest("Trade is no longer pending")
	}

	if dbTrade.FromUserID != userID {
		return nil, errors.BadRequest("You are not the proposer of this trade")
	}

	// Update trade status
	if err := e.store.UpdateTradeStatus(tradeID, "cancelled"); err != nil {
		return nil, err
	}

	var fromUsername, toUsername string
	for _, p := range state.Players {
		if p.UserID == dbTrade.FromUserID {
			fromUsername = p.Username
		}
		if p.UserID == dbTrade.ToUserID {
			toUsername = p.Username
		}
	}

	return &Event{
		Type:   "trade_cancelled",
		GameID: gameID,
		Payload: TradeResponsePayload{
			TradeID:      tradeID,
			FromUserID:   dbTrade.FromUserID,
			ToUserID:     dbTrade.ToUserID,
			Status:       "cancelled",
			FromUsername: fromUsername,
			ToUsername:   toUsername,
		},
	}, nil
}
