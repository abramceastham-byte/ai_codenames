import type { PageLoad } from './$types';
import { api } from '$lib/api';

export const load: PageLoad = async () => {
	return {
	  pendingGames: await api.getPendingGames()
	};
};

