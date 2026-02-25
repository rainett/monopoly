export class Router {
    constructor() {
        this.routes = new Map();
        this.currentRoute = null;
        this.defaultRoute = '/login';

        window.addEventListener('hashchange', () => this.handleRouteChange());
        window.addEventListener('load', () => this.handleRouteChange());
    }

    register(path, handler) {
        this.routes.set(path, handler);
    }

    navigate(path, params = {}) {
        const queryString = new URLSearchParams(params).toString();
        const hash = queryString ? `#${path}?${queryString}` : `#${path}`;
        window.location.hash = hash;
    }

    handleRouteChange() {
        const hash = window.location.hash.slice(1) || this.defaultRoute;
        const [path, queryString] = hash.split('?');
        const params = new URLSearchParams(queryString);

        const handler = this.routes.get(path);
        if (handler) {
            this.currentRoute = { path, params };
            handler(params);
        } else {
            this.navigate(this.defaultRoute);
        }
    }

    getCurrentRoute() {
        return this.currentRoute;
    }

    getParam(key) {
        return this.currentRoute?.params.get(key);
    }
}
