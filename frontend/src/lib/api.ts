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

	async createUser(): Promise<{ user_id: string; name: string; success: boolean }> {
		return this.post('/api/user', {});
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

	async requestAI(gameId: string, team: Team, role: Role, backend?: string): Promise<void> {
		return this.post(`/api/game/${gameId}/requestAI`, {
			team,
			role,
			backend: backend ?? ''
		});
	}

	async removePlayer(gameId: string, playerId: string): Promise<Player[]> {
		return this.post(`/api/game/${gameId}/removePlayer`, { player_id: playerId });
	}

	async getAIBackends(): Promise<{ backends: string[]; default: string }> {
		return this.get('/api/ai/backends');
	}

	async startGame(
		gameId: string,
		randomAssignment: boolean = false
	): Promise<{ success: boolean }> {
		return this.post(`/api/game/${gameId}/start`, { random_assignment: randomAssignment });
	}

	// getClueHold reads whether this game withholds clues. Creator-only — the
	// server refuses anyone else, since knowing whether the hold is on tells an
	// operative something they shouldn't have.
	async getClueHold(gameId: string): Promise<{ enabled: boolean; delay_ms: number }> {
		return this.get(`/api/game/${gameId}/clueHold`);
	}

	// setClueHold turns this game's clue hold on or off, taking effect at once:
	// switching it off releases a clue that's mid-hold.
	async setClueHold(
		gameId: string,
		enabled: boolean
	): Promise<{ enabled: boolean; delay_ms: number }> {
		return this.post(`/api/game/${gameId}/clueHold`, { enabled });
	}

	// sendClue submits a clue. The server withholds it from the operatives until
	// the clue phase reaches its minimum length, and reports when that happens
	// as release_at_ms (Unix milliseconds).
	async sendClue(
		gameId: string,
		word: string,
		count: number
	): Promise<{ success: boolean; release_at_ms: number }> {
		return this.post(`/api/game/${gameId}/clue`, { word, count });
	}

	async sendGuess(
		gameId: string,
		guess: string,
		confirmed: boolean = true
	): Promise<{ success: boolean }> {
		return this.post(`/api/game/${gameId}/guess`, { guess, confirmed });
	}

	async saveLog(gameId: string, entries: unknown[]): Promise<{ path: string }> {
		return this.post(`/api/game/${gameId}/log`, { entries });
	}

	async getAdminGameLog(gameId: string, adminSecret: string): Promise<AdminGameLog> {
		const res = await this.fetch(`${PUBLIC_API_URL}/api/admin/game/${gameId}/log`, {
			credentials: 'include',
			headers: { 'X-Admin-Secret': adminSecret }
		});
		if (!res.ok) {
			const text = await res.text();
			throw new Error(text || res.statusText);
		}
		return res.json();
	}
}

// AdminPlayerID is distinct from the player-facing PlayerID type: the admin
// endpoint is the one place player_type is legitimately sent over the wire.
export interface AdminPlayerID {
	player_type: string;
	id: string;
}

export interface AdminPlayer {
	player_id: AdminPlayerID;
	name: string;
	team: Team;
	role: Role;
	backend?: string;
}

export interface AdminReasoningEntry {
	timestamp: string;
	game_id: string;
	round: number;
	team: Team;
	role: Role;
	backend: string;
	action: 'clue' | 'guess';
	detail: string;
	reasoning: string;
	error?: string;
}

export interface AdminGameLog {
	players: AdminPlayer[];
	reasoning: AdminReasoningEntry[];
}
