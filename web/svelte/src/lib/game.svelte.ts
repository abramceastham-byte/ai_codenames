import { type Game, type Player, type Team, type WsMessage } from './types';
import { api } from './api';
import { goto } from '$app/navigation';

export class GameStore {
	// User State
	user = $state<{ id: string; name: string } | null>(null);

	// Game State
	game = $state<Game | null>(null);
	players = $state<Player[]>([]);

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

	async restoreSession() {
		try {
			const u = await api.getUser();
			if (u) {
				this.user = u;
			}
		} catch (e) {
			console.error('Failed to restore session', e);
		}
	}

	async login(name: string) {
		const res = await api.createUser(name);
		if (res.success) {
			this.user = { id: res.user_id, name };
			await goto('/lobby');
		}
	}

	async fetchGame(id: string) {
		try {
			this.game = await api.getGame(id);
			this.players = await api.getGamePlayers(id);
			this.connectWs(id);
		} catch (e) {
			this.error = 'Failed to load game: ' + e;
		}
	}

	connectWs(gameId: string) {
		if (this.ws) {
			this.ws.close();
		}

		// Use current host but upgrade protocol
		const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const url = `${proto}//localhost:8080/api/game/${gameId}/ws`;

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
		console.log('WS Msg:', msg);
		if ('game' in msg) {
			this.game = msg.game;
		}

		switch (msg.action) {
			case 'GAME_START':
				if (msg.players) this.players = msg.players;
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
				// but we could animate or show a toast here
				break;
			case 'GAME_END':
				// Show victory screen logic could go here
				break;
		}
	}

	get myPlayer(): Player | undefined {
		if (!this.user || !this.players) return undefined;
		return this.players.find(
			(p) => p.player_id.id === this.user?.id && p.player_id.player_type === 'HUMAN'
		);
	}

	get isMyTurn(): boolean {
		if (!this.game || !this.myPlayer) return false;
		const s = this.game.state;
		const p = this.myPlayer;
		return s.active_team === p.team && s.active_role === p.role;
	}
}

export const gameStore = new GameStore();
