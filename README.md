# Monopoly Backend MVP

A real-time multiplayer Monopoly web application backend built with Go, WebSockets, and SQLite.

## Implementation Status ✅

All planned components have been successfully implemented:

### Backend Components
- ✅ **Configuration Module** (`config/`) - Server settings and session management
- ✅ **Database Layer** (`store/`) - SQLite with migrations, user/game/player management
- ✅ **Authentication** (`auth/`) - User registration, login, session management with bcrypt
- ✅ **Game Engine** (`game/`) - Immutable state transitions, turn management, lobby system
- ✅ **WebSocket Manager** (`ws/`) - Real-time communication, room-based broadcasting
- ✅ **HTTP Server** (`http/`) - REST API endpoints, middleware, static file serving

### Frontend Components
- ✅ **Landing Page** (`static/index.html`) - Login form
- ✅ **Registration Page** (`static/register.html`) - User registration
- ✅ **Lobby Page** (`static/lobby.html`) - Game list and creation
- ✅ **Game Room** (`static/game.html`) - Real-time gameplay with WebSocket connection

## Architecture

```
monopoly/
├── main.go                 # Entry point with graceful shutdown
├── config/
│   └── config.go           # Server configuration
├── store/
│   ├── store.go            # Database interface and SQLite implementation
│   └── migrations.go       # Database schema
├── auth/
│   ├── auth.go             # Authentication service
│   └── session.go          # Session management
├── game/
│   ├── models.go           # Game state structures
│   ├── engine.go           # Game state machine
│   └── lobby.go            # Lobby management
├── ws/
│   ├── manager.go          # WebSocket connection manager
│   ├── room.go             # Game room with broadcasting
│   └── messages.go         # Message types
├── http/
│   ├── server.go           # HTTP server setup and routing
│   ├── handlers.go         # API handlers
│   └── middleware.go       # Auth middleware, logging, CORS
└── static/
    ├── index.html          # Login page
    ├── register.html       # Registration page
    ├── lobby.html          # Game lobby
    └── game.html           # Game room

Database: monopoly.db (SQLite)
```

## API Endpoints

### Authentication
- `POST /api/auth/register` - Create new user account
- `POST /api/auth/login` - Login and create session
- `POST /api/auth/logout` - Logout and clear session

### Lobby (Protected)
- `GET /api/lobby/games` - List all available games
- `POST /api/lobby/create` - Create new game
- `POST /api/lobby/join/{gameId}` - Join a game
- `GET /api/lobby/games/{gameId}` - Get game details

### WebSocket (Protected)
- `GET /ws/game/{gameId}` - WebSocket connection for real-time updates

## WebSocket Messages

### Client → Server
```json
{"type": "ready", "payload": {"isReady": true}}
{"type": "end_turn", "payload": {}}
```

### Server → Client
```json
{"type": "player_joined", "payload": {"player": {...}}}
{"type": "player_ready", "payload": {"userId": 1, "isReady": true}}
{"type": "game_started", "payload": {"currentPlayerId": 1}}
{"type": "turn_changed", "payload": {"previousPlayerId": 1, "currentPlayerId": 2}}
{"type": "error", "payload": {"message": "..."}}
```

## Database Schema

### users
- `id` - Primary key
- `username` - Unique, alphanumeric, 3-20 chars
- `password_hash` - bcrypt hash
- `created_at` - Timestamp

### games
- `id` - Primary key
- `status` - waiting/in_progress/finished
- `created_at` - Timestamp
- `max_players` - Default 4

### game_players
- `game_id` + `user_id` - Composite primary key
- `player_order` - Turn order
- `is_ready` - Ready status
- `is_current_turn` - Current turn indicator

## Running the Server

### Build
```bash
go build -o monopoly.exe .
```

### Run
```bash
./monopoly.exe
```

Server will start on `http://localhost:8080`

### Graceful Shutdown
Press `Ctrl+C` to trigger graceful shutdown

## Quick Test Flow

1. **Start Server**: `./monopoly.exe`
2. **Open Browser**: Navigate to `http://localhost:8080`
3. **Register Users**: Create 2-3 test accounts
4. **Create Game**: User 1 creates a game
5. **Join Game**: Users 2-3 join the game
6. **Start Game**: All players click "Ready"
7. **Play Turns**: Current player clicks "End Turn" to advance

## Features Implemented

### Authentication
- Password hashing with bcrypt (cost 10)
- Session-based authentication with secure cookies
- Username validation (alphanumeric, 3-20 characters)
- Password validation (minimum 6 characters)

### Game Flow
1. **Waiting Phase**: Players join and mark themselves ready
2. **Auto-Start**: Game starts when all players (min 2) are ready
3. **Turn System**: Players take turns in order
4. **Real-time Updates**: All connected clients receive updates instantly

### Optimizations
- Prepared SQL statements for all queries
- Connection pooling (max 25 connections)
- WebSocket message batching
- In-memory session cache
- Efficient room-based broadcasting

## Security Features
- Password hashing with bcrypt
- HttpOnly session cookies
- SameSite cookie protection
- SQL injection protection via prepared statements
- Input validation on username/password

## Configuration

Located in `config/config.go`:
- **Server Port**: `:8080`
- **Database Path**: `./monopoly.db`
- **Session Secret**: Auto-generated 32-byte random string
- **Session Expiry**: 7 days

## Dependencies

- `github.com/gorilla/websocket` - WebSocket support
- `github.com/gorilla/mux` - HTTP routing
- `modernc.org/sqlite` - Pure Go SQLite driver
- `golang.org/x/crypto/bcrypt` - Password hashing

## Future Enhancements (Out of MVP Scope)

- Actual Monopoly game mechanics (dice, properties, money)
- Game state persistence/snapshots
- Player reconnection handling
- Comprehensive test suite
- Metrics and monitoring
- Database migrations tool
- Rate limiting
- Enhanced frontend styling
- Mobile responsiveness

## Project Statistics

- **Go Files**: 15
- **HTML Files**: 4
- **Total Packages**: 7
- **Lines of Code**: ~1,500+ (estimated)
- **Build Size**: ~15 MB (includes SQLite driver)

## Notes

- This is an MVP focused on infrastructure, not game mechanics
- Database file (`monopoly.db`) is created on first run
- All sessions are stored in-memory (cleared on restart)
- WebSocket connections auto-reconnect on disconnect
- CORS enabled for development (configure for production)

## Testing Checklist

- [x] Server builds successfully
- [x] Database schema created
- [x] Server starts on port 8080
- [ ] User registration works
- [ ] User login works
- [ ] Game creation works
- [ ] Game joining works
- [ ] WebSocket connection establishes
- [ ] Ready system works
- [ ] Game auto-starts with all ready
- [ ] Turn system advances correctly

---

**Status**: Implementation Complete ✅
**Build**: Successful ✅
**Database**: Initialized ✅
**Server**: Ready to test 🚀
