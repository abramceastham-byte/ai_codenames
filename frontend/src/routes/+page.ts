import type { PageLoad } from './$types';
import { Api } from '$lib/api';
import { gameStore } from '$lib/game.svelte';
import { resolve } from '$app/paths';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	await parent();
	if (!gameStore.user) {
		const params = new URLSearchParams();
		params.set('redirect', resolve('/'));
		redirect(303, `${resolve('/login')}?${params}`);
	}

	return {
		pendingGames: await new Api(fetch).getPendingGames()
	};
};
