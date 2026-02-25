import { api } from '../api.js';
import { templateLoader } from '../template.js';

export async function render(container, router) {
    const template = await templateLoader.load('register');
    container.innerHTML = template;

    const form = container.querySelector('#registerForm');
    const errorDiv = container.querySelector('#error');
    const successDiv = container.querySelector('#success');

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const username = container.querySelector('#username').value;
        const password = container.querySelector('#password').value;

        errorDiv.textContent = '';
        successDiv.textContent = '';

        try {
            await api.register(username, password);
            successDiv.textContent = 'Account created! Redirecting...';
            setTimeout(() => {
                router.navigate('/login');
            }, 1500);
        } catch (error) {
            errorDiv.textContent = error.message || 'Registration failed';
        }
    });
}

export function cleanup() {
}
