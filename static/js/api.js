class ApiClient {
    constructor() {
        this.baseURL = window.location.origin;
        this.csrfToken = null;
        this.csrfPromise = null;
    }

    async fetchCSRFToken() {
        // If already fetching, return the existing promise
        if (this.csrfPromise) {
            return this.csrfPromise;
        }

        this.csrfPromise = (async () => {
            try {
                const response = await fetch(`${this.baseURL}/api/csrf-token`, {
                    credentials: 'include',
                });
                if (!response.ok) {
                    throw new Error(`Failed to fetch CSRF token: ${response.status}`);
                }
                const data = await response.json();
                this.csrfToken = data.csrfToken;
                console.log('CSRF token fetched:', this.csrfToken ? 'success' : 'empty');
            } catch (error) {
                console.error('Failed to fetch CSRF token:', error);
                throw error;
            } finally {
                this.csrfPromise = null;
            }
        })();

        return this.csrfPromise;
    }

    async ensureCSRFToken() {
        if (!this.csrfToken) {
            await this.fetchCSRFToken();
        }
    }

    async request(endpoint, options = {}) {
        // Fetch CSRF token if this is a POST request
        if (options.method === 'POST' || options.method === 'PUT' || options.method === 'PATCH' || options.method === 'DELETE') {
            await this.ensureCSRFToken();
        }

        const headers = {
            'Content-Type': 'application/json',
            ...options.headers,
        };

        // Add CSRF token to state-changing requests
        if (this.csrfToken && (options.method === 'POST' || options.method === 'PUT' || options.method === 'PATCH' || options.method === 'DELETE')) {
            headers['X-CSRF-Token'] = this.csrfToken;
        }

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
                this.handleUnauthorized();
                throw new Error('Unauthorized');
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

    getWebSocketURL(gameId) {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        return `${protocol}//${window.location.host}/ws/game/${gameId}`;
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
