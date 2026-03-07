# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Build & Run

```bash
go build -o monopoly.exe .    # Build
go test ./game/... -v         # Run tests
./monopoly.exe                # Run (http://localhost:8080)
taskkill /F /IM monopoly.exe  # Stop if port in use (Windows)
```

## Project Overview

Multiplayer Monopoly web app with **fully implemented game mechanics** including:
- Core gameplay (dice, movement, properties, rent, taxes, bankruptcy)
- Jail mechanics (escape via doubles, bail, Get Out of Jail Free cards)
- Doubles re-roll (up to 3 times, third doubles = jail)
- Chance & Community Chest cards (16 cards each, all effects implemented)
- Houses & Hotels (even build rule, 32 house / 12 hotel supply limit, sell constraint)
- Mortgage system (receive half price, pay 110% to unmortgage)
- Trading system (propose/accept/decline trades for properties and money)
- Turn timer with 3-strike elimination (60s per turn, 3 consecutive timeouts = eliminated)
- **Auctions** - Round-robin bidding when a player passes on a property
- **Friends system** - User search, friend requests, friends list
- **UI enhancements** - Property info panel, card modals, token animations, browser notifications

## Architecture

### Dependency Injection (main.go)

```
config.Load() → Config
store.NewSQLiteStore(dbPath) → Store interface
auth.NewSessionManager(db) → SessionManager  ← takes *sql.DB (DB-backed sessions)
auth.NewService(store, sessionManager) → Service
game.NewLobby(store) → Lobby
game.NewEngine(store) → Engine  ← owns activeAuctions map internally
ws.NewManager(engine, lobbyManager) → Manager  ← owns TurnTimer internally
http.NewServer(authService, authStore, lobby, engine, wsManager, lobbyManager) → Server
```

### Project Structure

```
auth/           Auth service, input sanitization, DB-backed sessions
config/         Server configuration
errors/         Centralized error codes and AppError type
game/           Engine (state machine), lobby logic, models, board, cards, turn timer
http/           Handlers, middleware (auth/logging/CORS/security), rate limiting, routing
static/         SPA frontend (vanilla JS ES6 modules)
  css/main.css          All styles including Monopoly board CSS grid, friends panel
  js/app.js             Main app + routing
  js/router.js          Hash-based router
  js/api.js             API client (includes friends API methods)
  js/template.js        Template loader with cache
  js/views/*.js         View modules (render + cleanup)
  templates/*.html      HTML templates
store/          SQLite store (interfaces, implementations, migrations)
ws/             WebSocket managers (game rooms + lobby), message types
```

### Key Patterns

**1. Store Interface** — `store/` splits into `AuthStore`, `LobbyStore`, `GameStore` interfaces. All DB access goes through interfaces. `GameStore` includes transaction variants (`*Tx` methods) for atomic operations.

**2. Game Engine State Machine** — `game/engine.go` validates all transitions. State: `waiting` → `in_progress` → `finished`. Multi-step state changes (ready→start, endTurn→nextTurn) use SQL transactions via `BeginTx()`/`CommitTx()`/`RollbackTx()`.

**3. Centralized Errors** — `errors/errors.go` defines `AppError` with machine-readable codes (`GAME_NOT_FOUND`, `NOT_YOUR_TURN`, `UNAUTHORIZED`, `AUCTION_IN_PROGRESS`, etc.). HTTP handlers map codes to status codes. WebSocket sends `{"type":"error","payload":{"code":"...","message":"..."}}`.

**4. Turn Timer System** — `game/turn_timer.go` manages turn timeouts:
- 60 second timeout per turn (configurable via `TurnTimeout` constant)
- Tracks consecutive timeouts per player (resets when player takes action)
- 3 consecutive timeouts = player eliminated (auto-bankrupted)
- Frontend receives `timer_started` event with duration, displays UTF-8 block progress bar
- Timer shown in action box when your turn, in players list when other's turn
- Timer also applies to auction bidders (each bid/pass triggers timer for next bidder)
- Timer cancels on manual `end_turn` or `game_finished`

**5. WebSocket Rooms** — `ws/manager.go` maintains `map[gameID]*Room`. Lobby has its own manager (`ws/lobby_manager.go`). Event flow: Client → WS → Engine → Store → Event → Room.Broadcast().

**6. DB-Backed Sessions** — `auth/session.go` stores sessions in `sessions` table (persists across restarts). Periodic cleanup of expired sessions.

**7. Auction System** — `game/engine.go` maintains `activeAuctions map[int64]*Auction`. When a player passes on a property, an auction starts with round-robin bidding among all non-bankrupt players. Frontend shows inline "BID $X" / "PASS" buttons in action box (no modal). Bid auto-increments by $10. Each bidder gets turn timer.

### Database Schema

```sql
users (id, username, password_hash, created_at)
sessions (session_id, user_id, created_at, expires_at)
games (id, status, max_players, created_at)
game_players (game_id, user_id, player_order, is_ready, is_current_turn,
              has_played_turn, money, position, is_bankrupt, has_rolled,
              pending_action, in_jail, jail_turns)
game_properties (game_id, position, owner_id, is_mortgaged)
game_improvements (game_id, position, count)  -- 1-4 houses, 5 = hotel
game_card_decks (game_id, deck_type, card_order, next_index)
player_jail_cards (game_id, user_id, deck_type)  -- Get Out of Jail Free cards
game_trades (id, game_id, from_user_id, to_user_id, offer_json, status, created_at)
friendships (user_id_1, user_id_2, status, created_at)  -- pending/accepted
game_invites (id, game_id, from_user_id, to_user_id, status, created_at)
```

Schema lives in `store/migrations.go`. To modify: update `schema` const, delete `monopoly.db`, restart.

### Game State (Player & GameState models)

**Player fields:** `UserID`, `Username`, `Order`, `IsReady`, `IsCurrentTurn`, `Money` (starts 1500), `Position` (0–39), `IsBankrupt`, `HasRolled`, `PendingAction`, `InJail`, `JailTurns`

**GameState fields:** `ID`, `Status`, `Players`, `CurrentPlayerID`, `MaxPlayers`, `Properties` (map[position→ownerID]), `MortgagedProperties`, `Improvements`, `Board` ([40]BoardSpace)

**Auction fields:** `GameID`, `Position`, `PropertyName`, `HighestBid`, `HighestBidderID`, `BidderOrder`, `CurrentBidder`, `PassedBidders`

### Game State Lifecycle

1. Create game → `status='waiting'`
2. Players join → `game_players` with `player_order`
3. All ready (min 2) OR game full → `status='in_progress'`, decks shuffled, first player gets turn
4. Player rolls dice → movement resolved (properties, cards, jail, etc.)
5. Land on unowned property → buy prompt → buy or pass → **if pass, auction starts**
6. End turn → round-robin via `player_order`, 60s timer starts
7. Timer expires → auto-skip with `turn_timeout` event (3 consecutive = eliminated)
8. All but one bankrupt → `status='finished'`

### Implemented Game Mechanics

- **Dice & movement**: Two d6, position wraps modulo 40
- **Doubles**: Roll again (up to 3x), third doubles = Go to Jail
- **Passing GO**: Collect $200 when position wraps
- **Properties** (28), **railroads** (4), **utilities** (2): buy on landing, pay rent to owner
- **Rent calculation**: Base rent → color monopoly (2x) → houses/hotels (defined in board.go)
- **Houses/Hotels**: Even build rule, 32 house / 12 hotel supply limit, cannot sell hotel without 4 houses available
- **Mortgage**: Receive 50% value, pay 110% to unmortgage, no rent while mortgaged
- **Tax spaces**: Income Tax ($200, pos 4), Luxury Tax ($100, pos 38)
- **Jail**: Position 30 → jail; escape via doubles, $50 bail, or Get Out of Jail Free card
- **Cards**: Chance (positions 7, 22, 36) and Community Chest (positions 2, 17, 33)
  - "Advance to nearest Railroad" cards apply 2x rent multiplier
  - "Advance to nearest Utility" cards apply 10x dice (instead of normal 4x)
- **Trading**: Propose trades for properties and money between players
- **Bankruptcy**: Cannot pay → properties transfer to creditor (or bank if tax/card); last solvent player wins
- **Turn timer**: 60s per turn, 3 consecutive timeouts = eliminated
- **Auctions**: When player passes on property, round-robin bidding starts; each bidder has 60s timer; bid auto-increments by $10; highest bidder wins

### WebSocket Message Types

**Game room** (client→server):
- `roll_dice`, `buy_property`, `pass_property`, `end_turn`
- `pay_jail_bail`, `use_jail_card`
- `mortgage_property`, `unmortgage_property`
- `buy_house`, `sell_house`
- `propose_trade`, `accept_trade`, `decline_trade`, `cancel_trade`
- `place_bid`, `pass_auction`
- `chat`

**Game room** (server→client):
- `game_started`, `turn_changed`, `turn_timeout`, `timer_started`
- `dice_rolled`, `buy_prompt`, `property_bought`, `property_passed`
- `rent_paid`, `tax_paid`, `go_to_jail`, `jail_escape`, `jail_roll_failed`
- `card_drawn`, `card_used`
- `property_mortgaged`, `property_unmortgaged`
- `house_built`, `hotel_built`, `house_sold`
- `trade_proposed`, `trade_accepted`, `trade_declined`, `trade_cancelled`
- `auction_started`, `auction_bid`, `auction_passed`, `auction_ended`
- `player_bankrupt`, `game_finished`, `chat`, `error`

**Lobby** (server→client): `game_created`, `game_deleted`, `player_joined`, `player_left`, `game_status_changed`

### Frontend

- SPA with hash-based routing: `#/login`, `#/register`, `#/lobby`, `#/game?gameId=X`
- Views export `render(container, router)` + `cleanup()`
- Game view uses a 13×13 CSS grid Monopoly board with 40 spaces
- Board renders player tokens (with bounce animation), ownership bars, mortgage indicators, house/hotel indicators
- **Action box**: Hidden when not your turn; shows only when you have actions available
- **Turn timer**: UTF-8 block progress bar (`████░░░░`) with color coding (green >30s, yellow 15-30s, red <15s)
  - Your turn: timer in action box
  - Other's turn: timer in players list next to current player
- Trade modal for proposing and receiving trades
- Dice result modal, buy/pass prompt modal, game over modal
- **Property info panel**: Click on board spaces to see property details
- **Card modal**: Displays drawn cards with styling
- **Inline auction controls**: "BID $X" and "PASS" buttons in action box (auto-increment by $10)
- **Browser notifications**: Notifies when it's your turn (if tab hidden)
- **Custom confirmation modal**: Replaces browser `confirm()` for give up, mortgage, accept trade, use jail card
- **Reconnection**: Exponential backoff with jitter, visual reconnect indicator
- **Friends panel**: Collapsible sidebar in lobby with user search, friend requests, friends list

### HTTP API

**Public:**
- `POST /api/auth/register`
- `POST /api/auth/login`

**Protected (require auth):**
- `POST /api/auth/logout`
- `GET /api/lobby/games` - List games
- `POST /api/lobby/create` - Create game
- `POST /api/lobby/join/{gameId}` - Join game
- `POST /api/lobby/leave/{gameId}` - Leave game
- `GET /api/lobby/games/{gameId}` - Get game details

**Friends:**
- `GET /api/users/search?q=...` - Search users by username
- `GET /api/friends` - Get friends list
- `GET /api/friends/requests` - Get pending friend requests
- `POST /api/friends/request` - Send friend request `{userId}`
- `POST /api/friends/accept/{friendId}` - Accept friend request
- `POST /api/friends/decline/{friendId}` - Decline friend request

**WebSocket:**
- `GET /ws/lobby` - Lobby WebSocket
- `GET /ws/game/{gameId}` - Game WebSocket (verifies player membership)

**Middleware**: Logging → CORS → Auth (protected only). Auth injects `userID` via `context.WithValue()`.

**Error responses**: `{"error": "CODE", "message": "user-friendly text"}` with appropriate HTTP status

## Board CSS Architecture (for customization)

The Monopoly board uses a **13×13 CSS grid** layout in `static/css/main.css`:

### Grid Structure
```css
.board {
  display: grid;
  grid-template-columns: repeat(13, 1fr);  /* 13 equal columns */
  grid-template-rows: repeat(13, 1fr);     /* 13 equal rows */
  aspect-ratio: 1 / 1;                      /* Always square */
}
```

### Space Positioning

**Corners (2×2 cells each):**
```css
.space-free-parking { grid-area: 1 / 1 / 3 / 3; }    /* Top-left */
.space-go-to-jail   { grid-area: 1 / 12 / 3 / 14; }  /* Top-right */
.space-jail         { grid-area: 12 / 1 / 14 / 3; }  /* Bottom-left */
.space-go           { grid-area: 12 / 12 / 14 / 14; }/* Bottom-right */
```

**Edge Spaces (1×2 or 2×1 cells, 9 per side):**
- Top row: `.top-1` through `.top-9` (columns 3-11, rows 1-2)
- Bottom row: `.bottom-1` through `.bottom-9` (columns 3-11, rows 12-13, reversed order)
- Left column: `.left-1` through `.left-9` (columns 1-2, rows 3-11)
- Right column: `.right-1` through `.right-9` (columns 12-13, rows 3-11)

**Center Area (9×9 cells):**
```css
.board-center { grid-area: 3 / 3 / 12 / 12; }  /* Game log + controls */
```

### Space Components

Each space has this structure:
```html
<div class="space property top-1" data-space="21">
  <div class="space-inner">
    <div class="space-color brown"></div>  <!-- Color bar for properties -->
    <div class="space-name">Kentucky Ave</div>
  </div>
</div>
```

**Color Classes** (property groups):
- `.brown`, `.light-blue`, `.pink`, `.orange`
- `.red`, `.yellow`, `.green`, `.dark-blue`

**Orientation Handling:**
- Top/Bottom rows: normal orientation
- Left column: `.space-inner` rotated 90° clockwise
- Right column: `.space-inner` rotated 90° counter-clockwise
- Bottom row: `.space-inner` uses `flex-direction: column-reverse`

### Customization Points

1. **Change colors**: Modify `.space-color.*` classes
2. **Resize board**: Adjust `max-height` in `.board`
3. **Space styling**: Modify `.space` base styles
4. **Add indicators**: See `.ownership-bar`, `.mortgage-indicator`, `.house-indicator`
5. **Center content**: Modify `.board-center`, `.center-panel`, `.game-controls`

## Conventions (enforce these)

1. **Always use `errors/` package** for errors returned from engine/auth. Never create ad-hoc error types. Use factory functions (`errors.GameNotFound()`, `errors.NotYourTurn()`, `errors.AuctionInProgress()`, etc.).
2. **Use transactions for multi-step DB operations.** If a state change touches multiple rows (e.g., setting ready + starting game), wrap in `BeginTx()`/`CommitTx()` with `defer RollbackTx()`.
3. **HTTP error responses go through `writeError()`** in `http/handlers.go`. It handles `AppError` → HTTP status mapping.
4. **WebSocket errors use structured format**: `{"type":"error","payload":{"code":"...","message":"..."}}`.
5. **New game commands follow the pattern**: Engine method validates + updates DB → returns Event(s) → `ws/manager.go` calls `BroadcastGameEvent()` → room broadcasts + timer management.
6. **Frontend views must clean up** WebSocket connections and timers in `cleanup()`. Use close code 1000 for normal closure.
7. **Store interface first**: Add interface method to the relevant store interface in `store/`, then implement in the concrete file (`game_store.go`, `lobby_store.go`, `auth_store.go`).
8. **Use custom modals instead of browser dialogs**: Use `showConfirmModal(message, onConfirm, container)` instead of `confirm()`. Modal uses `.modal-overlay` + `.modal-content` CSS classes.

## Testing

Basic test coverage exists in `game/engine_test.go`:

```bash
go test ./game/... -v    # Run all game tests (24 tests)
```

**Test coverage includes:**
- Engine initialization
- Game state retrieval (success and not found cases)
- Join game validation (success, game started, game full, already in game)
- Roll dice validation (not your turn, already rolled, game not started, bankrupt, pending action)
- Rent calculation (base rent, monopoly bonus, houses, railroads, utilities)
- Board setup verification (40 spaces, corners, property groups, tax spaces)

**Mock pattern:** `MockGameStore` implements `store.GameStore` interface for testing without database.

**Manual testing:**
- Multiple browsers for WebSocket sync
- `sqlite3 monopoly.db "SELECT * FROM game_players;"`

## Config

`config/config.go`: Port `:8080`, DB `./monopoly.db`, session secret random 32 bytes, max 25 open / 5 idle DB conns.

## Future Improvements

Potential enhancements (not yet implemented):
- **Game invites UI** - Backend exists, frontend UI for inviting friends to games needed
- **Private games** - Toggle for private/public, only invited friends can join
- **In-game chat UI** - WebSocket infrastructure exists, needs chat panel
- **Sound effects** - Dice rolls, purchases, notifications
- **Player statistics** - Track games played, wins, properties owned
- **Spectator mode** - Watch in-progress games
- **AI players** - Single-player or fill empty slots
- **Custom house rules** - Free Parking jackpot, starting money, etc.
- **PWA support** - Installable app, offline splash screen
- **More test coverage** - Integration tests, WebSocket tests, E2E tests
