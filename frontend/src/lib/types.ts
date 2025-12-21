export type Team = 'RED' | 'BLUE' | '';
export type Role = 'SPYMASTER' | 'OPERATIVE' | '';
export type Agent = number; // 0=Unknown, 1=Red, 2=Blue, 3=Bystander, 4=Assassin

// Constants for Agent types to make code readable
export const AGENT_UNKNOWN = 0;
export const AGENT_RED = 1;
export const AGENT_BLUE = 2;
export const AGENT_BYSTANDER = 3;
export const AGENT_ASSASSIN = 4;

export interface PlayerID {
	player_type: string;
	id: string;
}

export interface Player {
	player_id: PlayerID;
	name: string;
	team: Team;
	role: Role;
}

export interface Card {
	codeword: string;
	agent: Agent;
	revealed: boolean;
	revealed_by: Team;
}

export interface Board {
	cards: Card[];
}

export interface GameState {
	active_team: Team;
	active_role: Role;
	board: Board;
	clues: SpymasterClue[];
	num_guesses_left: number;
	starting_team: Team;
	winning_team: Team;
}

export interface SpymasterClue {
	clue: Clue;
	team: Team;
}

export interface Game {
	id: string;
	created_by: string; // UserID is string
	status: 'PENDING' | 'PLAYING' | 'FINISHED';
	state: GameState;
}

export interface Clue {
	word: string;
	count: number;
}

// WS Messages
export interface GameStartMsg {
	action: 'GAME_START';
	game: Game;
	players: Player[];
}

export interface RoleAssignedMsg {
	action: 'ROLE_ASSIGNED';
	players: Player[];
}

export interface ClueGivenMsg {
	action: 'CLUE_GIVEN';
	clue: Clue;
	team: Team;
	game: Game;
}

export interface GuessGivenMsg {
	action: 'GUESS_GIVEN';
	guess: string;
	team: Team;
	can_keep_guessing: boolean;
	card: Card;
	game: Game;
}

export interface GameEndMsg {
	action: 'GAME_END';
	winning_team: Team;
	game: Game;
}

export type WsMessage = GameStartMsg | RoleAssignedMsg | ClueGivenMsg | GuessGivenMsg | GameEndMsg;
