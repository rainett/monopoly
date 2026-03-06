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
    container.querySelector('#rollDiceBtn').addEventListener('click', rollDice);
    container.querySelector('#buyBtn').addEventListener('click', buyProperty);
    container.querySelector('#passBtn').addEventListener('click', passProperty);

    const chatInput = container.querySelector('#chatInput');
    const sendChatBtn = container.querySelector('#sendChatBtn');

    sendChatBtn.addEventListener('click', () => sendChatMessage(chatInput, container));
    chatInput.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            sendChatMessage(chatInput, container);
        }
    });

    connectWebSocket(gameId, user.userId, container);
}

export function cleanup() {
    if (reconnectTimeout) {
        clearTimeout(reconnectTimeout);
        reconnectTimeout = null;
    }

    if (ws) {
        ws.onclose = null;
        ws.close(1000, 'Navigation');
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
        if (ws !== null) {
            reconnectTimeout = setTimeout(() => {
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
        updateBoard(gameState, container);
        updateUI(gameState, userId, container);
    } catch (error) {
        console.error('Failed to load game state:', error);
    }
}

function handleWebSocketMessage(message, gameId, userId, container) {
    switch (message.type) {
        case 'player_joined':
            addLog(`${message.payload.player.username} joined the game`, 'event', container);
            loadGameState(gameId, userId, container);
            break;

        case 'game_started':
            addLog('Game started!', 'event', container);
            loadGameState(gameId, userId, container);
            break;

        case 'turn_changed':
            updateTurnFromPayload(message.payload, userId, container);
            addLog(`Turn changed to ${getPlayerName(message.payload.currentPlayerId)}`, 'event', container);
            break;

        case 'turn_timeout':
            updateTurnFromPayload(message.payload, userId, container);
            addLog('Turn timeout - automatically skipped', 'system', container);
            break;

        case 'dice_rolled': {
            const p = message.payload;
            const name = getPlayerName(p.userId);
            addLog(`${name} rolled ${p.die1} + ${p.die2} = ${p.total}`, 'event', container);
            if (p.passedGo) {
                addLog(`${name} passed GO - collected $200`, 'event', container);
            }
            addLog(`${name} landed on ${p.spaceName}`, 'event', container);
            // Update player position in local state
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) {
                    player.position = p.newPos;
                    player.hasRolled = true;
                    if (p.passedGo) player.money += 200;
                }
                updateBoard(gameState, container);
                updateControls(userId, container);
                showDiceResult(p.die1, p.die2, container);
            }
            break;
        }

        case 'buy_prompt': {
            const p = message.payload;
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) player.pendingAction = 'buy_or_pass';
            }
            if (p.userId === userId) {
                showBuyPrompt(p.name, p.price, container);
            } else {
                addLog(`${getPlayerName(p.userId)} can buy ${p.name} for $${p.price}`, 'event', container);
            }
            updateControls(userId, container);
            break;
        }

        case 'property_bought': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} bought ${p.name} for $${p.price}`, 'event', container);
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) {
                    player.money = p.newMoney;
                    player.pendingAction = '';
                }
                if (!gameState.properties) gameState.properties = {};
                gameState.properties[p.position] = p.userId;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            hideBuyPrompt(container);
            break;
        }

        case 'property_passed': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} passed on ${p.name}`, 'event', container);
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) player.pendingAction = '';
            }
            hideBuyPrompt(container);
            updateControls(userId, container);
            break;
        }

        case 'rent_paid': {
            const p = message.payload;
            addLog(`${getPlayerName(p.payerId)} paid $${p.amount} rent to ${getPlayerName(p.ownerId)} for ${p.name}`, 'event', container);
            if (gameState) {
                const payer = gameState.players.find(pl => pl.userId === p.payerId);
                const owner = gameState.players.find(pl => pl.userId === p.ownerId);
                if (payer) payer.money = p.payerMoney;
                if (owner) owner.money = p.ownerMoney;
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'tax_paid': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} paid $${p.amount} in taxes`, 'event', container);
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) player.money = p.newMoney;
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'go_to_jail': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} was sent to Jail!`, 'event', container);
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) player.position = 10;
                updateBoard(gameState, container);
            }
            break;
        }

        case 'player_bankrupt': {
            const p = message.payload;
            addLog(`${p.username} went bankrupt! (${p.reason})`, 'event', container);
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) {
                    player.isBankrupt = true;
                    player.money = 0;
                }
                // Remove their properties
                if (gameState.properties) {
                    for (const [pos, ownerId] of Object.entries(gameState.properties)) {
                        if (ownerId === p.userId) {
                            delete gameState.properties[pos];
                        }
                    }
                }
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'game_finished':
            addLog('Game Over!', 'event', container);
            if (gameState) gameState.status = 'finished';
            showGameOver(message.payload, container);
            break;

        case 'error':
            addLog(`Error: ${message.payload.message}`, 'system', container);
            break;
    }
}

function updateTurnFromPayload(payload, userId, container) {
    if (!gameState) return;

    const currentPlayerId = payload.currentPlayerId || payload.currentPlayerID;
    if (!currentPlayerId) return;

    gameState.currentPlayerId = currentPlayerId;
    gameState.players.forEach(player => {
        player.isCurrentTurn = player.userId === currentPlayerId;
        if (player.userId === currentPlayerId) {
            player.hasRolled = false;
            player.pendingAction = '';
        }
    });

    hideBuyPrompt(container);
    hideDiceResult(container);
    updateUI(gameState, userId, container);
}

function updateBoard(state, container) {
    if (!state || !state.board) return;

    // Populate space names and colors from board data
    state.board.forEach(space => {
        const el = container.querySelector(`[data-space="${space.position}"]`);
        if (!el) return;

        const nameEl = el.querySelector('.space-name');
        if (nameEl && space.position !== 0 && space.position !== 10 && space.position !== 20 && space.position !== 30) {
            nameEl.textContent = space.name;
        }

        // Set color bar
        const colorEl = el.querySelector('.space-color');
        if (colorEl && space.color) {
            colorEl.className = 'space-color ' + space.color;
        }
    });

    // Clear existing tokens and ownership bars
    container.querySelectorAll('.player-tokens, .ownership-bar').forEach(el => el.remove());

    // Group players by position
    const positionPlayers = {};
    state.players.forEach((player, idx) => {
        if (player.isBankrupt) return;
        if (!positionPlayers[player.position]) positionPlayers[player.position] = [];
        positionPlayers[player.position].push({ ...player, colorIndex: idx });
    });

    // Render player tokens
    for (const [pos, players] of Object.entries(positionPlayers)) {
        const el = container.querySelector(`[data-space="${pos}"]`);
        if (!el) continue;

        const tokensDiv = document.createElement('div');
        tokensDiv.className = 'player-tokens';
        players.forEach(p => {
            const token = document.createElement('div');
            token.className = `player-token color-${p.colorIndex}`;
            token.title = p.username;
            tokensDiv.appendChild(token);
        });
        el.appendChild(tokensDiv);
    }

    // Render ownership indicators
    if (state.properties) {
        for (const [pos, ownerId] of Object.entries(state.properties)) {
            const el = container.querySelector(`[data-space="${pos}"]`);
            if (!el) continue;

            const ownerIdx = state.players.findIndex(p => p.userId === ownerId);
            if (ownerIdx === -1) continue;

            const bar = document.createElement('div');
            bar.className = `ownership-bar owner-${ownerIdx}`;
            el.appendChild(bar);
        }
    }
}

function updateUI(state, userId, container) {
    if (!state) return;

    container.querySelector('#gameStatus').textContent = state.status;

    const playersListDiv = container.querySelector('#playersList');
    playersListDiv.innerHTML = state.players.map((player, idx) => `
        <div class="player-item ${player.isCurrentTurn ? 'current-turn' : ''} ${player.isBankrupt ? 'bankrupt' : ''}">
            <div class="player-name">
                <span class="player-color-dot" style="background-color:${['#FF4444','#4444FF','#44FF44','#FFFF44'][idx]}"></span>
                ${player.username}${player.userId === userId ? ' (You)' : ''}
            </div>
            <div class="player-money">${player.isBankrupt ? 'BANKRUPT' : '$' + player.money}</div>
        </div>
    `).join('');

    const currentTurnDiv = container.querySelector('#currentTurn');
    if (state.status === 'in_progress') {
        const currentPlayer = state.players.find(p => p.isCurrentTurn);
        if (currentPlayer) {
            currentTurnDiv.textContent = `Current turn: ${currentPlayer.username}`;
        }
    } else {
        currentTurnDiv.textContent = '';
    }

    updateControls(userId, container);
}

function updateControls(userId, container) {
    if (!gameState) return;

    const rollBtn = container.querySelector('#rollDiceBtn');
    const endTurnBtn = container.querySelector('#endTurnBtn');

    if (gameState.status !== 'in_progress') {
        rollBtn.disabled = true;
        endTurnBtn.disabled = true;
        return;
    }

    const me = gameState.players.find(p => p.userId === userId);
    if (!me) return;

    const isMyTurn = me.isCurrentTurn && !me.isBankrupt;

    // Roll dice: enabled if my turn, haven't rolled, no pending action
    rollBtn.disabled = !(isMyTurn && !me.hasRolled && !me.pendingAction);

    // End turn: enabled if my turn, have rolled, no pending action
    endTurnBtn.disabled = !(isMyTurn && me.hasRolled && !me.pendingAction);
}

function showDiceResult(die1, die2, container) {
    const el = container.querySelector('#diceResult');
    if (el) {
        el.textContent = `Dice: [${die1}] [${die2}] = ${die1 + die2}`;
        el.style.display = 'block';
    }
}

function hideDiceResult(container) {
    const el = container.querySelector('#diceResult');
    if (el) el.style.display = 'none';
}

function showBuyPrompt(name, price, container) {
    const prompt = container.querySelector('#buyPrompt');
    const text = container.querySelector('#buyPromptText');
    if (prompt && text) {
        text.textContent = `Buy ${name} for $${price}?`;
        prompt.style.display = 'block';
    }
}

function hideBuyPrompt(container) {
    const prompt = container.querySelector('#buyPrompt');
    if (prompt) prompt.style.display = 'none';
}

function rollDice() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'roll_dice', payload: {} }));
}

function buyProperty() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'buy_property', payload: {} }));
}

function passProperty() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'pass_property', payload: {} }));
}

function endTurn() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'end_turn', payload: {} }));
}

function sendChatMessage(input, container) {
    const message = input.value.trim();
    if (!message) return;
    addLog(`[Chat] You: ${message}`, 'system', container);
    input.value = '';
}

function getPlayerName(userId) {
    if (!gameState) return `Player ${userId}`;
    const player = gameState.players.find(p => p.userId === userId);
    return player ? player.username : `Player ${userId}`;
}

function addLog(message, type = 'event', container) {
    const logDiv = container.querySelector('#gameLog');
    if (!logDiv) return;

    const entry = document.createElement('div');
    entry.className = `log-entry ${type}`;
    entry.textContent = message;
    logDiv.appendChild(entry);

    const maxEntries = 100;
    while (logDiv.children.length > maxEntries) {
        logDiv.removeChild(logDiv.firstChild);
    }

    logDiv.scrollTop = logDiv.scrollHeight;
}

function showGameOver(payload, container) {
    const rollBtn = container.querySelector('#rollDiceBtn');
    const endTurnBtn = container.querySelector('#endTurnBtn');
    if (rollBtn) rollBtn.style.display = 'none';
    if (endTurnBtn) endTurnBtn.style.display = 'none';
    hideBuyPrompt(container);

    const winner = payload.players.find(p => p.userId === payload.winnerId);
    const winnerName = winner ? winner.username : 'Unknown';

    const overlay = document.createElement('div');
    overlay.className = 'game-over-overlay';
    overlay.innerHTML = `
        <div class="game-over-modal">
            <h2>Game Over!</h2>
            <h3>Winner: ${winnerName}</h3>
            <div class="results-list">
                ${payload.players
                    .sort((a, b) => (b.money || 0) - (a.money || 0))
                    .map((player, index) => `
                    <div class="result-item">
                        <span class="result-rank">#${index + 1}</span>
                        <span class="result-name">${player.username}</span>
                        <span class="result-money" style="margin-left:auto;color:#858585;">$${player.money || 0}</span>
                    </div>
                `).join('')}
            </div>
            <button id="returnToLobbyBtn" class="primary-btn">Return to Lobby</button>
        </div>
    `;

    container.appendChild(overlay);

    const returnBtn = overlay.querySelector('#returnToLobbyBtn');
    returnBtn.addEventListener('click', () => {
        cleanup();
        window.location.hash = '#/lobby';
    });
}
