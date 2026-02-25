import { api } from '../api.js';
import { templateLoader } from '../template.js';

export async function render(container, router) {
    const template = await templateLoader.load('login');
    container.innerHTML = template;

    const form = container.querySelector('#loginForm');
    const errorDiv = container.querySelector('#error');

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const username = container.querySelector('#username').value;
        const password = container.querySelector('#password').value;

        errorDiv.textContent = '';

        try {
            await api.login(username, password);
            router.navigate('/lobby');
        } catch (error) {
            errorDiv.textContent = error.message || 'Login failed';
        }
    });
}

export function cleanup() {
}
