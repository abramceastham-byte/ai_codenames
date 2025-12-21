import type { PageLoad } from './$types';
import { api } from '$lib/api';
import { gameStore } from '$lib/game.svelte';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';

export const load: PageLoad = async ({ params }) => {
	await api.joinGame(params.id)
	if (!gameStore.user) {
		await goto(resolve('/login'));
		return
	}
	await gameStore.fetchGame(params.id)

	return {
		gameId: params.id
	};
};
