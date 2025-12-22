import type { PageLoad } from './$types';
import { Api } from '$lib/api';
import { gameStore } from '$lib/game.svelte';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';

export const prerender = false;

export const load: PageLoad = async ({ params, fetch, parent }) => {
	await parent();
	await new Api(fetch).joinGame(params.id);
	if (!gameStore.user) {
		await goto(resolve('/login'));
		return;
	}
	await gameStore.fetchGame(params.id);

	return {
		gameId: params.id
	};
};
