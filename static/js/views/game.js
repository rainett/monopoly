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

    container.querySelector('#rollDiceBtn').addEventListener('click', rollDice);
    container.querySelector('#buyBtn').addEventListener('click', buyProperty);
    container.querySelector('#passBtn').addEventListener('click', passProperty);
    container.querySelector('#payBailBtn').addEventListener('click', payJailBail);
    container.querySelector('#useJailCardBtn').addEventListener('click', useJailCard);
    container.querySelector('#auctionBidBtn').addEventListener('click', placeBid);
    container.querySelector('#auctionPassBtn').addEventListener('click', passAuction);

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
            addLog('joined the game', 'event', container, message.payload.player.userId, message.payload.player.username);
            loadGameState(gameId, userId, container);
            break;

        case 'game_started':
            addLog('Game started!', 'event', container);
            loadGameState(gameId, userId, container);
            break;

        case 'turn_changed': {
            const tcPayload = message.payload;
            updateTurnFromPayload(tcPayload, userId, container);
            const tcPlayer = gameState?.players.find(p => p.userId === tcPayload.currentPlayerId);
            addLog("'s turn", 'event', container, tcPayload.currentPlayerId, tcPlayer?.username || getPlayerName(tcPayload.currentPlayerId));
            // Send browser notification if it's the user's turn and tab is not focused
            if (tcPayload.currentPlayerId === userId) {
                sendTurnNotification();
            }
            break;
        }

        case 'turn_timeout':
            updateTurnFromPayload(message.payload, userId, container);
            addLog('Turn timeout - automatically skipped', 'system', container);
            break;

        case 'dice_rolled': {
            const p = message.payload;
            const drPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            const drName = drPlayer?.username || getPlayerName(p.userId);
            let rollMsg = `rolled ${p.die1} + ${p.die2} = ${p.total}`;
            if (p.isDoubles) {
                rollMsg += ' DOUBLES!';
                if (p.doublesCount >= 3) {
                    rollMsg += ' (Third doubles - Go to Jail!)';
                }
            }
            addLog(rollMsg, 'event', container, p.userId, drName);
            if (p.passedGo) {
                addLog('passed GO - collected $200', 'event', container, p.userId, drName);
            }
            addLog(`landed on ${p.spaceName}`, 'event', container, p.userId, drName);
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
            const bpPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            if (gameState && bpPlayer) {
                bpPlayer.pendingAction = 'buy_or_pass';
            }
            if (p.userId === userId) {
                showBuyPrompt(p.name, p.price, container);
            } else {
                addLog(`can buy ${p.name} for $${p.price}`, 'event', container, p.userId, bpPlayer?.username || getPlayerName(p.userId));
            }
            updateControls(userId, container);
            break;
        }

        case 'property_bought': {
            const p = message.payload;
            const pbPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            addLog(`bought ${p.name} for $${p.price}`, 'event', container, p.userId, pbPlayer?.username || getPlayerName(p.userId));
            if (gameState) {
                if (pbPlayer) {
                    pbPlayer.money = p.newMoney;
                    pbPlayer.pendingAction = '';
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
            const ppPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            addLog(`passed on ${p.name}`, 'event', container, p.userId, ppPlayer?.username || getPlayerName(p.userId));
            if (gameState && ppPlayer) {
                ppPlayer.pendingAction = 'auction';
            }
            hideBuyPrompt(container);
            updateControls(userId, container);
            break;
        }

        case 'auction_started': {
            const p = message.payload;
            addLog(`Auction started for ${p.propertyName}!`, 'event', container);
            showAuctionControls(p.position, p.propertyName, p.startingBid, p.bidderOrder, p.currentBidderId, userId, container);
            break;
        }

        case 'auction_bid': {
            const p = message.payload;
            addLog(`${p.bidderName} bid $${p.bidAmount}`, 'event', container);
            updateAuctionBid(p.bidAmount, p.bidderName, p.nextBidderId, userId, container);
            break;
        }

        case 'auction_passed': {
            const p = message.payload;
            addLog(`${p.passerName} passed (${p.remainingCount} left)`, 'event', container);
            updateAuctionBid(null, null, p.nextBidderId, userId, container);
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
            hideAuctionControls(container);
            break;
        }

        case 'rent_paid': {
            const p = message.payload;
            const rpPayer = gameState?.players.find(pl => pl.userId === p.payerId);
            const rpOwner = gameState?.players.find(pl => pl.userId === p.ownerId);
            addLog(`paid $${p.amount} rent to ${rpOwner?.username || getPlayerName(p.ownerId)} for ${p.name}`, 'event', container, p.payerId, rpPayer?.username || getPlayerName(p.payerId));
            if (gameState) {
                if (rpPayer) rpPayer.money = p.payerMoney;
                if (rpOwner) rpOwner.money = p.ownerMoney;
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'tax_paid': {
            const p = message.payload;
            const tpPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            addLog(`paid $${p.amount} in taxes`, 'event', container, p.userId, tpPlayer?.username || getPlayerName(p.userId));
            if (gameState) {
                if (tpPlayer) tpPlayer.money = p.newMoney;
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'go_to_jail': {
            const p = message.payload;
            const gtjPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            const reason = p.reason === 'three_doubles' ? ' (rolled three doubles!)' : '';
            addLog(`was sent to Jail!${reason}`, 'event', container, p.userId, gtjPlayer?.username || getPlayerName(p.userId));
            if (gameState && gtjPlayer) {
                gtjPlayer.position = 10;
                gtjPlayer.inJail = true;
                gtjPlayer.jailTurns = 0;
                updateBoard(gameState, container);
                updateControls(userId, container);
            }
            break;
        }

        case 'jail_escape': {
            const p = message.payload;
            const jePlayer = gameState?.players.find(pl => pl.userId === p.userId);
            const methodText = p.method === 'doubles' ? 'by rolling doubles' : 'by paying $50 bail';
            addLog(`escaped from Jail ${methodText}!`, 'event', container, p.userId, jePlayer?.username || getPlayerName(p.userId));
            if (gameState && jePlayer) {
                jePlayer.inJail = false;
                jePlayer.jailTurns = 0;
                if (p.newMoney !== undefined) jePlayer.money = p.newMoney;
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'jail_roll_failed': {
            const p = message.payload;
            const jrfPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            const jrfName = jrfPlayer?.username || getPlayerName(p.userId);
            addLog(`rolled ${p.die1} + ${p.die2} - no doubles, still in jail (attempt ${p.jailTurns}/3)`, 'event', container, p.userId, jrfName);
            if (p.forcedBail) {
                addLog('was forced to pay $50 bail after 3 failed attempts', 'event', container, p.userId, jrfName);
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
            const pmPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            addLog(`mortgaged ${p.name} for $${p.amount}`, 'event', container, p.userId, pmPlayer?.username || getPlayerName(p.userId));
            if (gameState) {
                if (!gameState.mortgagedProperties) gameState.mortgagedProperties = {};
                gameState.mortgagedProperties[p.position] = true;
                if (pmPlayer) pmPlayer.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'property_unmortgaged': {
            const p = message.payload;
            const pumPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            addLog(`unmortgaged ${p.name} for $${p.amount}`, 'event', container, p.userId, pumPlayer?.username || getPlayerName(p.userId));
            if (gameState) {
                if (gameState.mortgagedProperties) {
                    delete gameState.mortgagedProperties[p.position];
                }
                if (pumPlayer) pumPlayer.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'house_built': {
            const p = message.payload;
            const hbPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            addLog(`built a house on ${p.name} (${p.houseCount}/4)`, 'event', container, p.userId, hbPlayer?.username || getPlayerName(p.userId));
            if (gameState) {
                if (!gameState.improvements) gameState.improvements = {};
                gameState.improvements[p.position] = p.houseCount;
                if (hbPlayer) hbPlayer.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'hotel_built': {
            const p = message.payload;
            const htbPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            addLog(`built a HOTEL on ${p.name}!`, 'event', container, p.userId, htbPlayer?.username || getPlayerName(p.userId));
            if (gameState) {
                if (!gameState.improvements) gameState.improvements = {};
                gameState.improvements[p.position] = p.houseCount; // 5 = hotel
                if (htbPlayer) htbPlayer.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'house_sold': {
            const p = message.payload;
            const hsPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            addLog(`sold a house from ${p.name} for $${p.refund}`, 'event', container, p.userId, hsPlayer?.username || getPlayerName(p.userId));
            if (gameState) {
                if (!gameState.improvements) gameState.improvements = {};
                if (p.houseCount > 0) {
                    gameState.improvements[p.position] = p.houseCount;
                } else {
                    delete gameState.improvements[p.position];
                }
                if (hsPlayer) hsPlayer.money = p.newMoney;
                updateBoard(gameState, container);
                updateUI(gameState, userId, container);
            }
            break;
        }

        case 'card_drawn': {
            const p = message.payload;
            const cdPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            const deckName = p.deckType === 'chance' ? 'Chance' : 'Community Chest';
            addLog(`drew a ${deckName} card: "${p.cardText}"`, 'event', container, p.userId, cdPlayer?.username || getPlayerName(p.userId));
            if (p.effect) {
                addLog(`  -> ${p.effect}`, 'event', container);
            }
            // Show card modal for current user
            if (p.userId === userId) {
                showCardModal(p.deckType, p.cardText, p.effect, container);
            }
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
            const cuPlayer = gameState?.players.find(pl => pl.userId === p.userId);
            addLog('used a Get Out of Jail Free card', 'event', container, p.userId, cuPlayer?.username || getPlayerName(p.userId));
            if (p.userId === userId) {
                hasJailCard = false;
            }
            updateControls(userId, container);
            break;
        }

        case 'player_bankrupt': {
            const p = message.payload;
            addLog(`went bankrupt! (${p.reason})`, 'event', container, p.userId, p.username);
            if (gameState) {
                const pbkPlayer = gameState.players.find(pl => pl.userId === p.userId);
                if (pbkPlayer) {
                    pbkPlayer.isBankrupt = true;
                    pbkPlayer.money = 0;
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
            addLog(p.message, 'chat', container, p.userId, p.username);
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

// Generate SVG icon based on space type and color
function getSpaceIcon(space) {
    const size = 24;
    const fill = '#FFFFFF';

    if (space.type === 'property') {
        switch (space.color) {
            case 'brown':
            case 'light_blue':
                // Circle
                return `<svg viewBox="0 0 ${size} ${size}" class="space-icon">
                    <circle cx="12" cy="12" r="10" fill="${fill}"/>
                </svg>`;
            case 'pink':
            case 'orange':
                // Triangle
                return `<svg viewBox="0 0 ${size} ${size}" class="space-icon">
                    <polygon points="12,2 22,22 2,22" fill="${fill}"/>
                </svg>`;
            case 'red':
            case 'yellow':
                // Square
                return `<svg viewBox="0 0 ${size} ${size}" class="space-icon">
                    <rect x="2" y="2" width="20" height="20" fill="${fill}"/>
                </svg>`;
            case 'green':
            case 'dark_blue':
                // Diamond
                return `<svg viewBox="0 0 ${size} ${size}" class="space-icon">
                    <polygon points="12,2 22,12 12,22 2,12" fill="${fill}"/>
                </svg>`;
        }
    }

    if (space.type === 'railroad') {
        // 4-point star
        return `<svg viewBox="0 0 ${size} ${size}" class="space-icon">
            <polygon points="12,0 14,10 24,12 14,14 12,24 10,14 0,12 10,10" fill="${fill}"/>
        </svg>`;
    }

    if (space.type === 'utility') {
        // Hexagon
        return `<svg viewBox="0 0 ${size} ${size}" class="space-icon">
            <polygon points="12,2 21,7 21,17 12,22 3,17 3,7" fill="${fill}"/>
        </svg>`;
    }

    if (space.type === 'chance') {
        // Circle with ?
        return `<svg viewBox="0 0 ${size} ${size}" class="space-icon">
            <circle cx="12" cy="12" r="10" fill="none" stroke="${fill}" stroke-width="2"/>
            <text x="12" y="17" text-anchor="middle" fill="${fill}" font-size="14" font-weight="bold">?</text>
        </svg>`;
    }

    if (space.type === 'community_chest') {
        // Simple chest box
        return `<svg viewBox="0 0 ${size} ${size}" class="space-icon">
            <rect x="2" y="8" width="20" height="14" fill="${fill}"/>
            <rect x="2" y="6" width="20" height="4" fill="${fill}"/>
        </svg>`;
    }

    if (space.type === 'tax') {
        // Circle with $
        return `<svg viewBox="0 0 ${size} ${size}" class="space-icon">
            <circle cx="12" cy="12" r="10" fill="none" stroke="${fill}" stroke-width="2"/>
            <text x="12" y="17" text-anchor="middle" fill="${fill}" font-size="12" font-weight="bold">$</text>
        </svg>`;
    }

    return '';  // No icon for corners
}

function updateBoard(state, container) {
    if (!state || !state.board) return;

    // Populate space icons and colors from board data
    state.board.forEach(space => {
        const el = container.querySelector(`[data-space="${space.position}"]`);
        if (!el) return;

        const nameEl = el.querySelector('.space-name');
        if (nameEl && space.position !== 0 && space.position !== 10 && space.position !== 20 && space.position !== 30) {
            const icon = getSpaceIcon(space);
            nameEl.innerHTML = icon || space.name;  // Fallback to name if no icon
        }

        // Set color bar
        const colorEl = el.querySelector('.space-color');
        if (colorEl && space.color) {
            colorEl.className = 'space-color ' + space.color;
        }
    });

    // Clear existing tokens, mortgage indicators, and house indicators
    container.querySelectorAll('.player-tokens, .mortgage-indicator, .house-indicator').forEach(el => el.remove());

    // Clear previous ownership classes
    container.querySelectorAll('.space').forEach(el => {
        el.classList.remove('owned-0', 'owned-1', 'owned-2', 'owned-3');
    });

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

    // Render ownership indicators via background color
    if (state.properties) {
        for (const [pos, ownerId] of Object.entries(state.properties)) {
            const el = container.querySelector(`[data-space="${pos}"]`);
            if (!el) continue;

            const ownerIdx = state.players.findIndex(p => p.userId === ownerId);
            if (ownerIdx === -1) continue;

            el.classList.add(`owned-${ownerIdx}`);
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

    const me = state.players.find(p => p.userId === userId);
    const isMyTurn = me && me.isCurrentTurn && !me.isBankrupt;
    const currentPlayer = state.players.find(p => p.isCurrentTurn);

    const playersListDiv = container.querySelector('#playersList');
    playersListDiv.innerHTML = state.players.map((player, idx) => `
        <div class="player-item ${player.isCurrentTurn ? 'current-turn' : ''} ${player.isBankrupt ? 'bankrupt' : ''}"
             data-user-id="${player.userId}" data-username="${player.username}">
            <div class="player-name">
                <span class="player-color-dot" style="background-color:${['#FF4444','#4444FF','#44FF44','#FFFF44'][idx]}"></span>
                ${player.username}${player.userId === userId ? ' (You)' : ''}
            </div>
            <div class="player-info-row">
                <span class="player-money">${player.isBankrupt ? 'BANKRUPT' : '$' + player.money}</span>
                ${player.isCurrentTurn ? '<span class="player-timer" id="playerTimer"></span>' : ''}
            </div>
        </div>
    `).join('');

    // Add click handlers for player items
    playersListDiv.querySelectorAll('.player-item').forEach(item => {
        const itemUserId = parseInt(item.dataset.userId);
        const isBankrupt = item.classList.contains('bankrupt');

        if (!isBankrupt && state.status === 'in_progress') {
            item.addEventListener('click', (e) => {
                e.stopPropagation();
                if (itemUserId === userId) {
                    showPlayerActionPopup(e, 'self', container);
                } else {
                    openTradeModalWithPlayer(itemUserId, item.dataset.username, container);
                }
            });
        }
    });

    updateControls(userId, container);
}

function updateControls(userId, container) {
    if (!gameState) return;

    const gameControls = container.querySelector('#gameControls');
    const rollBtn = container.querySelector('#rollDiceBtn');
    const payBailBtn = container.querySelector('#payBailBtn');
    const buyPrompt = container.querySelector('#buyPrompt');

    if (gameState.status !== 'in_progress') {
        rollBtn.disabled = true;
        if (payBailBtn) payBailBtn.style.display = 'none';
        if (gameControls) gameControls.style.display = 'none';
        return;
    }

    const me = gameState.players.find(p => p.userId === userId);
    if (!me) return;

    const isMyTurn = me.isCurrentTurn && !me.isBankrupt;
    const hasBuyPrompt = buyPrompt && buyPrompt.style.display !== 'none';
    const isAuctionMyTurn = activeAuction && activeAuction.currentBidderId === userId;

    // Show action box only when: my turn, or buy prompt visible, or auction active and my bid
    const showControls = isMyTurn || hasBuyPrompt || me.pendingAction || isAuctionMyTurn;
    if (gameControls) {
        gameControls.style.display = showControls ? 'flex' : 'none';
    }

    // Roll dice: enabled if my turn, haven't rolled, no pending action
    rollBtn.disabled = !(isMyTurn && !me.hasRolled && !me.pendingAction);

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

function giveUp() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'give_up', payload: {} }));
}

function showPlayerActionPopup(event, type, container) {
    // Remove any existing popup
    hidePlayerActionPopup(container);

    const popup = document.createElement('div');
    popup.className = 'player-action-popup';
    popup.id = 'playerActionPopup';

    if (type === 'self') {
        popup.innerHTML = `
            <button class="give-up-btn">Give Up</button>
        `;

        popup.querySelector('.give-up-btn').addEventListener('click', () => {
            hidePlayerActionPopup(container);
            showConfirmModal('Are you sure you want to give up? You will forfeit the game.', giveUp, container);
        });
    }

    // Position popup near the click
    popup.style.left = `${event.clientX}px`;
    popup.style.top = `${event.clientY}px`;

    container.appendChild(popup);

    // Close popup when clicking outside
    const closeHandler = (e) => {
        if (!popup.contains(e.target)) {
            hidePlayerActionPopup(container);
            document.removeEventListener('click', closeHandler);
        }
    };
    setTimeout(() => document.addEventListener('click', closeHandler), 0);
}

function hidePlayerActionPopup(container) {
    const popup = container.querySelector('#playerActionPopup');
    if (popup) popup.remove();
}

// Custom confirmation modal to replace browser confirm()
function showConfirmModal(message, onConfirm, container = null) {
    const parent = container || document.body;
    let modal = document.querySelector('#confirmModal');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'confirmModal';
        modal.className = 'modal-overlay';
        parent.appendChild(modal);
    }

    modal.innerHTML = `
        <div class="modal-content confirm-modal">
            <h2>Confirm</h2>
            <p class="confirm-message">${message}</p>
            <div class="modal-actions">
                <button class="secondary-btn" id="confirmCancelBtn">Cancel</button>
                <button class="primary-btn" id="confirmYesBtn">Confirm</button>
            </div>
        </div>
    `;

    modal.style.display = 'flex';

    const cancelBtn = modal.querySelector('#confirmCancelBtn');
    const yesBtn = modal.querySelector('#confirmYesBtn');

    const closeModal = () => {
        modal.style.display = 'none';
    };

    cancelBtn.addEventListener('click', closeModal);
    yesBtn.addEventListener('click', () => {
        closeModal();
        onConfirm();
    });

    // Close on escape key
    const escHandler = (e) => {
        if (e.key === 'Escape') {
            closeModal();
            document.removeEventListener('keydown', escHandler);
        }
    };
    document.addEventListener('keydown', escHandler);
}

function hideConfirmModal(container) {
    const modal = container.querySelector('#confirmModal');
    if (modal) modal.style.display = 'none';
}

function payJailBail() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'pay_jail_bail', payload: {} }));
}

function useJailCard() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    showConfirmModal('Are you sure you want to use your Get Out of Jail Free card?', () => {
        ws.send(JSON.stringify({ type: 'use_jail_card', payload: {} }));
    });
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

function getPlayerColor(userId) {
    if (!gameState) return '#c7731a';
    const idx = gameState.players.findIndex(p => p.userId === userId);
    return idx >= 0 ? ['#FF4444','#4444FF','#44FF44','#FFFF44'][idx] : '#c7731a';
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function addLog(message, type = 'event', container, userId = null, username = null) {
    const logDiv = container.querySelector('#gameLog');
    if (!logDiv) return;

    const entry = document.createElement('div');
    entry.className = `log-entry ${type}`;

    if (userId && username) {
        entry.classList.add('has-user');
        const color = getPlayerColor(userId);
        const prefix = type === 'chat' ? '' : '>>> ';
        entry.innerHTML = `${prefix}<span class="log-username" style="background-color:${color};">${escapeHtml(username)}</span>${type === 'chat' ? ': ' : ' '}${escapeHtml(message)}`;
    } else {
        entry.textContent = message;
    }

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

function openTradeModalWithPlayer(targetUserId, targetUsername, container) {
    // Open the trade modal first
    openTradeModal(container);

    // Then pre-select the target player
    const overlay = container.querySelector('#tradeModal');
    if (overlay) {
        const targetSelect = overlay.querySelector('#tradeTargetPlayer');
        if (targetSelect) {
            targetSelect.value = targetUserId;
            // Trigger change event to update the requested properties
            targetSelect.dispatchEvent(new Event('change'));
        }
    }
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
    showConfirmModal('Are you sure you want to accept this trade?', () => {
        ws.send(JSON.stringify({ type: 'accept_trade', payload: { tradeId } }));
    });
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
    if (!turnTimerEnd) {
        hideTimerElements(container);
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
        hideTimerElements(container);
        return;
    }

    // Build UTF-8 block progress bar (10 chars wide)
    const barWidth = 10;
    const filled = Math.round(progress * barWidth);
    const empty = barWidth - filled;
    const bar = '\u2588'.repeat(filled) + '\u2591'.repeat(empty);

    // Determine color class based on time remaining
    let colorClass = 'timer-normal';
    if (remaining <= 15) {
        colorClass = 'timer-critical';
    } else if (remaining <= 30) {
        colorClass = 'timer-warning';
    }

    // Show timer in players list for current player
    const playerTimerEl = container.querySelector('#playerTimer');
    if (playerTimerEl) {
        playerTimerEl.innerHTML = `<span class="${colorClass}">[${bar}] ${remaining}s</span>`;
    }
}

function hideTimerElements(container) {
    const playerTimerEl = container.querySelector('#playerTimer');
    if (playerTimerEl) playerTimerEl.innerHTML = '';
}

function stopTurnTimerDisplay(container) {
    if (turnTimerInterval) {
        clearInterval(turnTimerInterval);
        turnTimerInterval = null;
    }
    turnTimerEnd = null;
    hideTimerElements(container);
}

// Auction functions - inline controls
function showAuctionControls(position, propertyName, startingBid, bidderOrder, currentBidderId, userId, container) {
    activeAuction = {
        position,
        propertyName,
        highestBid: startingBid,
        highestBidderName: null,
        currentBidderId,
        bidderOrder
    };

    updateAuctionControls(userId, container);
}

function updateAuctionControls(userId, container) {
    const controls = container.querySelector('#auctionControls');
    const infoEl = container.querySelector('#auctionInfo');
    const bidBtn = container.querySelector('#auctionBidBtn');
    const passBtn = container.querySelector('#auctionPassBtn');

    if (!controls || !activeAuction) {
        if (controls) controls.style.display = 'none';
        return;
    }

    const isMyTurn = activeAuction.currentBidderId === userId;
    const nextBid = activeAuction.highestBid + 10;
    const minBid = Math.max(10, nextBid);

    // Build info text
    let infoText = `<span class="auction-property-name">${activeAuction.propertyName}</span>`;
    if (activeAuction.highestBid > 0) {
        infoText += ` | High: $${activeAuction.highestBid} (${activeAuction.highestBidderName})`;
    }

    if (isMyTurn) {
        infoText += ` | <span class="your-bid-turn">Your turn!</span>`;
    } else {
        infoText += ` | Waiting for ${getPlayerName(activeAuction.currentBidderId)}...`;
    }

    infoEl.innerHTML = infoText;
    bidBtn.textContent = `Bid $${minBid}`;
    bidBtn.dataset.bidAmount = minBid;

    // Show/hide buttons based on turn
    bidBtn.style.display = isMyTurn ? 'inline-block' : 'none';
    passBtn.style.display = isMyTurn ? 'inline-block' : 'none';

    controls.style.display = 'block';

    // Also show the game controls when auction is active
    const gameControls = container.querySelector('#gameControls');
    if (gameControls) gameControls.style.display = 'flex';
}

function updateAuctionBid(bidAmount, bidderName, nextBidderId, userId, container) {
    if (!activeAuction) return;

    if (bidAmount !== null) {
        activeAuction.highestBid = bidAmount;
        activeAuction.highestBidderName = bidderName;
    }

    activeAuction.currentBidderId = nextBidderId;
    updateAuctionControls(userId, container);
}

function hideAuctionControls(container) {
    activeAuction = null;
    const controls = container.querySelector('#auctionControls');
    if (controls) {
        controls.style.display = 'none';
    }
    updateControls(currentUserId, container);
}

function placeBid() {
    if (!ws || ws.readyState !== WebSocket.OPEN || !activeAuction) return;
    const bidBtn = document.querySelector('#auctionBidBtn');
    const amount = parseInt(bidBtn.dataset.bidAmount);
    if (amount > activeAuction.highestBid) {
        ws.send(JSON.stringify({ type: 'place_bid', payload: { amount } }));
    }
}

function passAuction() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'pass_auction', payload: {} }));
}

// Card Modal
function showCardModal(deckType, cardText, effect, container) {
    const modal = container.querySelector('#cardModal');
    if (!modal) return;

    const header = modal.querySelector('.card-header');
    const textEl = modal.querySelector('.card-text');
    const effectEl = modal.querySelector('.card-effect');

    header.textContent = deckType === 'chance' ? 'Chance' : 'Community Chest';
    header.className = 'card-header ' + deckType;
    textEl.textContent = cardText;
    effectEl.textContent = effect || '';
    effectEl.style.display = effect ? 'block' : 'none';

    modal.style.display = 'flex';

    // Close on overlay click
    const closeModal = (e) => {
        if (e.target === modal) {
            modal.style.display = 'none';
            modal.removeEventListener('click', closeModal);
        }
    };
    modal.addEventListener('click', closeModal);

    // Close on OK button
    const dismissBtn = modal.querySelector('.card-dismiss-btn');
    const closeOnDismiss = () => {
        modal.style.display = 'none';
        dismissBtn.removeEventListener('click', closeOnDismiss);
        modal.removeEventListener('click', closeModal);
    };
    dismissBtn.addEventListener('click', closeOnDismiss);
}

// Property Info Panel
function showPropertyInfoPanel(space, state, userId, container) {
    let panel = container.querySelector('#propertyInfoPanel');
    if (!panel) {
        panel = document.createElement('div');
        panel.id = 'propertyInfoPanel';
        panel.className = 'property-info-panel';
        const boardCenter = container.querySelector('.board-center');
        if (boardCenter) {
            boardCenter.appendChild(panel);
        } else {
            container.appendChild(panel);
        }
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

    // Click outside to close
    const boardCenter = container.querySelector('.board-center');
    if (boardCenter) {
        const clickOutsideHandler = (e) => {
            if (!panel.contains(e.target) && panel.style.display !== 'none') {
                hidePropertyInfoPanel(container);
                boardCenter.removeEventListener('click', clickOutsideHandler);
            }
        };
        // Delay adding the listener to prevent immediate close
        setTimeout(() => {
            boardCenter.addEventListener('click', clickOutsideHandler);
        }, 0);
    }

    // Set up global functions for the buttons
    window.hidePropertyPanel = () => hidePropertyInfoPanel(container);
    window.mortgageProperty = (pos) => {
        showConfirmModal('Are you sure you want to mortgage this property? You will need to pay 110% to unmortgage it.', () => {
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ type: 'mortgage_property', payload: { position: pos } }));
            }
        }, container);
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

