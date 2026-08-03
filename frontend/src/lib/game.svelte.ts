import {
	type Game,
	type Player,
	type Team,
	type WsMessage,
	type PlayerVote,
	type PlayerID,
	type LogEntry,
	AGENT_RED,
	AGENT_BLUE,
	AGENT_BYSTANDER,
	AGENT_ASSASSIN
} from './types';
import { Api } from '$lib/api';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { PUBLIC_API_URL } from '$env/static/public';
import { SvelteMap } from 'svelte/reactivity';

export class GameStore {
	api = new Api();

	// User State
	user = $state<{ id: string; name: string } | null>(null);

	// Game State
	game = $state<Game | null>(null);
	players = $state<Player[]>([]);

	// Vote tracking: Map of playerID -> player votes
	votes = new SvelteMap<string, PlayerVote>();

	// Game history log
	history = $state<LogEntry[]>([]);
	private _actionStartTime = 0;
	private _teamClueCount: Record<string, number> = { RED: 0, BLUE: 0 };

	// Game timer
	gameStartTime = $state<number | null>(null);
	gameEndTime = $state<number | null>(null);

	// UI State
	connected = $state(false);
	error = $state<string | null>(null);
	lastClue = $derived.by<{ word: string; count: number; team: Team } | null>(() => {
		const clues = this.game?.state.clues;
		if (!clues || clues.length === 0) {
			return null;
		}
		const clue = clues[clues.length - 1];
		return { word: clue.clue.word, count: clue.clue.count, team: clue.team };
	});

	ws: WebSocket | null = null;

	async restoreSession(fetch: typeof window.fetch) {
		try {
			const u = await new Api(fetch).getUser();
			if (u) {
				this.user = u;
			}
		} catch (e) {
			console.error('Failed to restore session', e);
		}
	}

	async login() {
		const res = await this.api.createUser();
		if (res.success) {
			this.user = { id: res.user_id, name: res.name };
			const redirect = new URLSearchParams(window.location.search).get('redirect');
			if (redirect && redirect.startsWith('/')) {
				await goto(redirect);
			} else {
				await goto(resolve('/'));
			}
		}
	}

	async fetchGame(id: string) {
		try {
			this.game = await this.api.getGame(id);
			this.players = await this.api.getGamePlayers(id);
			// Recover the timer for a game we didn't see GAME_START for (e.g. a
			// page reload, or opening the board after the game was already
			// underway) — otherwise gameStartTime stays null and the elapsed
			// timer is frozen at 0:00 forever.
			if (this.gameStartTime === null && this.game.status !== 'PENDING') {
				const startedAt = new Date(this.game.started_at).getTime();
				if (!Number.isNaN(startedAt)) {
					this.gameStartTime = startedAt;
				}
			}
			this.connectWs(id);
		} catch (e) {
			this.error = 'Failed to load game: ' + e;
		}
	}

	connectWs(gameId: string) {
		if (this.ws) {
			this.ws.close();
		}

		const apiUrl = URL.parse(PUBLIC_API_URL);
		const host = apiUrl?.host ?? window.location.host;
		const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const url = `${proto}//${host}/api/game/${gameId}/ws`;

		this.ws = new WebSocket(url);
		this.ws.onopen = () => {
			this.connected = true;
		};
		this.ws.onclose = () => {
			this.connected = false;
		};
		this.ws.onmessage = (event) => {
			try {
				const msg = JSON.parse(event.data) as WsMessage;
				this.handleMessage(msg);
			} catch (e) {
				console.error('Failed to parse WS message', e);
			}
		};
	}

	handleMessage(msg: WsMessage) {
		if ('game' in msg) {
			this.game = msg.game;
		}

		switch (msg.action) {
			case 'GAME_START':
				if (msg.players) this.players = msg.players;
				this.votes.clear();
				this.history = [];
				this._teamClueCount = { RED: 0, BLUE: 0 };
				this._actionStartTime = Date.now();
				this.gameStartTime = Date.now();
				this.gameEndTime = null;
				break;
			case 'ROLE_ASSIGNED':
				if (msg.players) this.players = msg.players;
				break;
			case 'CLUE_GIVEN': {
				const duration = Date.now() - this._actionStartTime;
				if (msg.clue) {
					this.lastClue = { ...msg.clue, team: msg.team };
				}
				this._teamClueCount[msg.team] = (this._teamClueCount[msg.team] ?? 0) + 1;
				const clueRound = Math.max(
					this._teamClueCount['RED'] ?? 0,
					this._teamClueCount['BLUE'] ?? 0
				);
				this.history = [
					...this.history,
					{
						round: clueRound,
						team: msg.team,
						type: 'clue',
						detail: `${msg.clue.word} (${msg.clue.count})`,
						result: '',
						durationMs: duration
					}
				];
				this._actionStartTime = Date.now();
				break;
			}
			case 'GUESS_GIVEN': {
				const duration = Date.now() - this._actionStartTime;
				const round = Math.max(this._teamClueCount['RED'] ?? 0, this._teamClueCount['BLUE'] ?? 0);
				this.history = [
					...this.history,
					{
						round,
						team: msg.team,
						type: 'guess',
						detail: msg.guess || '(pass)',
						result: this._agentToResult(msg.card?.agent),
						durationMs: duration
					}
				];
				this.votes.clear();
				this._actionStartTime = Date.now();
				break;
			}
			case 'GAME_END':
				this.gameEndTime = Date.now();
				if (this.game && this.history.length > 0) {
					this.api.saveLog(this.game.id, this.history).catch((e) => {
						console.warn('Failed to save game log:', e);
					});
				}
				break;
			case 'PLAYER_VOTE':
				this.handlePlayerVote(msg);
				break;
		}
	}

	private _agentToResult(agent: number | undefined): string {
		switch (agent) {
			case AGENT_RED:
				return 'red';
			case AGENT_BLUE:
				return 'blue';
			case AGENT_BYSTANDER:
				return 'bystander';
			case AGENT_ASSASSIN:
				return 'assassin';
			default:
				return '';
		}
	}

	handlePlayerVote(msg: { player_id: PlayerID; guess: string; confirmed: boolean }) {
		const player = this.players.find((p) => p.player_id.id === msg.player_id.id);
		if (!player) return;

		this.votes.set(player.player_id.id, {
			playerId: msg.player_id,
			playerName: player.name,
			confirmed: msg.confirmed,
			guess: msg.guess
		});
	}

	get myPlayer(): Player | undefined {
		if (!this.user || !this.players) return undefined;
		return this.players.find((p) => p.player_id.id === this.user?.id);
	}

	get isMyTurn(): boolean {
		if (!this.game || !this.myPlayer) return false;
		const s = this.game.state;
		const p = this.myPlayer;
		return s.active_team === p.team && s.active_role === p.role;
	}
}

export const gameStore = new GameStore();
