import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		// Proxy the API through the dev server so API calls are same-origin.
		// Otherwise the auth cookie is dropped whenever the browser's host
		// doesn't exactly match the API host (e.g. 127.0.0.1 vs localhost):
		// it's SameSite=Lax, and those count as different sites.
		// ws: true also covers the /api/game/<id>/ws upgrade.
		proxy: {
			'/api': {
				target: 'http://localhost:8080',
				ws: true
			}
		}
	}
});
