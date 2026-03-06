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
    container.querySelector('#createGameBtn').addEventListener('click', () => showCreateGameModal(container, router));
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
        ws.close(1000, 'Navigation'); // Send normal closure code
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
            // Full state update (for backward compatibility)
            displayGames(container, message.payload, router);
            break;
        case 'game_created':
            handleGameCreated(container, message.payload, router);
            break;
        case 'game_deleted':
            handleGameDeleted(container, message.payload);
            break;
        case 'player_joined':
            handlePlayerJoined(container, message.payload, router);
            break;
        case 'player_left':
            handlePlayerLeft(container, message.payload, router);
            break;
        case 'game_status_changed':
            handleGameStatusChanged(container, message.payload, router);
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

function getGameButtonHTML(game) {
    if (game.status === 'waiting') {
        if (game.isJoined) {
            return `<button class="leave-game-btn" data-game-id="${game.id}">LEAVE</button>`;
        } else {
            const isFull = game.players.length >= game.maxPlayers;
            return `<button class="join-game-btn" data-game-id="${game.id}" ${isFull ? 'disabled' : ''}>JOIN</button>`;
        }
    } else if (game.status === 'in_progress') {
        if (game.isJoined) {
            return `<button class="enter-game-btn" data-game-id="${game.id}">ENTER</button>`;
        } else {
            return `<button class="spectate-game-btn" disabled>SPECTATE</button>`;
        }
    } else if (game.status === 'finished') {
        return ''; // No button for finished games
    }
    return '';
}

function displayGames(container, games, router) {
    const gamesListDiv = container.querySelector('#gamesList');
    if (!gamesListDiv) return;

    if (!games || games.length === 0) {
        gamesListDiv.innerHTML = '<div class="empty-state">No games available. Create one!</div>';
        return;
    }

    gamesListDiv.innerHTML = games.map(game => `
        <div class="game-item ${game.isJoined ? 'current-game' : ''}" data-game-id="${game.id}">
            <div class="game-info">
                <div class="game-name">GAME #${game.id}</div>
                <div class="game-meta">
                    <span class="players-count">PLAYERS: ${game.players.length}/${game.maxPlayers}</span>
                    <span class="game-status ${game.status === 'in_progress' ? 'in-progress' : game.status === 'finished' ? 'finished' : 'waiting'}">${game.status.toUpperCase()}</span>
                </div>
                <div class="players-list">
                    ${game.players.map(p => `<span class="player-name" data-user-id="${p.userId}">${p.username}</span>`).join(', ')}
                </div>
            </div>
            ${getGameButtonHTML(game)}
        </div>
    `).join('');

    // Add event listeners
    container.querySelectorAll('.join-game-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const gameId = parseInt(btn.dataset.gameId);
            joinGame(gameId, container, router);
        });
    });

    container.querySelectorAll('.leave-game-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const gameId = parseInt(btn.dataset.gameId);
            leaveGame(gameId, container, router);
        });
    });

    container.querySelectorAll('.enter-game-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const gameId = parseInt(btn.dataset.gameId);
            router.navigate(`/game?gameId=${gameId}`);
        });
    });
}

function showCreateGameModal(container, router) {
    const modal = container.querySelector('#createGameModal');
    const form = container.querySelector('#createGameForm');
    const maxPlayersInput = container.querySelector('#maxPlayers');
    const decreaseBtn = container.querySelector('#decreasePlayersBtn');
    const increaseBtn = container.querySelector('#increasePlayersBtn');
    const cancelBtn = container.querySelector('#cancelCreateBtn');

    // Reset to default
    maxPlayersInput.value = 4;

    // Show modal
    modal.style.display = 'flex';

    // Close modal function
    const closeModal = () => {
        modal.style.display = 'none';
        document.removeEventListener('keydown', escHandler);
    };

    // Player count controls
    decreaseBtn.onclick = () => {
        const current = parseInt(maxPlayersInput.value);
        if (current > 2) {
            maxPlayersInput.value = current - 1;
        }
    };

    increaseBtn.onclick = () => {
        const current = parseInt(maxPlayersInput.value);
        if (current < 8) {
            maxPlayersInput.value = current + 1;
        }
    };

    // Handle form submission
    form.onsubmit = async (e) => {
        e.preventDefault();
        const maxPlayers = parseInt(maxPlayersInput.value);
        closeModal();
        await createGame(container, router, maxPlayers);
    };

    // Handle cancel
    cancelBtn.onclick = closeModal;

    // Close on background click
    modal.onclick = (e) => {
        if (e.target === modal) {
            closeModal();
        }
    };

    // Close on ESC key
    const escHandler = (e) => {
        if (e.key === 'Escape') {
            closeModal();
        }
    };
    document.addEventListener('keydown', escHandler);
}

async function createGame(container, router, maxPlayers = 4) {
    showError(container, '');

    try {
        await api.createGame(maxPlayers);
        // Don't navigate - stay in lobby
        // WebSocket will update the game list automatically
    } catch (error) {
        console.error('Failed to create game:', error);
        showError(container, error.message || 'Failed to create game');
    }
}

async function joinGame(gameId, container, router) {
    showError(container, '');

    try {
        await api.joinGame(gameId);
        // WebSocket will handle UI updates via player_joined event
    } catch (error) {
        console.error('Failed to join game:', error);
        showError(container, error.message || 'Failed to join game');
    }
}

async function leaveGame(gameId, container, router) {
    showError(container, '');

    try {
        await api.leaveGame(gameId);
    } catch (error) {
        console.error('Failed to leave game:', error);
        showError(container, error.message || 'Failed to leave game');
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

// Event-based diff handlers

function handleGameCreated(container, payload, router) {
    const gamesListDiv = container.querySelector('#gamesList');
    if (!gamesListDiv) return;

    const game = payload.game;

    // Check if empty state message exists and remove it
    const emptyState = gamesListDiv.querySelector('.empty-state');
    if (emptyState) {
        emptyState.remove();
    }

    // Create new game element
    const gameElement = createGameElement(game, router);

    // Insert at the beginning (newest first)
    gamesListDiv.insertBefore(gameElement, gamesListDiv.firstChild);
}

function handleGameDeleted(container, payload) {
    const gameElement = container.querySelector(`[data-game-id="${payload.gameId}"]`);
    if (gameElement) {
        gameElement.remove();
    }

    // Check if games list is now empty
    const gamesListDiv = container.querySelector('#gamesList');
    if (gamesListDiv && gamesListDiv.children.length === 0) {
        gamesListDiv.innerHTML = '<div class="empty-state">No games available. Create one!</div>';
    }
}

function handlePlayerJoined(container, payload, router) {
    const gameElement = container.querySelector(`[data-game-id="${payload.gameId}"]`);
    if (!gameElement) return;

    // Update players list
    const playersList = gameElement.querySelector('.players-list');
    if (playersList) {
        const playerSpan = document.createElement('span');
        playerSpan.className = 'player-name';
        playerSpan.dataset.userId = payload.player.userId;
        playerSpan.textContent = payload.player.username;

        if (playersList.textContent.trim() === '') {
            playersList.appendChild(playerSpan);
        } else {
            playersList.appendChild(document.createTextNode(', '));
            playersList.appendChild(playerSpan);
        }
    }

    // Update player count
    const playersCount = gameElement.querySelector('.players-count');
    if (playersCount) {
        const match = playersCount.textContent.match(/PLAYERS: (\d+)\/(\d+)/);
        if (match) {
            const current = parseInt(match[1]) + 1;
            const max = match[2];
            playersCount.textContent = `PLAYERS: ${current}/${max}`;
        }
    }

    // If this is you joining, replace button
    if (payload.isYou) {
        gameElement.classList.add('current-game');
        const button = gameElement.querySelector('.join-game-btn');
        if (button) {
            button.outerHTML = `<button class="leave-game-btn" data-game-id="${payload.gameId}">LEAVE</button>`;
            // Re-attach event listener
            const newButton = gameElement.querySelector('.leave-game-btn');
            newButton.addEventListener('click', () => {
                leaveGame(payload.gameId, container, router);
            });
        }
    }

    // Check if game is now full and disable join button for others
    const match = gameElement.querySelector('.players-count')?.textContent.match(/PLAYERS: (\d+)\/(\d+)/);
    if (match) {
        const current = parseInt(match[1]);
        const max = parseInt(match[2]);
        if (current >= max) {
            const joinBtn = gameElement.querySelector('.join-game-btn');
            if (joinBtn) {
                joinBtn.disabled = true;
            }
        }
    }
}

function handlePlayerLeft(container, payload, router) {
    const gameElement = container.querySelector(`[data-game-id="${payload.gameId}"]`);
    if (!gameElement) return;

    // Remove player from the players list
    const playersList = gameElement.querySelector('.players-list');
    if (playersList) {
        const playerSpan = playersList.querySelector(`[data-user-id="${payload.userId}"]`);
        if (playerSpan) {
            playerSpan.remove();

            // Rebuild the comma-separated list by cleaning up text nodes
            const playerSpans = Array.from(playersList.querySelectorAll('.player-name'));

            // Clear all text nodes (commas)
            Array.from(playersList.childNodes).forEach(node => {
                if (node.nodeType === Node.TEXT_NODE) {
                    node.remove();
                }
            });

            // Rebuild with proper comma separators
            playerSpans.forEach((span, index) => {
                if (index > 0) {
                    playersList.appendChild(document.createTextNode(', '));
                }
                playersList.appendChild(span);
            });
        }
    }

    // Update player count
    const playersCount = gameElement.querySelector('.players-count');
    if (playersCount) {
        const match = playersCount.textContent.match(/PLAYERS: (\d+)\/(\d+)/);
        if (match) {
            const current = Math.max(0, parseInt(match[1]) - 1);
            const max = match[2];
            playersCount.textContent = `PLAYERS: ${current}/${max}`;
        }
    }

    // If this is you leaving, replace button
    if (payload.isYou) {
        gameElement.classList.remove('current-game');
        const button = gameElement.querySelector('.leave-game-btn');
        if (button) {
            const gameStatus = gameElement.querySelector('.game-status')?.textContent.toLowerCase();
            const match = playersCount?.textContent.match(/PLAYERS: (\d+)\/(\d+)/);
            const isFull = match && parseInt(match[1]) >= parseInt(match[2]);
            const disabled = gameStatus !== 'waiting' || isFull ? 'disabled' : '';

            button.outerHTML = `<button class="join-game-btn" data-game-id="${payload.gameId}" ${disabled}>JOIN</button>`;

            // Re-attach event listener
            const newButton = gameElement.querySelector('.join-game-btn');
            newButton.addEventListener('click', () => {
                joinGame(payload.gameId, container, router);
            });
        }
    } else {
        // Enable join button if game is no longer full
        const joinBtn = gameElement.querySelector('.join-game-btn');
        if (joinBtn && joinBtn.disabled) {
            const gameStatus = gameElement.querySelector('.game-status')?.textContent.toLowerCase();
            if (gameStatus === 'waiting') {
                joinBtn.disabled = false;
            }
        }
    }
}

function handleGameStatusChanged(container, payload, router) {
    const gameElement = container.querySelector(`[data-game-id="${payload.gameId}"]`);
    if (!gameElement) return;

    const isUserInGame = gameElement.classList.contains('current-game');

    // Auto-redirect user to game if it started and they're in it
    if (payload.status === 'in_progress' && isUserInGame) {
        router.navigate(`/game?gameId=${payload.gameId}`);
        return;
    }

    // Remove finished games from the lobby
    if (payload.status === 'finished') {
        gameElement.remove();

        // Check if games list is now empty
        const gamesListDiv = container.querySelector('#gamesList');
        if (gamesListDiv && gamesListDiv.children.length === 0) {
            gamesListDiv.innerHTML = '<div class="empty-state">No games available. Create one!</div>';
        }
        return;
    }

    const statusElement = gameElement.querySelector('.game-status');
    if (statusElement) {
        statusElement.textContent = payload.status.toUpperCase();
        statusElement.className = `game-status ${payload.status === 'in_progress' ? 'in-progress' : 'waiting'}`;
    }

    // Update buttons based on status
    if (payload.status === 'in_progress') {
        // Replace JOIN button with SPECTATE button (disabled)
        const joinBtn = gameElement.querySelector('.join-game-btn');
        if (joinBtn) {
            joinBtn.outerHTML = '<button class="spectate-game-btn" disabled>SPECTATE</button>';
        }

        // If user is in game, show ENTER button instead of LEAVE
        if (isUserInGame) {
            const leaveBtn = gameElement.querySelector('.leave-game-btn');
            if (leaveBtn) {
                leaveBtn.outerHTML = `<button class="enter-game-btn" data-game-id="${payload.gameId}">ENTER</button>`;
                const enterBtn = gameElement.querySelector('.enter-game-btn');
                enterBtn.addEventListener('click', () => {
                    router.navigate(`/game?gameId=${payload.gameId}`);
                });
            }
        }
    }
}

function createGameElement(game, router) {
    const div = document.createElement('div');
    div.className = `game-item ${game.isJoined ? 'current-game' : ''}`;
    div.dataset.gameId = game.id;

    div.innerHTML = `
        <div class="game-info">
            <div class="game-name">GAME #${game.id}</div>
            <div class="game-meta">
                <span class="players-count">PLAYERS: ${game.players.length}/${game.maxPlayers}</span>
                <span class="game-status ${game.status === 'in_progress' ? 'in-progress' : game.status === 'finished' ? 'finished' : 'waiting'}">${game.status.toUpperCase()}</span>
            </div>
            <div class="players-list">
                ${game.players.map(p => `<span class="player-name" data-user-id="${p.userId}">${p.username}</span>`).join(', ')}
            </div>
        </div>
        ${getGameButtonHTML(game)}
    `;

    // Attach event listeners
    const joinBtn = div.querySelector('.join-game-btn');
    if (joinBtn) {
        joinBtn.addEventListener('click', () => {
            joinGame(game.id, div.closest('.container') || document.body, router);
        });
    }

    const leaveBtn = div.querySelector('.leave-game-btn');
    if (leaveBtn) {
        leaveBtn.addEventListener('click', () => {
            leaveGame(game.id, div.closest('.container') || document.body, router);
        });
    }

    const enterBtn = div.querySelector('.enter-game-btn');
    if (enterBtn) {
        enterBtn.addEventListener('click', () => {
            router.navigate(`/game?gameId=${game.id}`);
        });
    }

    return div;
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
