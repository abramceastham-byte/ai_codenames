import type { PageLoad } from './$types';
import { resolve } from '$app/paths';
import { gameStore } from '$lib/game.svelte';
import { goto } from '$app/navigation';

export const load: PageLoad = async () => {
	const redirect = new URLSearchParams(window.location.search).get('redirect')
	if (gameStore.user) {
		if (redirect && redirect.startsWith('/')) {
			await goto(redirect);
		} else {
			await goto(resolve('/'));
		}
	}
	return {
		hasRedirect: !!redirect,
	}
};

