import { api } from '../api.js';
import { templateLoader } from '../template.js';

let ws = null;
let gameState = null;
let reconnectTimeout = null;

export async function render(container, router) {
    const params = router.getCurrentRoute()?.params;
    const gameIdParam = params?.get('gameId');
    const gameId = parseInt(gameIdParam);
    const user = api.getCurrentUser();

    if (!gameIdParam || isNaN(gameId) || !user.userId || !user.username) {
        console.warn('Invalid game access attempt:', { gameIdParam, gameId, user });
        router.navigate('/lobby');
        return;
    }

    const template = await templateLoader.load('game');
    container.innerHTML = template;

    container.querySelector('#gameId').textContent = gameId;
    container.querySelector('#backBtn').addEventListener('click', () => {
        cleanup();
        router.navigate('/lobby');
    });

    container.querySelector('#endTurnBtn').addEventListener('click', endTurn);

    connectWebSocket(gameId, user.userId, container);
}

export function cleanup() {
    if (reconnectTimeout) {
        clearTimeout(reconnectTimeout);
        reconnectTimeout = null;
    }

    if (ws) {
        ws.onclose = null;
        ws.close();
        ws = null;
    }

    gameState = null;
}

function connectWebSocket(gameId, userId, container) {
    const wsURL = api.getWebSocketURL(gameId);
    ws = new WebSocket(wsURL);

    ws.onopen = () => {
        addLog('Connected to game', 'system', container);
        loadGameState(gameId, userId, container);
    };

    ws.onmessage = (event) => {
        const message = JSON.parse(event.data);
        handleWebSocketMessage(message, gameId, userId, container);
    };

    ws.onerror = (error) => {
        addLog('WebSocket error', 'system', container);
        console.error('WebSocket error:', error);
    };

    ws.onclose = () => {
        addLog('Disconnected from game', 'system', container);

        // Only reconnect if we haven't been cleaned up
        if (ws !== null) {
            reconnectTimeout = setTimeout(() => {
                // Double-check we're still active before reconnecting
                if (ws !== null) {
                    connectWebSocket(gameId, userId, container);
                }
            }, 3000);
        }
    };
}

async function loadGameState(gameId, userId, container) {
    try {
        gameState = await api.getGame(gameId);
        updateUI(gameState, userId, container);
    } catch (error) {
        console.error('Failed to load game state:', error);
    }
}

function handleWebSocketMessage(message, gameId, userId, container) {
    switch (message.type) {
        case 'player_joined':
            addLog(`${message.payload.player.username} joined the game`, 'event', container);
            // TODO: Update UI directly from payload instead of refetching
            loadGameState(gameId, userId, container);
            break;

        case 'game_started':
            addLog('Game started!', 'event', container);
            // Game started - need full state to show current turn
            loadGameState(gameId, userId, container);
            break;

        case 'turn_changed':
            // Update UI directly from payload without refetching
            updateTurnFromPayload(message.payload, userId, container);
            addLog('Turn changed', 'event', container);
            break;

        case 'game_finished':
            addLog('Game Over!', 'event', container);
            showGameOver(message.payload, container);
            break;

        case 'error':
            addLog(`Error: ${message.payload.message}`, 'system', container);
            break;
    }
}

function updateTurnFromPayload(payload, userId, container) {
    // Update turn display without full API call
    if (!gameState) return;

    // Update which player has current turn
    gameState.players.forEach(player => {
        player.isCurrentTurn = player.userId === payload.currentPlayerId;
    });

    // Update UI
    updateUI(gameState, userId, container);
}

function updateUI(state, userId, container) {
    if (!state) return;

    container.querySelector('#gameStatus').textContent = state.status;

    const playersListDiv = container.querySelector('#playersList');
    playersListDiv.innerHTML = state.players.map(player => `
        <div class="player-item ${player.isCurrentTurn ? 'current-turn' : ''}">
            <div>
                <strong>${player.username}</strong>
                ${player.userId === userId ? ' (You)' : ''}
            </div>
        </div>
    `).join('');

    const currentTurnDiv = container.querySelector('#currentTurn');
    if (state.status === 'in_progress') {
        const currentPlayer = state.players.find(p => p.isCurrentTurn);
        if (currentPlayer) {
            currentTurnDiv.textContent = `Current turn: ${currentPlayer.username}`;

            const endTurnBtn = container.querySelector('#endTurnBtn');
            endTurnBtn.disabled = currentPlayer.userId !== userId;
        }
    } else {
        currentTurnDiv.textContent = '';
        container.querySelector('#endTurnBtn').disabled = true;
    }
}

function endTurn() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    ws.send(JSON.stringify({
        type: 'end_turn',
        payload: {}
    }));
}

function addLog(message, type = 'event', container) {
    const logDiv = container.querySelector('#gameLog');
    if (!logDiv) return;

    const entry = document.createElement('div');
    entry.className = `log-entry ${type}`;
    const timestamp = new Date().toLocaleTimeString();
    entry.textContent = `[${timestamp}] ${message}`;
    logDiv.appendChild(entry);

    // Keep only last 100 entries to prevent unbounded growth
    const maxEntries = 100;
    while (logDiv.children.length > maxEntries) {
        logDiv.removeChild(logDiv.firstChild);
    }

    logDiv.scrollTop = logDiv.scrollHeight;
}

function showGameOver(payload, container) {
    // Hide game controls
    const endTurnBtn = container.querySelector('#endTurnBtn');
    if (endTurnBtn) endTurnBtn.style.display = 'none';

    // Create game over overlay
    const overlay = document.createElement('div');
    overlay.className = 'game-over-overlay';
    overlay.innerHTML = `
        <div class="game-over-modal">
            <h2>Game Over!</h2>
            <h3>Final Results</h3>
            <div class="results-list">
                ${payload.players.map((player, index) => `
                    <div class="result-item">
                        <span class="result-rank">#${index + 1}</span>
                        <span class="result-name">${player.username}</span>
                    </div>
                `).join('')}
            </div>
            <button id="returnToLobbyBtn" class="primary-btn">Return to Lobby</button>
        </div>
    `;

    container.appendChild(overlay);

    // Add event listener to return button
    const returnBtn = overlay.querySelector('#returnToLobbyBtn');
    returnBtn.addEventListener('click', () => {
        cleanup();
        // Get router from app - need to navigate back
        window.location.hash = '#/lobby';
    });
}
