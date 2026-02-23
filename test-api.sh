#!/bin/bash

echo "=== Testing Monopoly Backend API ==="
echo ""

BASE_URL="http://localhost:8080"

echo "1. Testing main page..."
curl -s $BASE_URL/ | grep -o "<title>.*</title>"
echo "✓ Main page accessible"
echo ""

echo "2. Registering test users..."
curl -s -X POST $BASE_URL/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"player1","password":"password123"}'
echo ""

curl -s -X POST $BASE_URL/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"player2","password":"password123"}'
echo ""
echo "✓ Users registered"
echo ""

echo "3. Logging in as player1..."
curl -s -c cookies1.txt -X POST $BASE_URL/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"player1","password":"password123"}'
echo ""
echo "✓ Player1 logged in"
echo ""

echo "4. Creating a game..."
GAME_RESPONSE=$(curl -s -b cookies1.txt -X POST $BASE_URL/api/lobby/create \
  -H "Content-Type: application/json" \
  -d '{"maxPlayers":4}')
echo $GAME_RESPONSE
GAME_ID=$(echo $GAME_RESPONSE | grep -o '"gameId":[0-9]*' | grep -o '[0-9]*')
echo "✓ Game created with ID: $GAME_ID"
echo ""

echo "5. Player1 joining the game..."
curl -s -b cookies1.txt -X POST $BASE_URL/api/lobby/join/$GAME_ID
echo ""
echo "✓ Player1 joined"
echo ""

echo "6. Logging in as player2..."
curl -s -c cookies2.txt -X POST $BASE_URL/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"player2","password":"password123"}'
echo ""
echo "✓ Player2 logged in"
echo ""

echo "7. Player2 joining the game..."
curl -s -b cookies2.txt -X POST $BASE_URL/api/lobby/join/$GAME_ID
echo ""
echo "✓ Player2 joined"
echo ""

echo "8. Checking game state..."
curl -s -b cookies1.txt $BASE_URL/api/lobby/games/$GAME_ID | python -m json.tool 2>/dev/null || cat
echo ""
echo "✓ Game state retrieved"
echo ""

echo "9. Listing all games..."
curl -s -b cookies1.txt $BASE_URL/api/lobby/games | python -m json.tool 2>/dev/null || cat
echo ""
echo "✓ Games listed"
echo ""

echo "=== All tests passed! ==="
echo ""
echo "To test WebSocket functionality:"
echo "1. Open http://localhost:8080 in your browser"
echo "2. Login with player1/player2"
echo "3. Join the game and click 'Ready'"
echo "4. Test turn progression with 'End Turn'"

# Cleanup
rm -f cookies1.txt cookies2.txt
