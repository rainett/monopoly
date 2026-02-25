import { Router } from './router.js';
import { api } from './api.js';
import * as LoginView from './views/login.js';
import * as RegisterView from './views/register.js';
import * as LobbyView from './views/lobby.js';
import * as GameView from './views/game.js';

class App {
    constructor() {
        this.router = new Router();
        this.container = document.getElementById('app');
        this.currentView = null;

        this.setupRoutes();
        this.setupAuthCheck();
    }

    setupRoutes() {
        this.router.register('/login', () => this.loadView(LoginView));
        this.router.register('/register', () => this.loadView(RegisterView));
        this.router.register('/lobby', () => this.loadView(LobbyView));
        this.router.register('/game', () => this.loadView(GameView));

        this.router.defaultRoute = api.isAuthenticated() ? '/lobby' : '/login';
    }

    setupAuthCheck() {
        const protectedRoutes = ['/lobby', '/game'];
        window.addEventListener('hashchange', () => {
            const currentPath = this.router.getCurrentRoute()?.path;
            if (protectedRoutes.includes(currentPath) && !api.isAuthenticated()) {
                this.router.navigate('/login');
            }
        });
    }

    async loadView(view) {
        try {
            if (this.currentView?.cleanup) {
                this.currentView.cleanup();
            }

            this.currentView = view;
            this.container.innerHTML = '';

            await view.render(this.container, this.router);
        } catch (error) {
            console.error('Error loading view:', error);
            this.container.innerHTML = `
                <div style="padding: 2rem; text-align: center; color: #f48771;">
                    <h2>Error Loading Page</h2>
                    <p>${error.message || 'An unexpected error occurred'}</p>
                    <button onclick="window.location.reload()">Reload</button>
                </div>
            `;
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    new App();
});
