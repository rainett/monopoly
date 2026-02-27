class ApiClient {
    constructor() {
        this.baseURL = window.location.origin;
    }

    async request(endpoint, options = {}) {
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers,
        };

        const config = {
            ...options,
            credentials: 'include',
            headers,
        };

        // Add timeout to fetch request
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), options.timeout || 30000);
        config.signal = controller.signal;

        try {
            const response = await fetch(`${this.baseURL}${endpoint}`, config);
            clearTimeout(timeoutId);

            if (response.status === 401) {
                // Don't redirect on login/register endpoints, just return the error
                if (endpoint !== '/api/auth/login' && endpoint !== '/api/auth/register') {
                    this.handleUnauthorized();
                    throw new Error('Unauthorized');
                }
                // For login/register, get the actual error message
                const errorText = await response.text();
                throw new Error(errorText || 'Invalid credentials');
            }

            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(errorText || `HTTP ${response.status}`);
            }

            const contentType = response.headers.get('content-type');
            if (contentType && contentType.includes('application/json')) {
                return await response.json();
            }

            return await response.text();
        } catch (error) {
            clearTimeout(timeoutId);
            if (error.name === 'AbortError') {
                throw new Error('Request timeout');
            }
            throw error;
        }
    }

    handleUnauthorized() {
        try {
            localStorage.clear();
        } catch (e) {
            console.warn('Failed to clear localStorage:', e);
        }
        if (window.location.hash !== '#/login' && window.location.hash !== '#/register') {
            window.location.hash = '#/login';
        }
    }

    async register(username, password) {
        return this.request('/api/auth/register', {
            method: 'POST',
            body: JSON.stringify({ username, password }),
        });
    }

    async login(username, password) {
        const data = await this.request('/api/auth/login', {
            method: 'POST',
            body: JSON.stringify({ username, password }),
        });

        if (data.userId && data.username) {
            try {
                localStorage.setItem('userId', data.userId);
                localStorage.setItem('username', data.username);
            } catch (e) {
                console.warn('Failed to save to localStorage:', e);
            }
        }

        return data;
    }

    async logout() {
        try {
            await this.request('/api/auth/logout', { method: 'POST' });
        } finally {
            try {
                localStorage.clear();
            } catch (e) {
                console.warn('Failed to clear localStorage:', e);
            }
        }
    }

    async listGames() {
        return this.request('/api/lobby/games');
    }

    async getGame(gameId) {
        return this.request(`/api/lobby/games/${gameId}`);
    }

    async createGame(maxPlayers = 4) {
        return this.request('/api/lobby/create', {
            method: 'POST',
            body: JSON.stringify({ maxPlayers }),
        });
    }

    async joinGame(gameId) {
        return this.request(`/api/lobby/join/${gameId}`, {
            method: 'POST',
        });
    }

    getWebSocketURL(target) {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        if (target === 'lobby') {
            return `${protocol}//${window.location.host}/ws/lobby`;
        }
        // Assume target is a gameId
        return `${protocol}//${window.location.host}/ws/game/${target}`;
    }

    isAuthenticated() {
        try {
            return !!localStorage.getItem('userId');
        } catch (e) {
            // localStorage may not be available in private browsing
            console.warn('localStorage not available:', e);
            return false;
        }
    }

    getCurrentUser() {
        try {
            const userId = localStorage.getItem('userId');
            const username = localStorage.getItem('username');

            const parsedUserId = parseInt(userId);
            if (isNaN(parsedUserId)) {
                return { userId: null, username: null };
            }

            return {
                userId: parsedUserId,
                username: username,
            };
        } catch (e) {
            console.warn('Error accessing localStorage:', e);
            return { userId: null, username: null };
        }
    }
}

export const api = new ApiClient();
