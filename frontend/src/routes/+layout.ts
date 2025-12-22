import type { LayoutLoad } from './$types';
import { gameStore } from '$lib/game.svelte';
import { resolve } from '$app/paths';

export const prerender = true;
export const ssr = false;

export const load: LayoutLoad = async ({ fetch }) => {
	await gameStore.restoreSession(fetch);
	if (!gameStore.user && window.location.pathname !== '/login') {
		let params = new URLSearchParams()
		params.set('redirect', window.location.pathname)
		window.location.href = resolve('/login') + `?${params}`
	}
};
