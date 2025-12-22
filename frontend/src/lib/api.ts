import type { Game, Player, Team, Role, PlayerID } from './types';
import { PUBLIC_API_URL } from '$env/static/public';

export class Api {
	fetch: typeof window.fetch;

	constructor(fetch?: typeof window.fetch) {
		this.fetch = fetch ?? window.fetch;
	}

	async post<T>(url: string, body: any): Promise<T> {
		const res = await this.fetch(`${PUBLIC_API_URL}${url}`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(body),
			credentials: 'include'
		});
		if (!res.ok) {
			const text = await res.text();
			throw new Error(text || res.statusText);
		}
		return res.json();
	}

	async get<T>(url: string): Promise<T> {
		const res = await this.fetch(`${PUBLIC_API_URL}${url}`, { credentials: 'include' });
		if (!res.ok) throw new Error(res.statusText);
		return res.json();
	}

	async createUser(name: string): Promise<{ user_id: string; success: boolean }> {
		return this.post('/api/user', { name });
	}

	async getUser(): Promise<{ id: string; name: string } | null> {
		return this.get('/api/user');
	}

	async createGame(privateGame: boolean): Promise<{ id: string }> {
		return this.post('/api/game', { private: privateGame });
	}

	async getPendingGames(): Promise<string[]> {
		return this.get('/api/games');
	}

	async joinGame(id: string): Promise<{ success: boolean }> {
		return this.post(`/api/game/${id}/join`, {});
	}

	async getGame(id: string): Promise<Game> {
		return this.get(`/api/game/${id}`);
	}

	async getGamePlayers(id: string): Promise<Player[]> {
		return this.get(`/api/game/${id}/players`);
	}

	async assignRole(gameId: string, team: Team, role: Role): Promise<Player[]> {
		return this.post(`/api/game/${gameId}/assignRole`, {
			team,
			role
		});
	}

	async requestAI(gameId: string, team: Team, role: Role): Promise<void> {
		return this.post(`/api/game/${gameId}/requestAI`, {
			team,
			role
		});
	}

	async startGame(
		gameId: string,
		randomAssignment: boolean = false
	): Promise<{ success: boolean }> {
		return this.post(`/api/game/${gameId}/start`, { random_assignment: randomAssignment });
	}

	async sendClue(gameId: string, word: string, count: number): Promise<{ success: boolean }> {
		return this.post(`/api/game/${gameId}/clue`, { word, count });
	}

	async sendGuess(
		gameId: string,
		guess: string,
		confirmed: boolean = true
	): Promise<{ success: boolean }> {
		return this.post(`/api/game/${gameId}/guess`, { guess, confirmed });
	}
}
