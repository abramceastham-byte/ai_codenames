import type { PageLoad } from './$types';
import { Api } from '$lib/api';

export const load: PageLoad = async ({ fetch }) => {
	return {
		pendingGames: await new Api(fetch).getPendingGames()
	};
};
