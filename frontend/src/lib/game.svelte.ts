import {
	type Game,
	type Player,
	type Team,
	type WsMessage,
	type PlayerVote,
	type PlayerID
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

	async login(name: string) {
		const res = await this.api.createUser(name);
		if (res.success) {
			this.user = { id: res.user_id, name };
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
				// Clear votes when game starts
				this.votes.clear();
				break;
			case 'ROLE_ASSIGNED':
				if (msg.players) this.players = msg.players;
				break;
			case 'CLUE_GIVEN':
				if (msg.clue) {
					this.lastClue = { ...msg.clue, team: msg.team };
				}
				break;
			case 'GUESS_GIVEN':
				// Card update is handled by msg.game update above,
				// Clear votes after a guess is confirmed
				this.votes.clear();
				break;
			case 'GAME_END':
				// Show victory screen logic could go here
				break;
			case 'PLAYER_VOTE':
				this.handlePlayerVote(msg);
				break;
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
