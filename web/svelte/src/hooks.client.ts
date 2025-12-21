import type { ClientInit } from '@sveltejs/kit';
import { gameStore } from '$lib/game.svelte';

export const init: ClientInit = async () => {
	await gameStore.restoreSession();
};
