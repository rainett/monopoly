# Monopoly MVP - Quick Start Guide

## Start the Server

```bash
./monopoly.exe
```

Expected output:
```
Starting Monopoly server...
Configuration loaded - Server port: :8080, DB path: ./monopoly.db
Database initialized successfully
Server listening on http://localhost:8080
```

## Test the Application

### 1. Open Your Browser
Navigate to: `http://localhost:8080`

### 2. Register Users
- Click "Create Account"
- Create 2-3 test users:
  - Username: `player1`, Password: `pass123`
  - Username: `player2`, Password: `pass123`
  - Username: `player3`, Password: `pass123`

### 3. Login
- Login as `player1`
- You'll be redirected to the lobby

### 4. Create a Game
- Click "Create Game"
- You'll be automatically joined and redirected to the game room

### 5. Join with Other Players
- Open new incognito/private browser windows
- Login as `player2` and `player3`
- In the lobby, click "Join" on the available game

### 6. Start the Game
- In each browser window, click the "Ready" button
- Once all players are ready, the game will auto-start
- The first player's turn will be highlighted

### 7. Play Turns
- The current player clicks "End Turn"
- Watch the turn indicator move to the next player
- All browsers update in real-time via WebSocket

## API Testing with curl

### Register a User
```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"testpass123"}'
```

### Login
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"testpass123"}' \
  -c cookies.txt
```

### List Games (requires login)
```bash
curl -X GET http://localhost:8080/api/lobby/games \
  -b cookies.txt
```

### Create Game (requires login)
```bash
curl -X POST http://localhost:8080/api/lobby/create \
  -H "Content-Type: application/json" \
  -d '{"maxPlayers":4}' \
  -b cookies.txt
```

## Stop the Server

Press `Ctrl+C` in the terminal running the server.

Expected output:
```
Shutting down gracefully...
Server stopped
```

## Troubleshooting

### Port 8080 Already in Use
```bash
# Windows
netstat -ano | findstr :8080
taskkill /PID <PID> /F

# Or change the port in config/config.go and rebuild
```

### Database Issues
```bash
# Delete and recreate the database
rm monopoly.db
./monopoly.exe
```

### WebSocket Connection Fails
- Check browser console for errors
- Ensure you're logged in (session cookie present)
- Verify the game ID in the URL

## File Locations

- **Binary**: `./monopoly.exe`
- **Database**: `./monopoly.db`
- **Logs**: stdout (terminal)

## Development

### Rebuild After Changes
```bash
go build -o monopoly.exe .
```

### Run with Debug Logging
```bash
# The server already logs all HTTP requests
./monopoly.exe
```

### Check Database
```bash
# If sqlite3 is installed
sqlite3 monopoly.db

# Inside sqlite shell:
.tables
SELECT * FROM users;
SELECT * FROM games;
SELECT * FROM game_players;
.quit
```

## What to Test

1. ✅ User registration with validation
2. ✅ Login and session management
3. ✅ Game creation
4. ✅ Multiple players joining
5. ✅ Ready status updates in real-time
6. ✅ Game auto-start when all ready
7. ✅ Turn advancement
8. ✅ Real-time WebSocket updates
9. ✅ Graceful disconnection handling
10. ✅ Logout functionality

---

Happy testing! 🎲🏠
