package game

import (
	"database/sql"
	"math/rand"
	"monopoly/errors"
	"monopoly/store"
)

type Engine struct {
	store store.GameStore
}

func NewEngine(store store.GameStore) *Engine {
	return &Engine{store: store}
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
	for _, p := range props {
		properties[p.Position] = p.OwnerID
	}

	return &GameState{
		ID:              game.ID,
		Status:          game.Status,
		Players:         gamePlayers,
		CurrentPlayerID: currentPlayerID,
		MaxPlayers:      game.MaxPlayers,
		Properties:      properties,
		Board:           Board,
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

	if err := e.store.SetPlayerHasRolledTx(tx, gameID, userID, true); err != nil {
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
			UserID:    userID,
			Die1:      die1,
			Die2:      die2,
			Total:     total,
			OldPos:    oldPos,
			NewPos:    newPos,
			PassedGo:  passedGo,
			SpaceName: space.Name,
			SpaceType: string(space.Type),
		},
	})

	resolutionEvents, err := e.resolveSpaceLanding(tx, gameID, userID, player.Username, currentMoney, space, total)
	if err != nil {
		return nil, err
	}
	events = append(events, resolutionEvents...)

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	return events, nil
}

func (e *Engine) resolveSpaceLanding(tx *sql.Tx, gameID, userID int64, username string, currentMoney int, space BoardSpace, diceTotal int) ([]*Event, error) {
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
			// Owned by someone else - pay rent
			ownerProps, err := e.store.GetPlayerPropertiesTx(tx, gameID, ownerID)
			if err != nil {
				return nil, err
			}

			rent := CalculateRent(space, ownerProps, diceTotal)

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
		events = append(events, &Event{
			Type:   "go_to_jail",
			GameID: gameID,
			Payload: GoToJailPayload{
				UserID: userID,
				OldPos: space.Position,
			},
		})

	// Go, Jail (visiting), Free Parking, Chance, Community Chest - no-op for MVP
	}

	return events, nil
}

func (e *Engine) handleBankruptcyTx(tx *sql.Tx, gameID, userID int64, username, reason string, creditorID int64) ([]*Event, error) {
	var events []*Event

	// Mark player bankrupt
	if err := e.store.SetPlayerBankruptTx(tx, gameID, userID); err != nil {
		return nil, err
	}

	// Release all properties (become unowned)
	if err := e.store.DeletePlayerPropertiesTx(tx, gameID, userID); err != nil {
		return nil, err
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

func (e *Engine) PassProperty(gameID, userID int64) (*Event, error) {
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

	if err := e.store.SetPlayerPendingActionTx(tx, gameID, userID, ""); err != nil {
		return nil, err
	}

	if err := e.store.CommitTx(tx); err != nil {
		return nil, err
	}

	space := Board[player.Position]
	return &Event{
		Type:   "property_passed",
		GameID: gameID,
		Payload: PropertyPassedPayload{
			UserID:   userID,
			Position: player.Position,
			Name:     space.Name,
		},
	}, nil
}

func (e *Engine) EndTurn(gameID, userID int64) (*Event, error) {
	return e.endTurnInternal(gameID, userID, false)
}

func (e *Engine) ForceEndTurn(gameID, userID int64) (*Event, error) {
	return e.endTurnInternal(gameID, userID, true)
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
