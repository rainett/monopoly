class TemplateLoader {
    constructor() {
        this.cache = new Map();
    }

    async load(templateName) {
        if (this.cache.has(templateName)) {
            return this.cache.get(templateName);
        }

        try {
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), 10000);

            const response = await fetch(`/templates/${templateName}.html`, {
                signal: controller.signal
            });

            clearTimeout(timeoutId);

            if (!response.ok) {
                throw new Error(`Failed to load template: ${templateName} (${response.status})`);
            }

            const html = await response.text();

            if (!html || html.trim() === '') {
                throw new Error(`Template ${templateName} is empty`);
            }

            this.cache.set(templateName, html);
            return html;
        } catch (error) {
            console.error(`Template loading error for ${templateName}:`, error);

            if (error.name === 'AbortError') {
                throw new Error(`Template ${templateName} loading timeout`);
            }

            // Return fallback error template
            const fallback = `<div class="error">Failed to load template: ${templateName}</div>`;
            return fallback;
        }
    }

    clearCache() {
        this.cache.clear();
    }
}

export const templateLoader = new TemplateLoader();
