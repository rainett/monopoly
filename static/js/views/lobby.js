import { api } from '../api.js';
import { templateLoader } from '../template.js';

let ws = null;
let reconnectTimeout = null;

export async function render(container, router) {
    if (!api.isAuthenticated()) {
        router.navigate('/login');
        return;
    }

    const user = api.getCurrentUser();
    const template = await templateLoader.load('lobby');
    container.innerHTML = template;

    container.querySelector('#username').textContent = user.username;
    container.querySelector('#createGameBtn').addEventListener('click', () => createGame(container, router));
    container.querySelector('#logoutBtn').addEventListener('click', () => logout(router));

    // Initial load of games
    loadGames(container, router);

    // Connect to lobby WebSocket
    connectLobbyWebSocket(container, router);
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
}

function connectLobbyWebSocket(container, router) {
    const wsURL = api.getWebSocketURL('lobby');

    try {
        ws = new WebSocket(wsURL);
    } catch (error) {
        console.error('WebSocket connection error:', error);
        scheduleReconnect(container, router);
        return;
    }

    ws.onopen = () => {
        console.log('Lobby WebSocket connected');
    };

    ws.onmessage = (event) => {
        try {
            const message = JSON.parse(event.data);
            handleWebSocketMessage(message, container, router);
        } catch (error) {
            console.error('Error parsing WebSocket message:', error);
        }
    };

    ws.onerror = (error) => {
        console.error('Lobby WebSocket error:', error);
    };

    ws.onclose = () => {
        console.log('Lobby WebSocket disconnected');
        ws = null;
        scheduleReconnect(container, router);
    };
}

function scheduleReconnect(container, router) {
    if (reconnectTimeout) return;

    reconnectTimeout = setTimeout(() => {
        reconnectTimeout = null;
        if (!ws && api.isAuthenticated()) {
            connectLobbyWebSocket(container, router);
        }
    }, 3000);
}

function handleWebSocketMessage(message, container, router) {
    switch (message.type) {
        case 'games_update':
            displayGames(container, message.payload, router);
            break;
        default:
            console.log('Unknown message type:', message.type);
    }
}

async function loadGames(container, router) {
    try {
        const games = await api.listGames();
        displayGames(container, games, router);
    } catch (error) {
        console.error('Failed to load games:', error);
        const gamesListDiv = container.querySelector('#gamesList');
        if (gamesListDiv) {
            gamesListDiv.innerHTML = '<div class="error">FAILED TO LOAD GAMES<button onclick="location.reload()">RETRY</button></div>';
        }
    }
}

function displayGames(container, games, router) {
    const gamesListDiv = container.querySelector('#gamesList');
    if (!gamesListDiv) return;

    if (!games || games.length === 0) {
        gamesListDiv.innerHTML = '<div class="empty-state">No games available. Create one!</div>';
        return;
    }

    gamesListDiv.innerHTML = games.map(game => `
        <div class="game-item">
            <div class="game-info">
                <div class="game-name">GAME #${game.id}</div>
                <div class="game-meta">
                    <span>PLAYERS: ${game.players.length}/${game.maxPlayers}: 
                      <span class="players-list">${game.players.map(player => player.username)}</span>
                    </span>
                    <span class="game-status ${game.status === 'in_progress' ? 'in-progress' : 'waiting'}">${game.status.toUpperCase()}</span>
                </div>
            </div>
            <button
                class="join-game-btn"
                data-game-id="${game.id}"
                ${game.status !== 'waiting' || game.players.length >= game.maxPlayers ? 'hidden' : ''}>
                Join
            </button>
            <button
                class="enter-game-btn"
                data-game-id="${game.id}"
                ${game.status !== 'in_game' ? 'hidden' : ''}>
                Join
            </button>
        </div>
    `).join('');

    container.querySelectorAll('.join-game-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const gameId = btn.dataset.gameId;
            joinGame(parseInt(gameId), container, router);
        });
    });
}

async function createGame(container, router) {
    const errorDiv = showError(container, '');

    try {
        const data = await api.createGame();
        await joinGame(data.gameId, container, router);
    } catch (error) {
        console.error('Failed to create game:', error);
        showError(container, error.message || 'Failed to create game');
    }
}

async function joinGame(gameId, container, router) {
    const errorDiv = showError(container, '');

    try {
        await api.joinGame(gameId);
        cleanup();
        router.navigate('/game', { gameId });
    } catch (error) {
        console.error('Failed to join game:', error);
        showError(container, error.message || 'Failed to join game');
    }
}

function showError(container, message) {
    let errorDiv = container.querySelector('.lobby-error');

    if (!errorDiv) {
        errorDiv = document.createElement('div');
        errorDiv.className = 'error lobby-error';
        const gamesContainer = container.querySelector('.games-container');
        if (gamesContainer) {
            gamesContainer.insertBefore(errorDiv, gamesContainer.firstChild);
        }
    }

    if (message) {
        errorDiv.textContent = message;
        errorDiv.style.display = 'block';

        // Auto-hide after 5 seconds
        setTimeout(() => {
            errorDiv.style.display = 'none';
        }, 5000);
    } else {
        errorDiv.style.display = 'none';
    }

    return errorDiv;
}

async function logout(router) {
    try {
        await api.logout();
    } catch (error) {
        console.error('Logout failed:', error);
    }
    cleanup();
    router.navigate('/login');
}
