import type { PageLoad } from './$types';
import { resolve } from '$app/paths';
import { gameStore } from '$lib/game.svelte';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ parent }) => {
	await parent();
	const redirectLoc = new URLSearchParams(window.location.search).get('redirect');
	if (gameStore.user) {
		if (redirectLoc && redirectLoc.startsWith('/')) {
			redirect(303, redirectLoc);
		} else {
			redirect(303, resolve('/'));
		}
	}
	return {
		hasRedirectToGame: redirectLoc?.startsWith('/game/') ?? false
	};
};
