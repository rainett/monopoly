import { api } from '../api.js';
import { templateLoader } from '../template.js';

let ws = null;
let gameState = null;
let reconnectTimeout = null;
let hasJailCard = false; // Track if current user has a jail card
let pendingTrades = []; // Track incoming trade offers
let currentUserId = null; // Store current user ID for trade UI
let turnTimerInterval = null; // Timer display interval
let turnTimerEnd = null; // When the current turn timer expires
let turnTimerDuration = 60; // Total duration in seconds
let activeAuction = null; // Current auction state
let reconnectAttempts = 0; // Reconnection attempts counter
const maxReconnectAttempts = 10; // Maximum reconnection attempts
const baseReconnectDelay = 1000; // Base delay in ms

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
    container.querySelector('#payBailBtn').addEventListener('click', payJailBail);
    container.querySelector('#useJailCardBtn').addEventListener('click', useJailCard);
    container.querySelector('#tradeBtn').addEventListener('click', () => openTradeModal(container));

    const chatInput = container.querySelector('#chatInput');
    const sendChatBtn = container.querySelector('#sendChatBtn');

    sendChatBtn.addEventListener('click', () => sendChatMessage(chatInput, container));
    chatInput.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            sendChatMessage(chatInput, container);
        }
    });

    currentUserId = user.userId;
    connectWebSocket(gameId, user.userId, container);

    // Request notification permission
    requestNotificationPermission();
}

export function cleanup() {
    if (reconnectTimeout) {
        clearTimeout(reconnectTimeout);
        reconnectTimeout = null;
    }

    if (turnTimerInterval) {
        clearInterval(turnTimerInterval);
        turnTimerInterval = null;
    }

    if (ws) {
        ws.onclose = null;
        ws.close(1000, 'Navigation');
        ws = null;
    }

    gameState = null;
    pendingTrades = [];
    currentUserId = null;
    hasJailCard = false;
    turnTimerEnd = null;
    activeAuction = null;
    reconnectAttempts = 0;
}

function connectWebSocket(gameId, userId, container) {
    const wsURL = api.getWebSocketURL(gameId);
    ws = new WebSocket(wsURL);

    ws.onopen = () => {
        addLog('Connected to game', 'system', container);
        reconnectAttempts = 0; // Reset reconnect attempts on successful connection
        hideReconnectIndicator(container);
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
        if (ws !== null && reconnectAttempts < maxReconnectAttempts) {
            reconnectAttempts++;
            // Exponential backoff with jitter: 1s, 2s, 4s, 8s... max 30s
            const delay = Math.min(baseReconnectDelay * Math.pow(2, reconnectAttempts - 1), 30000);
            const jitter = Math.random() * 1000; // Add up to 1s of jitter
            showReconnectIndicator(reconnectAttempts, delay, container);
            addLog(`Reconnecting in ${Math.round(delay/1000)}s (attempt ${reconnectAttempts}/${maxReconnectAttempts})...`, 'system', container);
            reconnectTimeout = setTimeout(() => {
                if (ws !== null) {
                    connectWebSocket(gameId, userId, container);
                }
            }, delay + jitter);
        } else if (reconnectAttempts >= maxReconnectAttempts) {
            addLog('Max reconnection attempts reached. Please refresh the page.', 'system', container);
            showReconnectIndicator(-1, 0, container); // -1 indicates max attempts reached
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
            // Send browser notification if it's the user's turn and tab is not focused
            if (message.payload.currentPlayerId === userId) {
                sendTurnNotification();
            }
            break;

        case 'turn_timeout':
            updateTurnFromPayload(message.payload, userId, container);
            addLog('Turn timeout - automatically skipped', 'system', container);
            break;

        case 'dice_rolled': {
            const p = message.payload;
            const name = getPlayerName(p.userId);
            let rollMsg = `${name} rolled ${p.die1} + ${p.die2} = ${p.total}`;
            if (p.isDoubles) {
                rollMsg += ' DOUBLES!';
                if (p.doublesCount >= 3) {
                    rollMsg += ' (Third doubles - Go to Jail!)';
                }
            }
            addLog(rollMsg, 'event', container);
            if (p.passedGo) {
                addLog(`${name} passed GO - collected $200`, 'event', container);
            }
            addLog(`${name} landed on ${p.spaceName}`, 'event', container);
            // Update player position in local state
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) {
                    player.position = p.newPos;
                    // If doubles (and not third doubles), player can roll again
                    player.hasRolled = !p.isDoubles || p.doublesCount >= 3;
                    if (p.passedGo) player.money += 200;
                }
                updateBoard(gameState, container);
                updateControls(userId, container);
                showDiceResult(p.die1, p.die2, p.isDoubles, container);
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
                if (player) player.pendingAction = 'auction';
            }
            hideBuyPrompt(container);
            updateControls(userId, container);
            break;
        }

        case 'auction_started': {
            const p = message.payload;
            addLog(`Auction started for ${p.propertyName}! Starting bid: $${p.startingBid}`, 'event', container);
            showAuctionModal(p.position, p.propertyName, p.startingBid, p.bidderOrder, p.currentBidderId, userId, container);
            break;
        }

        case 'auction_bid': {
            const p = message.payload;
            addLog(`${p.bidderName} bid $${p.bidAmount}`, 'event', container);
            updateAuctionModal(p.bidAmount, p.bidderName, p.nextBidderId, userId, container);
            break;
        }

        case 'auction_passed': {
            const p = message.payload;
            addLog(`${p.passerName} passed on the auction (${p.remainingCount} bidders left)`, 'event', container);
            updateAuctionModal(null, null, p.nextBidderId, userId, container);
            break;
        }

        case 'auction_ended': {
            const p = message.payload;
            if (p.noWinner) {
                addLog(`Auction ended - no winner, ${p.propertyName} remains unowned`, 'event', container);
            } else {
                addLog(`Auction won by ${p.winnerName} for $${p.finalBid}!`, 'event', container);
                if (gameState) {
                    const winner = gameState.players.find(pl => pl.userId === p.winnerId);
                    if (winner) winner.money -= p.finalBid;
                    if (!gameState.properties) gameState.properties = {};
                    gameState.properties[p.position] = p.winnerId;
                    updateBoard(gameState, container);
                    updateUI(gameState, userId, container);
                }
            }
            // Clear pending action for the player who passed
            if (gameState) {
                const player = gameState.players.find(pl => pl.pendingAction === 'auction');
                if (player) player.pendingAction = '';
            }
            hideAuctionModal(container);
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
            const reason = p.reason === 'three_doubles' ? ' (rolled three doubles!)' : '';
            addLog(`${getPlayerName(p.userId)} was sent to Jail!${reason}`, 'event', container);
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) {
                    player.position = 10;
                    player.inJail = true;
                    player.jailTurns = 0;
                }
                updateBoard(gameState, container);
                updateControls(userId, container);
            }
            break;
        }

        case 'jail_escape': {
            const p = message.payload;
            const methodText = p.method === 'doubles' ? 'by rolling doubles' : 'by paying $50 bail';
            addLog(`${getPlayerName(p.userId)} escaped from Jail ${methodText}!`, 'event', container);
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) {
                    player.inJail = false;
                    player.jailTurns = 0;
                    if (p.newMoney !== undefined) player.money = p.newMoney;
                }
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'jail_roll_failed': {
            const p = message.payload;
            const name = getPlayerName(p.userId);
            addLog(`${name} rolled ${p.die1} + ${p.die2} - no doubles, still in jail (attempt ${p.jailTurns}/3)`, 'event', container);
            if (p.forcedBail) {
                addLog(`${name} was forced to pay $50 bail after 3 failed attempts`, 'event', container);
            }
            if (gameState) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) {
                    player.jailTurns = p.jailTurns;
                    player.hasRolled = true;
                    if (p.newMoney !== undefined) player.money = p.newMoney;
                }
                updateControls(userId, container);
            }
            break;
        }

        case 'property_mortgaged': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} mortgaged ${p.name} for $${p.amount}`, 'event', container);
            if (gameState) {
                if (!gameState.mortgagedProperties) gameState.mortgagedProperties = {};
                gameState.mortgagedProperties[p.position] = true;
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) player.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'property_unmortgaged': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} unmortgaged ${p.name} for $${p.amount}`, 'event', container);
            if (gameState) {
                if (gameState.mortgagedProperties) {
                    delete gameState.mortgagedProperties[p.position];
                }
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) player.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'house_built': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} built a house on ${p.name} (${p.houseCount}/4)`, 'event', container);
            if (gameState) {
                if (!gameState.improvements) gameState.improvements = {};
                gameState.improvements[p.position] = p.houseCount;
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) player.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'hotel_built': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} built a HOTEL on ${p.name}!`, 'event', container);
            if (gameState) {
                if (!gameState.improvements) gameState.improvements = {};
                gameState.improvements[p.position] = p.houseCount; // 5 = hotel
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) player.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'house_sold': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} sold a house from ${p.name} for $${p.refund}`, 'event', container);
            if (gameState) {
                if (!gameState.improvements) gameState.improvements = {};
                if (p.houseCount > 0) {
                    gameState.improvements[p.position] = p.houseCount;
                } else {
                    delete gameState.improvements[p.position];
                }
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) player.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'card_drawn': {
            const p = message.payload;
            const deckName = p.deckType === 'chance' ? 'Chance' : 'Community Chest';
            addLog(`${getPlayerName(p.userId)} drew a ${deckName} card: "${p.cardText}"`, 'event', container);
            if (p.effect) {
                addLog(`  -> ${p.effect}`, 'event', container);
            }
            // Show card modal
            showCardModal(p.deckType, p.cardText, p.effect, container);
            // Track jail card
            if (p.cardType === 'get_out_of_jail' && p.userId === userId) {
                hasJailCard = true;
            }
            if (gameState && p.newMoney !== undefined) {
                const player = gameState.players.find(pl => pl.userId === p.userId);
                if (player) {
                    player.money = p.newMoney;
                    if (p.newPos !== undefined && p.newPos !== player.position) {
                        player.position = p.newPos;
                    }
                }
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            updateControls(userId, container);
            break;
        }

        case 'card_used': {
            const p = message.payload;
            addLog(`${getPlayerName(p.userId)} used a Get Out of Jail Free card`, 'event', container);
            if (p.userId === userId) {
                hasJailCard = false;
            }
            updateControls(userId, container);
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
            stopTurnTimerDisplay(container);
            showGameOver(message.payload, container);
            break;

        case 'chat': {
            const p = message.payload;
            addLog(`${p.username}: ${p.message}`, 'chat', container);
            break;
        }

        case 'timer_started': {
            const p = message.payload;
            startTurnTimerDisplay(p.playerId, p.duration, container);
            break;
        }

        case 'trade_proposed': {
            const p = message.payload;
            addLog(`${p.fromUsername} proposed a trade to ${p.toUsername}`, 'event', container);
            // If this trade is for the current user, show the trade notification
            if (p.trade.toUserId === userId) {
                pendingTrades.push(p);
                showTradeNotification(p, container);
            }
            break;
        }

        case 'trade_accepted': {
            const p = message.payload;
            addLog(`Trade accepted: ${p.fromUsername} and ${p.toUsername} completed a trade`, 'event', container);
            // Remove from pending trades
            pendingTrades = pendingTrades.filter(t => t.trade.id !== p.tradeId);
            hideTradeNotification(container);
            // Reload game state to reflect property/money changes
            loadGameState(gameId, userId, container);
            break;
        }

        case 'trade_declined': {
            const p = message.payload;
            addLog(`Trade declined by ${p.toUsername}`, 'event', container);
            pendingTrades = pendingTrades.filter(t => t.trade.id !== p.tradeId);
            hideTradeNotification(container);
            break;
        }

        case 'trade_cancelled': {
            const p = message.payload;
            addLog(`Trade cancelled by ${p.fromUsername}`, 'event', container);
            pendingTrades = pendingTrades.filter(t => t.trade.id !== p.tradeId);
            hideTradeNotification(container);
            break;
        }

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

    // Clear existing tokens, ownership bars, mortgage indicators, and house indicators
    container.querySelectorAll('.player-tokens, .ownership-bar, .mortgage-indicator, .house-indicator').forEach(el => el.remove());

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

    // Render mortgage indicators
    if (state.mortgagedProperties) {
        for (const pos of Object.keys(state.mortgagedProperties)) {
            const el = container.querySelector(`[data-space="${pos}"]`);
            if (!el) continue;

            const indicator = document.createElement('div');
            indicator.className = 'mortgage-indicator';
            indicator.textContent = 'M';
            indicator.title = 'Mortgaged';
            el.appendChild(indicator);
        }
    }

    // Render house/hotel indicators
    if (state.improvements) {
        for (const [pos, count] of Object.entries(state.improvements)) {
            const el = container.querySelector(`[data-space="${pos}"]`);
            if (!el || count <= 0) continue;

            const indicator = document.createElement('div');
            indicator.className = 'house-indicator';
            if (count === 5) {
                indicator.textContent = 'H';
                indicator.title = 'Hotel';
                indicator.classList.add('hotel');
            } else {
                indicator.textContent = count.toString();
                indicator.title = `${count} house${count > 1 ? 's' : ''}`;
            }
            el.appendChild(indicator);
        }
    }

    // Add click handlers for property info (only for properties, railroads, utilities)
    state.board.forEach(space => {
        if (space.type !== 'property' && space.type !== 'railroad' && space.type !== 'utility') return;

        const el = container.querySelector(`[data-space="${space.position}"]`);
        if (!el) return;

        // Remove existing click handler to avoid duplicates
        el.style.cursor = 'pointer';
        el.onclick = () => showPropertyInfoPanel(space, state, currentUserId, container);
    });
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
    const payBailBtn = container.querySelector('#payBailBtn');

    if (gameState.status !== 'in_progress') {
        rollBtn.disabled = true;
        endTurnBtn.disabled = true;
        if (payBailBtn) payBailBtn.style.display = 'none';
        return;
    }

    const me = gameState.players.find(p => p.userId === userId);
    if (!me) return;

    const isMyTurn = me.isCurrentTurn && !me.isBankrupt;

    // Roll dice: enabled if my turn, haven't rolled, no pending action
    rollBtn.disabled = !(isMyTurn && !me.hasRolled && !me.pendingAction);

    // End turn: enabled if my turn, have rolled, no pending action
    // If in jail and just rolled (failed), can end turn
    endTurnBtn.disabled = !(isMyTurn && me.hasRolled && !me.pendingAction);

    // Pay bail: show if in jail, my turn, haven't rolled, and have $50
    if (payBailBtn) {
        const canPayBail = isMyTurn && me.inJail && !me.hasRolled && me.money >= 50;
        payBailBtn.style.display = canPayBail ? 'inline-block' : 'none';
    }

    // Use jail card: show if in jail, my turn, haven't rolled, and have a jail card
    const useJailCardBtn = container.querySelector('#useJailCardBtn');
    if (useJailCardBtn) {
        const canUseCard = isMyTurn && me.inJail && !me.hasRolled && hasJailCard;
        useJailCardBtn.style.display = canUseCard ? 'inline-block' : 'none';
    }
}

function showDiceResult(die1, die2, isDoubles, container) {
    const el = container.querySelector('#diceResult');
    if (el) {
        let text = `Dice: [${die1}] [${die2}] = ${die1 + die2}`;
        if (isDoubles) text += ' DOUBLES!';
        el.textContent = text;
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

function payJailBail() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'pay_jail_bail', payload: {} }));
}

function useJailCard() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (confirm('Are you sure you want to use your Get Out of Jail Free card?')) {
        ws.send(JSON.stringify({ type: 'use_jail_card', payload: {} }));
    }
}

function mortgageProperty(position) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'mortgage_property', payload: { position } }));
}

function unmortgageProperty(position) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'unmortgage_property', payload: { position } }));
}

function buyHouse(position) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'buy_house', payload: { position } }));
}

function sellHouse(position) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'sell_house', payload: { position } }));
}

function sendChatMessage(input, container) {
    const message = input.value.trim();
    if (!message) return;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'chat', payload: { message } }));
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

// Trading functions
function openTradeModal(container) {
    if (!gameState || gameState.status !== 'in_progress') return;

    const me = gameState.players.find(p => p.userId === currentUserId);
    if (!me || me.isBankrupt) return;

    // Get other players who are not bankrupt
    const otherPlayers = gameState.players.filter(p => p.userId !== currentUserId && !p.isBankrupt);
    if (otherPlayers.length === 0) {
        addLog('No players available to trade with', 'system', container);
        return;
    }

    // Get my properties
    const myProperties = [];
    if (gameState.properties && gameState.board) {
        for (const [pos, ownerId] of Object.entries(gameState.properties)) {
            if (ownerId === currentUserId) {
                const space = gameState.board[parseInt(pos)];
                // Only tradeable if not mortgaged and no houses
                const isMortgaged = gameState.mortgagedProperties && gameState.mortgagedProperties[pos];
                const hasHouses = gameState.improvements && gameState.improvements[pos] > 0;
                if (!isMortgaged && !hasHouses && space) {
                    myProperties.push({ position: parseInt(pos), name: space.name });
                }
            }
        }
    }

    const overlay = document.createElement('div');
    overlay.className = 'trade-overlay';
    overlay.id = 'tradeModal';
    overlay.innerHTML = `
        <div class="trade-modal">
            <h2>Propose Trade</h2>
            <div class="trade-form">
                <div class="trade-section">
                    <label>Trade with:</label>
                    <select id="tradeTargetPlayer">
                        ${otherPlayers.map(p => `<option value="${p.userId}">${p.username} ($${p.money})</option>`).join('')}
                    </select>
                </div>
                <div class="trade-columns">
                    <div class="trade-column">
                        <h4>You Offer</h4>
                        <div class="trade-section">
                            <label>Money: $<input type="number" id="offeredMoney" min="0" max="${me.money}" value="0" /></label>
                        </div>
                        <div class="trade-section">
                            <label>Properties:</label>
                            <div class="property-checkboxes" id="offeredProperties">
                                ${myProperties.length > 0
                                    ? myProperties.map(p => `<label><input type="checkbox" value="${p.position}" /> ${p.name}</label>`).join('')
                                    : '<span class="no-properties">No tradeable properties</span>'}
                            </div>
                        </div>
                    </div>
                    <div class="trade-column">
                        <h4>You Request</h4>
                        <div class="trade-section">
                            <label>Money: $<input type="number" id="requestedMoney" min="0" value="0" /></label>
                        </div>
                        <div class="trade-section">
                            <label>Properties:</label>
                            <div class="property-checkboxes" id="requestedProperties">
                                <span class="no-properties">Select a player</span>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="trade-buttons">
                    <button id="submitTradeBtn" class="primary-btn">Propose Trade</button>
                    <button id="cancelTradeModalBtn" class="secondary-btn">Cancel</button>
                </div>
            </div>
        </div>
    `;

    container.appendChild(overlay);

    // Update requested properties when target player changes
    const targetSelect = overlay.querySelector('#tradeTargetPlayer');
    targetSelect.addEventListener('change', () => updateRequestedProperties(overlay));
    updateRequestedProperties(overlay);

    // Update max requested money when target changes
    targetSelect.addEventListener('change', () => {
        const targetId = parseInt(targetSelect.value);
        const target = gameState.players.find(p => p.userId === targetId);
        if (target) {
            overlay.querySelector('#requestedMoney').max = target.money;
        }
    });

    overlay.querySelector('#submitTradeBtn').addEventListener('click', () => submitTrade(container));
    overlay.querySelector('#cancelTradeModalBtn').addEventListener('click', () => closeTradeModal(container));
}

function updateRequestedProperties(overlay) {
    const targetSelect = overlay.querySelector('#tradeTargetPlayer');
    const targetId = parseInt(targetSelect.value);
    const requestedDiv = overlay.querySelector('#requestedProperties');

    const targetProperties = [];
    if (gameState.properties && gameState.board) {
        for (const [pos, ownerId] of Object.entries(gameState.properties)) {
            if (ownerId === targetId) {
                const space = gameState.board[parseInt(pos)];
                const isMortgaged = gameState.mortgagedProperties && gameState.mortgagedProperties[pos];
                const hasHouses = gameState.improvements && gameState.improvements[pos] > 0;
                if (!isMortgaged && !hasHouses && space) {
                    targetProperties.push({ position: parseInt(pos), name: space.name });
                }
            }
        }
    }

    if (targetProperties.length > 0) {
        requestedDiv.innerHTML = targetProperties.map(p =>
            `<label><input type="checkbox" value="${p.position}" /> ${p.name}</label>`
        ).join('');
    } else {
        requestedDiv.innerHTML = '<span class="no-properties">No tradeable properties</span>';
    }
}

function submitTrade(container) {
    const modal = container.querySelector('#tradeModal');
    if (!modal) return;

    const toUserId = parseInt(modal.querySelector('#tradeTargetPlayer').value);
    const offeredMoney = parseInt(modal.querySelector('#offeredMoney').value) || 0;
    const requestedMoney = parseInt(modal.querySelector('#requestedMoney').value) || 0;

    const offeredProperties = [];
    modal.querySelectorAll('#offeredProperties input:checked').forEach(cb => {
        offeredProperties.push(parseInt(cb.value));
    });

    const requestedProperties = [];
    modal.querySelectorAll('#requestedProperties input:checked').forEach(cb => {
        requestedProperties.push(parseInt(cb.value));
    });

    // Validate that something is being traded
    if (offeredMoney === 0 && requestedMoney === 0 && offeredProperties.length === 0 && requestedProperties.length === 0) {
        addLog('Trade must include something to offer or request', 'system', container);
        return;
    }

    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    ws.send(JSON.stringify({
        type: 'propose_trade',
        payload: {
            toUserId,
            offer: {
                offeredMoney,
                requestedMoney,
                offeredProperties,
                requestedProperties
            }
        }
    }));

    closeTradeModal(container);
}

function closeTradeModal(container) {
    const modal = container.querySelector('#tradeModal');
    if (modal) modal.remove();
}

function showTradeNotification(tradePayload, container) {
    // Remove existing notification if any
    hideTradeNotification(container);

    const trade = tradePayload.trade;
    const offer = trade.offer;

    // Build description of what's being offered/requested
    const getPropertyNames = (positions) => {
        if (!positions || positions.length === 0) return [];
        return positions.map(pos => {
            const space = gameState.board[pos];
            return space ? space.name : `Position ${pos}`;
        });
    };

    const offeredProps = getPropertyNames(offer.offeredProperties);
    const requestedProps = getPropertyNames(offer.requestedProperties);

    let offerText = [];
    if (offer.offeredMoney > 0) offerText.push(`$${offer.offeredMoney}`);
    if (offeredProps.length > 0) offerText.push(offeredProps.join(', '));

    let requestText = [];
    if (offer.requestedMoney > 0) requestText.push(`$${offer.requestedMoney}`);
    if (requestedProps.length > 0) requestText.push(requestedProps.join(', '));

    const overlay = document.createElement('div');
    overlay.className = 'trade-notification-overlay';
    overlay.id = 'tradeNotification';
    overlay.innerHTML = `
        <div class="trade-notification">
            <h3>Trade Offer from ${tradePayload.fromUsername}</h3>
            <div class="trade-details">
                <div class="trade-side">
                    <strong>They offer:</strong>
                    <div>${offerText.length > 0 ? offerText.join(', ') : 'Nothing'}</div>
                </div>
                <div class="trade-arrow">⇄</div>
                <div class="trade-side">
                    <strong>They want:</strong>
                    <div>${requestText.length > 0 ? requestText.join(', ') : 'Nothing'}</div>
                </div>
            </div>
            <div class="trade-buttons">
                <button id="acceptTradeBtn" class="primary-btn">Accept</button>
                <button id="declineTradeBtn" class="secondary-btn">Decline</button>
            </div>
        </div>
    `;

    container.appendChild(overlay);

    overlay.querySelector('#acceptTradeBtn').addEventListener('click', () => {
        acceptTrade(trade.id);
    });
    overlay.querySelector('#declineTradeBtn').addEventListener('click', () => {
        declineTrade(trade.id);
    });
}

function hideTradeNotification(container) {
    const notification = container.querySelector('#tradeNotification');
    if (notification) notification.remove();
}

function acceptTrade(tradeId) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (confirm('Are you sure you want to accept this trade?')) {
        ws.send(JSON.stringify({ type: 'accept_trade', payload: { tradeId } }));
    }
}

function declineTrade(tradeId) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'decline_trade', payload: { tradeId } }));
}

function cancelTrade(tradeId) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'cancel_trade', payload: { tradeId } }));
}

// Turn Timer Display
function startTurnTimerDisplay(playerId, duration, container) {
    // Clear any existing timer
    if (turnTimerInterval) {
        clearInterval(turnTimerInterval);
    }

    turnTimerDuration = duration;
    turnTimerEnd = Date.now() + (duration * 1000);

    // Update immediately
    updateTurnTimerDisplay(playerId, container);

    // Update every second
    turnTimerInterval = setInterval(() => {
        updateTurnTimerDisplay(playerId, container);
    }, 1000);
}

function updateTurnTimerDisplay(playerId, container) {
    const timerEl = container.querySelector('#turnTimer');
    if (!timerEl) return;

    if (!turnTimerEnd) {
        timerEl.style.display = 'none';
        return;
    }

    const remaining = Math.max(0, Math.ceil((turnTimerEnd - Date.now()) / 1000));
    const progress = remaining / turnTimerDuration;

    if (remaining <= 0) {
        // Timer expired
        if (turnTimerInterval) {
            clearInterval(turnTimerInterval);
            turnTimerInterval = null;
        }
        timerEl.style.display = 'none';
        return;
    }

    // Build ASCII progress bar (20 chars wide)
    const barWidth = 20;
    const filled = Math.round(progress * barWidth);
    const empty = barWidth - filled;
    const bar = '[' + '='.repeat(filled) + ' '.repeat(empty) + ']';

    // Determine color class based on time remaining
    let colorClass = 'timer-normal';
    if (remaining <= 10) {
        colorClass = 'timer-critical';
    } else if (remaining <= 20) {
        colorClass = 'timer-warning';
    }

    // Check if this is the current user's turn
    const isMyTurn = playerId === currentUserId;
    const turnIndicator = isMyTurn ? 'YOUR TURN' : getPlayerName(playerId);

    timerEl.innerHTML = `<span class="${colorClass}">${turnIndicator}: ${bar} ${remaining}s</span>`;
    timerEl.style.display = 'block';
}

function stopTurnTimerDisplay(container) {
    if (turnTimerInterval) {
        clearInterval(turnTimerInterval);
        turnTimerInterval = null;
    }
    turnTimerEnd = null;
    const timerEl = container.querySelector('#turnTimer');
    if (timerEl) {
        timerEl.style.display = 'none';
    }
}

// Auction functions
function showAuctionModal(position, propertyName, startingBid, bidderOrder, currentBidderId, userId, container) {
    activeAuction = {
        position,
        propertyName,
        highestBid: 0,
        highestBidderName: null,
        currentBidderId,
        bidderOrder
    };

    let modal = container.querySelector('#auctionModal');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'auctionModal';
        modal.className = 'modal';
        container.appendChild(modal);
    }

    const isMyTurn = currentBidderId === userId;

    modal.innerHTML = `
        <div class="modal-content auction-modal">
            <h3>Property Auction</h3>
            <p class="auction-property">${propertyName}</p>
            <div class="auction-info">
                <p>Minimum bid: $${startingBid}</p>
                <p id="auctionCurrentBid">Current bid: None</p>
                <p id="auctionBidder">Waiting for bids...</p>
            </div>
            <div id="auctionActions" style="display: ${isMyTurn ? 'block' : 'none'}">
                <p id="auctionTurnIndicator" class="your-turn">Your turn to bid!</p>
                <div class="auction-bid-controls">
                    <input type="number" id="auctionBidInput" min="${startingBid}" value="${startingBid}" step="1">
                    <button id="placeBidBtn" class="btn primary">Place Bid</button>
                    <button id="passAuctionBtn" class="btn secondary">Pass</button>
                </div>
            </div>
            <div id="auctionWaiting" style="display: ${isMyTurn ? 'none' : 'block'}">
                <p>Waiting for ${getPlayerName(currentBidderId)} to bid...</p>
            </div>
        </div>
    `;

    modal.style.display = 'flex';

    // Add event listeners
    const placeBidBtn = modal.querySelector('#placeBidBtn');
    const passAuctionBtn = modal.querySelector('#passAuctionBtn');
    const bidInput = modal.querySelector('#auctionBidInput');

    placeBidBtn.addEventListener('click', () => {
        const amount = parseInt(bidInput.value);
        if (amount > activeAuction.highestBid && ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'place_bid', payload: { amount } }));
        }
    });

    passAuctionBtn.addEventListener('click', () => {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'pass_auction', payload: {} }));
        }
    });
}

function updateAuctionModal(bidAmount, bidderName, nextBidderId, userId, container) {
    if (!activeAuction) return;

    const modal = container.querySelector('#auctionModal');
    if (!modal) return;

    if (bidAmount !== null) {
        activeAuction.highestBid = bidAmount;
        activeAuction.highestBidderName = bidderName;

        const currentBidEl = modal.querySelector('#auctionCurrentBid');
        const bidderEl = modal.querySelector('#auctionBidder');

        if (currentBidEl) currentBidEl.textContent = `Current bid: $${bidAmount}`;
        if (bidderEl) bidderEl.textContent = `Highest bidder: ${bidderName}`;

        // Update min bid input
        const bidInput = modal.querySelector('#auctionBidInput');
        if (bidInput) {
            bidInput.min = bidAmount + 1;
            bidInput.value = bidAmount + 1;
        }
    }

    activeAuction.currentBidderId = nextBidderId;
    const isMyTurn = nextBidderId === userId;

    const actionsEl = modal.querySelector('#auctionActions');
    const waitingEl = modal.querySelector('#auctionWaiting');

    if (actionsEl) actionsEl.style.display = isMyTurn ? 'block' : 'none';
    if (waitingEl) {
        waitingEl.style.display = isMyTurn ? 'none' : 'block';
        waitingEl.innerHTML = `<p>Waiting for ${getPlayerName(nextBidderId)} to bid...</p>`;
    }
}

function hideAuctionModal(container) {
    activeAuction = null;
    const modal = container.querySelector('#auctionModal');
    if (modal) {
        modal.style.display = 'none';
    }
}

// Property Info Panel
function showPropertyInfoPanel(space, state, userId, container) {
    let panel = container.querySelector('#propertyInfoPanel');
    if (!panel) {
        panel = document.createElement('div');
        panel.id = 'propertyInfoPanel';
        panel.className = 'property-info-panel';
        container.appendChild(panel);
    }

    const ownerId = state.properties ? state.properties[space.position] : null;
    const owner = ownerId ? state.players.find(p => p.userId === ownerId) : null;
    const isMortgaged = state.mortgagedProperties && state.mortgagedProperties[space.position];
    const improvements = state.improvements ? (state.improvements[space.position] || 0) : 0;
    const isOwnedByUser = ownerId === userId;

    // Calculate rent levels for display
    let rentInfo = '';
    if (space.type === 'property') {
        const rentLevels = space.rentWithHouses || [space.rent, 0, 0, 0, 0, 0];
        rentInfo = `
            <div class="rent-table">
                <div class="rent-row"><span>Rent:</span><span>$${rentLevels[0]}</span></div>
                <div class="rent-row"><span>With monopoly:</span><span>$${rentLevels[0] * 2}</span></div>
                <div class="rent-row"><span>1 house:</span><span>$${rentLevels[1]}</span></div>
                <div class="rent-row"><span>2 houses:</span><span>$${rentLevels[2]}</span></div>
                <div class="rent-row"><span>3 houses:</span><span>$${rentLevels[3]}</span></div>
                <div class="rent-row"><span>4 houses:</span><span>$${rentLevels[4]}</span></div>
                <div class="rent-row"><span>Hotel:</span><span>$${rentLevels[5]}</span></div>
            </div>
            <p>House cost: $${space.houseCost}</p>
        `;
    } else if (space.type === 'railroad') {
        rentInfo = `
            <div class="rent-table">
                <div class="rent-row"><span>1 railroad:</span><span>$25</span></div>
                <div class="rent-row"><span>2 railroads:</span><span>$50</span></div>
                <div class="rent-row"><span>3 railroads:</span><span>$100</span></div>
                <div class="rent-row"><span>4 railroads:</span><span>$200</span></div>
            </div>
        `;
    } else if (space.type === 'utility') {
        rentInfo = `
            <div class="rent-table">
                <div class="rent-row"><span>1 utility:</span><span>4x dice</span></div>
                <div class="rent-row"><span>2 utilities:</span><span>10x dice</span></div>
            </div>
        `;
    }

    const mortgageValue = Math.floor(space.price / 2);
    const unmortgageCost = Math.floor(mortgageValue * 1.1);

    let actionButtons = '';
    if (isOwnedByUser && state.status === 'in_progress') {
        if (isMortgaged) {
            actionButtons = `<button class="btn primary" onclick="window.unmortgageProperty(${space.position})">Unmortgage ($${unmortgageCost})</button>`;
        } else {
            actionButtons = `<button class="btn secondary" onclick="window.mortgageProperty(${space.position})">Mortgage (Get $${mortgageValue})</button>`;
            if (space.type === 'property') {
                if (improvements < 5) {
                    actionButtons += `<button class="btn primary" onclick="window.buyHouse(${space.position})">Buy House ($${space.houseCost})</button>`;
                }
                if (improvements > 0) {
                    actionButtons += `<button class="btn secondary" onclick="window.sellHouse(${space.position})">Sell House (Get $${Math.floor(space.houseCost / 2)})</button>`;
                }
            }
        }
    }

    panel.innerHTML = `
        <div class="panel-header">
            <h3 style="color: ${getColorForGroup(space.color)};">${space.name}</h3>
            <button class="close-btn" onclick="window.hidePropertyPanel()">X</button>
        </div>
        <div class="panel-content">
            <p><strong>Price:</strong> $${space.price}</p>
            <p><strong>Owner:</strong> ${owner ? owner.username : 'Unowned'}</p>
            ${isMortgaged ? '<p class="mortgaged-label">MORTGAGED</p>' : ''}
            ${improvements > 0 ? `<p><strong>Improvements:</strong> ${improvements === 5 ? 'Hotel' : improvements + ' house(s)'}</p>` : ''}
            <p><strong>Mortgage value:</strong> $${mortgageValue}</p>
            ${rentInfo}
            <div class="panel-actions">
                ${actionButtons}
            </div>
        </div>
    `;

    panel.style.display = 'block';

    // Set up global functions for the buttons
    window.hidePropertyPanel = () => hidePropertyInfoPanel(container);
    window.mortgageProperty = (pos) => {
        if (confirm('Are you sure you want to mortgage this property? You will need to pay 110% to unmortgage it.')) {
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ type: 'mortgage_property', payload: { position: pos } }));
            }
        }
    };
    window.unmortgageProperty = (pos) => {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'unmortgage_property', payload: { position: pos } }));
        }
    };
    window.buyHouse = (pos) => {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'buy_house', payload: { position: pos } }));
        }
    };
    window.sellHouse = (pos) => {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'sell_house', payload: { position: pos } }));
        }
    };
}

function hidePropertyInfoPanel(container) {
    const panel = container.querySelector('#propertyInfoPanel');
    if (panel) {
        panel.style.display = 'none';
    }
}

function getColorForGroup(color) {
    const colorMap = {
        'brown': '#8B4513',
        'light-blue': '#87CEEB',
        'pink': '#FF69B4',
        'orange': '#FFA500',
        'red': '#FF0000',
        'yellow': '#FFFF00',
        'green': '#00FF00',
        'dark-blue': '#0000CD'
    };
    return colorMap[color] || 'var(--accent-color)';
}

// Reconnection Indicator
function showReconnectIndicator(attempt, delay, container) {
    let indicator = container.querySelector('#reconnectIndicator');
    if (!indicator) {
        indicator = document.createElement('div');
        indicator.id = 'reconnectIndicator';
        indicator.className = 'reconnect-indicator';
        container.appendChild(indicator);
    }

    if (attempt === -1) {
        indicator.innerHTML = `
            <span class="reconnect-icon">&#x26A0;</span>
            <span>Connection lost. <button onclick="location.reload()">Refresh</button></span>
        `;
        indicator.className = 'reconnect-indicator error';
    } else {
        indicator.innerHTML = `
            <span class="reconnect-icon spinning">&#x21BB;</span>
            <span>Reconnecting... (${attempt}/${maxReconnectAttempts})</span>
        `;
        indicator.className = 'reconnect-indicator';
    }
    indicator.style.display = 'flex';
}

function hideReconnectIndicator(container) {
    const indicator = container.querySelector('#reconnectIndicator');
    if (indicator) {
        indicator.style.display = 'none';
    }
}

// Browser Notifications
function requestNotificationPermission() {
    if ('Notification' in window && Notification.permission === 'default') {
        Notification.requestPermission();
    }
}

function sendTurnNotification() {
    if ('Notification' in window && Notification.permission === 'granted' && document.hidden) {
        const notification = new Notification('Monopoly - Your Turn!', {
            body: "It's your turn to play",
            icon: '/static/images/monopoly-icon.png',
            tag: 'monopoly-turn'
        });
        // Close notification after 5 seconds
        setTimeout(() => notification.close(), 5000);
        // Focus window when notification is clicked
        notification.onclick = () => {
            window.focus();
            notification.close();
        };
    }
}

// Card Modal
function showCardModal(deckType, cardText, effect, container) {
    let modal = container.querySelector('#cardModal');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'cardModal';
        modal.className = 'card-modal-overlay';
        container.appendChild(modal);
    }

    const isChance = deckType === 'chance';
    const headerColor = isChance ? '#FF8C00' : '#4169E1';
    const headerText = isChance ? 'CHANCE' : 'COMMUNITY CHEST';

    modal.innerHTML = `
        <div class="card-modal ${isChance ? 'chance-card' : 'community-card'}">
            <div class="card-header" style="background-color: ${headerColor};">
                <span>${headerText}</span>
            </div>
            <div class="card-body">
                <p class="card-text">${cardText}</p>
                ${effect ? `<p class="card-effect">${effect}</p>` : ''}
            </div>
            <button class="card-dismiss-btn" onclick="this.closest('.card-modal-overlay').style.display='none'">OK</button>
        </div>
    `;

    modal.style.display = 'flex';

    // Auto-dismiss after 5 seconds
    setTimeout(() => {
        if (modal.style.display === 'flex') {
            modal.style.display = 'none';
        }
    }, 5000);

    // Click anywhere to dismiss
    modal.onclick = (e) => {
        if (e.target === modal) {
            modal.style.display = 'none';
        }
    };
}
