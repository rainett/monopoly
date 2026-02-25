# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

```bash
# Build the server
go build -o monopoly.exe .

# Run the server
./monopoly.exe

# Server starts on http://localhost:8080
# Press Ctrl+C for graceful shutdown

# Stop server if port is in use (Windows)
taskkill /F /IM monopoly.exe
```

## Architecture Overview

This is a multiplayer Monopoly backend MVP focused on **infrastructure, not game mechanics**. It demonstrates lobby system, turn management, and real-time communication patterns.

### Dependency Injection Flow

The application uses constructor-based dependency injection. Dependencies flow from `main.go`:

```
main.go
  ↓ creates
  ├─ config.Load() → Config
  ├─ store.NewSQLiteStore(dbPath) → Store interface
  ├─ auth.NewSessionManager(secret) → SessionManager
  ├─ auth.NewService(store, sessionManager) → Service
  ├─ game.NewLobby(store) → Lobby
  ├─ game.NewEngine(store) → Engine
  ├─ ws.NewManager(engine) → Manager
  └─ http.NewServer(authService, lobby, engine, wsManager, store) → Server
```

All components receive their dependencies via constructors, no globals.

### Key Architectural Patterns

**1. Store Interface Pattern**
- `store/store.go` defines `Store` interface
- `SQLiteStore` implements it
- All data access goes through this interface
- Makes database swappable without changing business logic

**2. Immutable State Transitions (Game Engine)**
- `game/engine.go` validates all state changes
- State transitions: `waiting` → `in_progress` → `finished`
- Commands: `JoinGame()`, `SetReady()`, `EndTurn()`
- Each command returns an `Event` for WebSocket broadcasting
- Engine queries current state, validates action, updates DB, returns event

**3. WebSocket Room-Based Broadcasting**
- `ws/manager.go` maintains `map[gameID]*Room`
- Each game gets its own room
- `ws/room.go` manages per-game client connections
- When engine returns event, room broadcasts to all connected clients
- Message flow: Client → WS → Engine → Store → Event → Room.Broadcast() → All Clients

**4. Session Management**
- Sessions stored in-memory (map with mutex)
- `auth/session.go` manages session creation/validation
- HTTP-only cookies prevent XSS
- Sessions cleared on server restart (MVP simplification)

### Critical State Management

**Game State Lifecycle:**
1. User creates game → `status='waiting'`, stored in `games` table
2. Users join → inserted into `game_players` with `player_order`
3. All click ready → `is_ready=1` for each player
4. When all ready (min 2 players) → `status='in_progress'`, first player gets `is_current_turn=1`
5. End turn → clear current player's turn flag, set next player's flag (round-robin via `player_order`)

**WebSocket Connection Flow:**
1. Client establishes WS connection with session cookie
2. `ws/manager.go` validates session via auth service
3. Client added to game's room
4. Client sends JSON messages: `{"type": "ready", "payload": {...}}`
5. Manager calls engine methods, broadcasts resulting events
6. Ping/pong keepalive prevents disconnections

### Database Schema Relationships

```
users (1) ──< (M) game_players (M) >── (1) games
         └─────────────┬─────────────┘
                       │
              Composite PK: (game_id, user_id)
```

- Users can be in multiple games
- Each game tracks player order and turn state
- `is_current_turn` is exclusive (only one player per game)

## Frontend Architecture (SPA)

The frontend is a Single Page Application using vanilla JavaScript ES6 modules:

**File Structure:**
```
static/
├── index.html              # Single entry point
├── css/
│   └── main.css           # Consolidated styles
├── js/
│   ├── app.js             # Main application & routing orchestration
│   ├── router.js          # Hash-based client router
│   ├── api.js             # API client with error handling
│   ├── template.js        # Template loader with caching
│   └── views/             # View modules (logic only)
│       ├── login.js       # Login view logic
│       ├── register.js    # Registration view logic
│       ├── lobby.js       # Game lobby view logic
│       └── game.js        # Game room view logic with WebSocket
└── templates/             # HTML templates (presentation only)
    ├── login.html         # Login template
    ├── register.html      # Registration template
    ├── lobby.html         # Lobby template
    └── game.html          # Game room template
```

**Routing:**
- Hash-based routing (#/login, #/lobby, #/game?gameId=X)
- No page reloads - smooth transitions between views
- Router handles navigation, query parameters, and route protection

**View Pattern:**
Each view consists of:
- HTML template in `static/templates/myview.html` (presentation)
- JavaScript module in `static/js/views/myview.js` (logic)

View modules export:
- `async render(container, router)` - Loads template and sets up event handlers
- `cleanup()` - Cleans up resources (timers, WebSocket connections)

**Adding New Frontend Features:**
1. Create HTML template in `static/templates/myview.html`
2. Create view module in `static/js/views/myview.js`:
   ```javascript
   import { api } from '../api.js';
   import { templateLoader } from '../template.js';

   export async function render(container, router) {
       const template = await templateLoader.load('myview');
       container.innerHTML = template;
       // Add event listeners here
   }

   export function cleanup() {
       // Cleanup code here
   }
   ```
3. Register route in `static/js/app.js`: `router.register('/myview', () => this.loadView(MyView))`
4. Navigate: `router.navigate('/myview', { param: 'value' })`

## HTTP API Structure

Routes in `http/server.go`:
- **Public**: `/api/auth/register`, `/api/auth/login`
- **Protected** (requires session): `/api/lobby/*`, `/ws/game/:gameId`
- **Static Assets**: `/css/*`, `/js/*` serve from `./static/`
- **SPA Fallback**: All other routes serve `index.html` (client-side routing)

Middleware chain: `LoggingMiddleware` → `CORSMiddleware` → `AuthMiddleware` (protected routes only)

`AuthMiddleware` validates session cookie and injects `userID` into request context via `context.WithValue()`.

## Adding New Game Mechanics

To add Monopoly game logic (dice, properties, money):

1. **Extend `game/models.go`**: Add fields to `Player` (money, position, properties) and `GameState`
2. **Add commands to `game/engine.go`**: e.g., `RollDice()`, `BuyProperty()`, `PayRent()`
3. **Extend database schema** in `store/migrations.go`: Add tables for properties, transactions
4. **Add Store methods** in `store/store.go` interface: e.g., `GetProperties()`, `UpdatePlayerMoney()`
5. **Add WebSocket message types** in `ws/messages.go`: e.g., `dice_rolled`, `property_bought`
6. **Handle new messages** in `ws/manager.go` `handleMessage()` switch statement
7. **Update frontend** `static/js/views/game.js` to handle new event types in `handleWebSocketMessage()`

The architecture is designed for this expansion—state machine pattern makes adding commands straightforward.

## Configuration

Server configuration in `config/config.go`:
- Port: `:8080` (hardcoded)
- DB path: `./monopoly.db` (hardcoded)
- Session secret: Generated randomly on startup (32 bytes)

To change these, modify `config.Load()`. No environment variable support yet (MVP).

## Database Migrations

Schema defined in `store/migrations.go` as a single SQL string. Executed on server startup via `db.Exec(schema)`.

To modify schema:
1. Update `schema` const in `migrations.go`
2. Delete `monopoly.db`
3. Restart server (creates new DB with updated schema)

No migration versioning system—this is MVP simplification.

## Testing Strategy

Currently no automated tests. To manually test:

1. Run `./test-api.sh` to test REST API endpoints
2. Open multiple browsers to test WebSocket synchronization
3. Check `monopoly.db` with: `sqlite3 monopoly.db "SELECT * FROM game_players;"`

When adding tests:
- Use `store.Store` interface for mocking database
- Test engine state transitions with in-memory mock store
- Test WebSocket broadcasting with mock connections
