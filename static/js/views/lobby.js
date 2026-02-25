import { api } from '../api.js';
import { templateLoader } from '../template.js';

let refreshInterval = null;

export async function render(container, router) {
    if (!api.isAuthenticated()) {
        router.navigate('/login');
        return;
    }

    const user = api.getCurrentUser();
    const template = await templateLoader.load('lobby');
    container.innerHTML = template;

    container.querySelector('#username').textContent = user.username;
    container.querySelector('#createGameBtn').addEventListener('click', () => createGame(router));
    container.querySelector('#logoutBtn').addEventListener('click', () => logout(router));

    loadGames(container, router);
    refreshInterval = setInterval(() => loadGames(container, router), 3000);
}

export function cleanup() {
    if (refreshInterval) {
        clearInterval(refreshInterval);
        refreshInterval = null;
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
            gamesListDiv.innerHTML = '<div class="error">Failed to load games. <button onclick="location.reload()">Retry</button></div>';
        }
    }
}

function displayGames(container, games, router) {
    const gamesListDiv = container.querySelector('#gamesList');

    if (!games || games.length === 0) {
        gamesListDiv.innerHTML = '<div class="empty-state">No games available. Create one!</div>';
        return;
    }

    gamesListDiv.innerHTML = games.map(game => `
        <div class="game-item">
            <div class="game-info">
                <div>Game #${game.id} - <span class="game-status ${game.status === 'in_progress' ? 'in-progress' : ''}">${game.status}</span></div>
                <div>Players: ${game.players.length}/${game.maxPlayers}</div>
            </div>
            <button
                class="join-game-btn"
                data-game-id="${game.id}"
                ${game.status !== 'waiting' || game.players.length >= game.maxPlayers ? 'disabled' : ''}
            >
                Join
            </button>
        </div>
    `).join('');

    container.querySelectorAll('.join-game-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const gameId = btn.dataset.gameId;
            joinGame(parseInt(gameId), router);
        });
    });
}

async function createGame(router) {
    try {
        const data = await api.createGame();
        await joinGame(data.gameId, router);
    } catch (error) {
        console.error('Failed to create game:', error);
        alert('Failed to create game: ' + error.message);
    }
}

async function joinGame(gameId, router) {
    try {
        await api.joinGame(gameId);
        router.navigate('/game', { gameId });
    } catch (error) {
        console.error('Failed to join game:', error);
        alert('Failed to join game: ' + error.message);
    }
}

async function logout(router) {
    try {
        await api.logout();
    } catch (error) {
        console.error('Logout failed:', error);
    }
    router.navigate('/login');
}
