# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Build & Run

```bash
go build -o monopoly.exe .    # Build
./monopoly.exe                # Run (http://localhost:8080)
taskkill /F /IM monopoly.exe  # Stop if port in use (Windows)
```

## Project Overview

Multiplayer Monopoly web app. Core game mechanics are fully implemented (dice, movement, properties, rent, taxes, bankruptcy, Go To Jail). **Next phase: advanced rules** (jail escape, doubles re-roll, Chance/Community Chest cards, houses/hotels, mortgage, trading).

**Known UI issue:** The board center area (game log + controls) renders squished in the top-left corner. Fix this before adding new frontend features.

## Architecture

### Dependency Injection (main.go)

```
config.Load() → Config
store.NewSQLiteStore(dbPath) → Store interface
auth.NewSessionManager(db) → SessionManager  ← takes *sql.DB (DB-backed sessions)
auth.NewService(store, sessionManager) → Service
game.NewLobby(store) → Lobby
game.NewEngine(store) → Engine
ws.NewManager(engine) → Manager  ← owns TurnTimer internally
http.NewServer(authService, lobby, engine, wsManager, store) → Server
```

### Project Structure

```
auth/           Auth service, input sanitization, DB-backed sessions
config/         Server configuration
errors/         Centralized error codes and AppError type
game/           Engine (state machine), lobby logic, models, board, turn timer
http/           Handlers, middleware (auth/logging/CORS/security), rate limiting, routing
static/         SPA frontend (vanilla JS ES6 modules)
  css/main.css          All styles including Monopoly board CSS grid
  js/app.js             Main app + routing
  js/router.js          Hash-based router
  js/api.js             API client
  js/template.js        Template loader with cache
  js/views/*.js         View modules (render + cleanup)
  templates/*.html      HTML templates
store/          SQLite store (interfaces, implementations, migrations)
ws/             WebSocket managers (game rooms + lobby), message types
```

### Key Patterns

**1. Store Interface** — `store/` splits into `AuthStore`, `LobbyStore`, `GameStore` interfaces. All DB access goes through interfaces. `GameStore` includes transaction variants (`*Tx` methods) for atomic operations.

**2. Game Engine State Machine** — `game/engine.go` validates all transitions. State: `waiting` → `in_progress` → `finished`. Multi-step state changes (ready→start, endTurn→nextTurn) use SQL transactions via `BeginTx()`/`CommitTx()`/`RollbackTx()`.

**3. Centralized Errors** — `errors/errors.go` defines `AppError` with machine-readable codes (`GAME_NOT_FOUND`, `NOT_YOUR_TURN`, `UNAUTHORIZED`, etc.). HTTP handlers map codes to status codes. WebSocket sends `{"type":"error","payload":{"code":"...","message":"..."}}`.

**4. Turn Timer** — `game/turn_timer.go` auto-skips turns after 60s. Managed by `ws/manager.go` via `BroadcastGameEvent()`. Timer starts on `game_started`/`turn_changed`, cancels on `end_turn`/`game_finished`.

**5. WebSocket Rooms** — `ws/manager.go` maintains `map[gameID]*Room`. Lobby has its own manager (`ws/lobby_manager.go`). Event flow: Client → WS → Engine → Store → Event → Room.Broadcast().

**6. DB-Backed Sessions** — `auth/session.go` stores sessions in `sessions` table (persists across restarts). Periodic cleanup of expired sessions.

### Database Schema

```sql
users (id, username, password_hash, created_at)
sessions (session_id, user_id, created_at, expires_at)  -- FK users.id CASCADE
games (id, status, max_players, created_at)
game_players (game_id, user_id, player_order, is_ready, is_current_turn,
              has_played_turn, money, position, is_bankrupt, has_rolled, pending_action)
  -- Composite PK (game_id, user_id), FKs to games and users
game_properties (game_id, position, owner_id)
  -- Composite PK (game_id, position), FKs to games and users
```

Schema lives in `store/migrations.go`. To modify: update `schema` const, delete `monopoly.db`, restart.

### Game State (Player & GameState models)

**Player fields:** `UserID`, `Username`, `Order`, `IsReady`, `IsCurrentTurn`, `Money` (starts 1500), `Position` (0–39), `IsBankrupt`, `HasRolled`, `PendingAction` ("buy_or_pass" or "")

**GameState fields:** `ID`, `Status`, `Players`, `CurrentPlayerID`, `MaxPlayers`, `Properties` (map[position→ownerID]), `Board` ([40]BoardSpace)

### Game State Lifecycle

1. Create game → `status='waiting'`
2. Players join → `game_players` with `player_order`
3. All ready (min 2) → `status='in_progress'`, first player gets `is_current_turn=1`
4. Player rolls dice → movement resolved (properties, taxes, jail, etc.), `has_rolled=1`
5. End turn → round-robin via `player_order`, 60s timer starts
6. Timer expires → auto-skip with `turn_timeout` event
7. All but one bankrupt → `status='finished'`

### Implemented Game Mechanics

- **Dice & movement**: Two d6, position wraps modulo 40
- **Passing GO**: Collect $200 when position wraps
- **Properties** (28), **railroads** (4), **utilities** (2): buy on landing, pay rent to owner
- **Rent calculation**: Base rent; doubled for color monopoly; railroad scales with count (25/50/100/200); utility is 4× or 10× dice roll
- **Buy/pass prompt**: `pending_action="buy_or_pass"` blocks end-turn until resolved
- **Tax spaces**: Income Tax ($200, pos 4), Luxury Tax ($100, pos 38)
- **Go To Jail**: Position 30 sends player to position 10
- **Bankruptcy**: Cannot pay → lose all properties, eliminated; last solvent player wins

### WebSocket Message Types

**Game room** (client→server): `roll_dice`, `buy_property`, `pass_property`, `end_turn`

**Game room** (server→client): `game_started`, `turn_changed`, `turn_timeout`, `dice_rolled`, `buy_prompt`, `property_bought`, `property_passed`, `rent_paid`, `tax_paid`, `go_to_jail`, `player_bankrupt`, `game_finished`, `error`

**Lobby** (server→client): `game_created`, `game_deleted`, `player_joined`, `player_left`, `game_status_changed`

### Frontend

- SPA with hash-based routing: `#/login`, `#/register`, `#/lobby`, `#/game?gameId=X`
- Views export `render(container, router)` + `cleanup()`
- Game view uses a 13×13 CSS grid Monopoly board with 40 spaces
- Board renders player tokens (multi-token per space), ownership bars, space names/colors
- Dice result modal, buy/pass prompt modal, game over modal
- Board center area holds game log + controls (currently has layout issues)

### HTTP API

- **Public**: `POST /api/auth/register`, `POST /api/auth/login`
- **Protected**: `/api/lobby/*`, `GET /ws/game/:gameId` (WebSocket upgrade, verifies player membership)
- **Middleware**: Logging → CORS → Auth (protected only). Auth injects `userID` via `context.WithValue()`.
- **Error responses**: `{"error": "CODE", "message": "user-friendly text"}` with appropriate HTTP status

## Conventions (enforce these)

1. **Always use `errors/` package** for errors returned from engine/auth. Never create ad-hoc error types. Use factory functions (`errors.GameNotFound()`, `errors.NotYourTurn()`, etc.).
2. **Use transactions for multi-step DB operations.** If a state change touches multiple rows (e.g., setting ready + starting game), wrap in `BeginTx()`/`CommitTx()` with `defer RollbackTx()`.
3. **HTTP error responses go through `writeError()`** in `http/handlers.go`. It handles `AppError` → HTTP status mapping.
4. **WebSocket errors use structured format**: `{"type":"error","payload":{"code":"...","message":"..."}}`.
5. **New game commands follow the pattern**: Engine method validates + updates DB → returns Event → `ws/manager.go` calls `BroadcastGameEvent()` → room broadcasts + timer management.
6. **Frontend views must clean up** WebSocket connections and timers in `cleanup()`. Use close code 1000 for normal closure.
7. **Store interface first**: Add interface method to the relevant store interface in `store/`, then implement in the concrete file (`game_store.go`, `lobby_store.go`, `auth_store.go`).

## Next Phase: Advanced Rules

Priority order:

### 1. Jail Mechanics
Currently `go_to_jail` moves player to position 10 but jail rules aren't enforced.
- Track `in_jail` and `jail_turns` on `game_players` (add DB columns)
- On roll while jailed: doubles escape jail; otherwise increment `jail_turns`
- After 3 failed rolls: pay $50 bail and move normally
- Add `pay_jail_bail` client command to pay $50 before rolling
- Add `get_out_of_jail_free` cards (see Chance/Community Chest)
- New WS events: `jail_escape_roll`, `jail_bail_paid`

### 2. Doubles Re-roll
When a player rolls doubles they should roll again immediately (up to 3 times).
- Track `doubles_count` per turn (in-memory in Engine is fine, or add DB column)
- Three doubles in a row → Go To Jail instead of re-rolling
- `HasRolled` should reset after a doubles roll so the player can roll again
- No `end_turn` required between doubles rolls (it happens automatically)

### 3. Chance & Community Chest Cards
- Define card decks in `game/cards.go` (shuffled on game start)
- Store deck state in DB: `game_card_decks (game_id, deck_type, card_order, next_index)`
- Card effects reuse existing engine methods (move, collect money, pay money, go to jail, etc.)
- New WS event: `card_drawn` with card text and effect
- "Get Out of Jail Free" card: store on player, use before roll

### 4. Houses & Hotels
- Add `game_improvements (game_id, position, count)` table (count 1–5; 5 = hotel)
- `BuyHouse(gameID, userID, position)` engine command
- Rent multipliers per improvement level defined in `game/board.go` BoardSpace
- Limit: 32 houses / 12 hotels total per game (supply constraint)
- New WS events: `house_built`, `hotel_built`

### 5. Mortgage System
- Add `mortgaged` flag to `game_properties`
- `MortgageProperty(gameID, userID, position)` — player receives half price
- `UnmortgageProperty(gameID, userID, position)` — pay 110% to lift
- Mortgaged properties: owner collects no rent
- New WS events: `property_mortgaged`, `property_unmortgaged`

### 6. Trading
- Two-phase: propose + accept/decline
- `ProposeTrade(gameID, fromUserID, toUserID, offer TradeOffer)` engine command
- Store pending trade in `game_trades (game_id, from_user, to_user, offer_json, status)`
- New WS events: `trade_proposed`, `trade_accepted`, `trade_declined`

## Testing

No automated tests yet. Manual testing:
- `./test-api.sh` for REST endpoints
- Multiple browsers for WebSocket sync
- `sqlite3 monopoly.db "SELECT * FROM game_players;"`

When adding tests: mock via store interfaces, test engine state transitions, test WS with mock connections.

## Config

`config/config.go`: Port `:8080`, DB `./monopoly.db`, session secret random 32 bytes, max 25 open / 5 idle DB conns.
