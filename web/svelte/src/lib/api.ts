import type { Game, Player, GameState, Clue, Team, Role, PlayerID } from './types';

class Api {
	async post<T>(url: string, body: any): Promise<T> {
		const res = await fetch(`http://localhost:8080${url}`, {
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
		const res = await fetch(`http://localhost:8080${url}`, { credentials: 'include' });
		if (!res.ok) throw new Error(res.statusText);
		return res.json();
	}

	async createUser(name: string): Promise<{ user_id: string; success: boolean }> {
		return this.post('/api/user', { name });
	}

	async getUser(): Promise<{ id: string; name: string } | null> {
		return this.get('/api/user');
	}

	async createGame(): Promise<{ id: string }> {
		return this.post('/api/game', {});
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

	async assignRole(gameId: string, playerId: PlayerID, team: Team, role: Role): Promise<Player[]> {
		return this.post(`/api/game/${gameId}/assignRole`, {
			player_id: playerId,
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

	async sendGuess(gameId: string, guess: string): Promise<{ success: boolean }> {
		return this.post(`/api/game/${gameId}/guess`, { guess, confirmed: true });
	}
}

export const api = new Api();
