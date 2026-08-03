import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		// Lets the dev server accept requests tunneled through a Cloudflare
		// quick tunnel (e.g. `cloudflared tunnel --url http://localhost:5173`),
		// whose hostname changes every run, and requests from sibling
		// containers reaching the host via Docker Desktop's fixed hostname
		// (e.g. a browser-automation container testing this dev server). Vite
		// blocks unrecognized Host headers by default to prevent DNS rebinding.
		allowedHosts: ['.trycloudflare.com', 'host.docker.internal'],
		// Proxy the API through the dev server so API calls are same-origin.
		// Otherwise the auth cookie is dropped whenever the browser's host
		// doesn't exactly match the API host (e.g. 127.0.0.1 vs localhost):
		// it's SameSite=Lax, and those count as different sites.
		// ws: true also covers the /api/game/<id>/ws upgrade.
		proxy: {
			'/api': {
				target: process.env.API_PROXY_TARGET ?? 'http://localhost:8080',
				ws: true
			}
		}
	}
});
