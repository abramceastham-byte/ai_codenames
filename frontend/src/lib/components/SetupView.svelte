<script lang="ts">
	import { onMount } from 'svelte';
	import { gameStore } from '$lib/game.svelte';
	import { Api } from '$lib/api';
	import type { Team, Role, Player } from '$lib/types';
	import ClueHoldToggle from './ClueHoldToggle.svelte';

	const { game, players, user } = $derived(gameStore);
	const api = new Api();

	let backends = $state<string[]>([]);
	// Selected backend per "team:role" slot. Empty string means use server default.
	let selectedBackend = $state<Record<string, string>>({});

	onMount(async () => {
		try {
			const res = await api.getAIBackends();
			backends = res.backends ?? [];
		} catch (e) {
			console.warn('Failed to fetch AI backends', e);
		}
	});

	function slotKey(team: Team, role: Role): string {
		return `${team}:${role}`;
	}

	function getPlayers(team: Team, role: Role): Player[] {
		return players.filter((p) => p.team === team && p.role === role);
	}

	async function joinRole(team: Team, role: Role) {
		if (!game || !user || !gameStore.user) return;
		const newPlayers = await api.assignRole(game.id, team, role);
		gameStore.players = newPlayers;
	}

	// Surfaced above the teams — otherwise a failed lobby action rejects into the
	// void and the page just sits there looking like nothing happened.
	let actionError = $state<string | null>(null);

	async function addAI(team: Team, role: Role) {
		if (!game || !user || !gameStore.user) return;
		await api.requestAI(game.id, team, role, selectedBackend[slotKey(team, role)] ?? '');
	}

	async function removePlayer(p: Player) {
		if (!game) return;
		actionError = null;
		try {
			gameStore.players = await api.removePlayer(game.id, p.player_id.id);
		} catch (e) {
			actionError = e instanceof Error ? e.message : String(e);
		}
	}

	async function startGame() {
		if (!game) return;
		actionError = null;
		try {
			await api.startGame(game.id);
		} catch (e) {
			actionError = e instanceof Error ? e.message : String(e);
		}
	}

	async function startRandom() {
		if (!game) return;
		actionError = null;
		try {
			await api.startGame(game.id, true);
		} catch (e) {
			actionError = e instanceof Error ? e.message : String(e);
		}
	}

	// Derived check for startability
	const canStart = $derived.by(() => {
		const rS = getPlayers('RED', 'SPYMASTER').length;
		const rO = getPlayers('RED', 'OPERATIVE').length;
		const bS = getPlayers('BLUE', 'SPYMASTER').length;
		const bO = getPlayers('BLUE', 'OPERATIVE').length;
		return rS === 1 && bS === 1 && rO > 0 && bO > 0;
	});

	const unassigned = $derived.by(() => getPlayers('', ''));

	const isCreator = $derived(game && gameStore.user && game.created_by === gameStore.user.id);
	const creatorName = $derived.by(() => {
		return players.find((p) => game?.created_by === p.player_id.id)?.name ?? 'game creator';
	});
</script>

<div class="mx-auto max-w-6xl p-4">
	<div class="mb-8 flex items-center justify-between">
		<h1 class="text-3xl font-bold text-gray-800">Game Setup: {game?.id}</h1>
		<div class="space-x-2">
			{#if isCreator}
				<button
					onclick={startRandom}
					disabled={players.length < 4}
					title={players.length < 4 ? 'You need at least four players to start' : undefined}
					class="rounded bg-purple-600 px-4 py-2 font-bold text-white hover:bg-purple-700 disabled:bg-gray-400"
				>
					Randomize & Start
				</button>
				<button
					onclick={startGame}
					disabled={!canStart}
					class="rounded bg-green-600 px-4 py-2 font-bold text-white hover:bg-green-700 disabled:bg-gray-400"
				>
					Start Game
				</button>
			{/if}
		</div>
	</div>

	{#if actionError}
		<div class="mb-6 rounded border border-red-300 bg-red-50 p-3 text-center text-red-700">
			{actionError}
		</div>
	{/if}

	<ClueHoldToggle />

	{#snippet joinButton(team: Team, role: Role, classes: string)}
		<button
			onclick={() => joinRole(team, role)}
			class="{classes} rounded-sm px-2 py-1 text-xs font-bold disabled:cursor-not-allowed disabled:opacity-50"
			disabled={getPlayers(team, role).length > 0}
		>
			Join
		</button>
	{/snippet}

	{#snippet aiButton(team: Team, role: Role, classes: string)}
		{#if isCreator}
			{#if backends.length > 1}
				<select
					bind:value={selectedBackend[slotKey(team, role)]}
					class="rounded-sm border border-gray-300 bg-white px-1 py-1 text-xs font-bold text-gray-700"
					title="AI backend"
				>
					<option value="">default</option>
					{#each backends as b (b)}
						<option value={b}>{b}</option>
					{/each}
				</select>
			{/if}
			<button
				onclick={() => addAI(team, role)}
				class="{classes} rounded-sm px-2 py-1 text-xs font-bold disabled:cursor-not-allowed disabled:opacity-50"
				disabled={getPlayers(team, role).length > 0}
			>
				Add AI
			</button>
		{/if}
	{/snippet}

	{#snippet joinButtons(team: Team, role: Role, classes: string)}
		<div class="flex gap-2">
			{@render aiButton(team, role, classes)}
			{@render joinButton(team, role, classes)}
		</div>
	{/snippet}

	{#snippet playerList(team: Team, role: Role, dotClass: string)}
		{#each getPlayers(team, role) as p (p.player_id.id)}
			<div class="group flex items-center gap-2">
				<div class="h-2 w-2 rounded-full {dotClass}"></div>
				<span class="flex-1">{p.name}</span>
				{#if p.player_id.id !== game?.created_by && (p.player_id.id === user?.id || isCreator)}
					{@const isSelf = p.player_id.id === user?.id}
					<button
						onclick={() => removePlayer(p)}
						title={isSelf ? 'Leave this role' : `Remove ${p.name} from the game`}
						aria-label={isSelf ? 'Leave this role' : `Remove ${p.name} from the game`}
						class="rounded-sm px-1.5 text-xs font-bold text-gray-400 opacity-0 group-hover:opacity-100 hover:bg-gray-100 hover:text-red-600 focus:opacity-100"
					>
						&times;
					</button>
				{/if}
			</div>
		{/each}
	{/snippet}

	<div class="grid grid-cols-2 gap-8">
		<!-- RED TEAM -->
		<div class="rounded-xl border-4 border-red-200 bg-red-50 p-6">
			<h2 class="mb-6 text-center text-2xl font-bold text-red-800">RED TEAM</h2>

			<div class="mb-6">
				<div class="mb-2 flex items-center justify-between">
					<h3 class="font-semibold text-red-900">Spymaster</h3>
					{@render joinButtons('RED', 'SPYMASTER', 'bg-red-200 text-red-800 hover:bg-red-300')}
				</div>
				<div class="min-h-[60px] rounded bg-white p-4 shadow-sm">
					{@render playerList('RED', 'SPYMASTER', 'bg-red-500')}
				</div>
			</div>

			<div>
				<div class="mb-2 flex items-center justify-between">
					<h3 class="font-semibold text-red-900">Operatives</h3>
					{@render joinButtons('RED', 'OPERATIVE', 'bg-red-200 text-red-800 hover:bg-red-300')}
				</div>
				<div class="min-h-[100px] rounded bg-white p-4 shadow-sm">
					{@render playerList('RED', 'OPERATIVE', 'bg-red-500')}
				</div>
			</div>
		</div>

		<!-- BLUE TEAM -->
		<div class="rounded-xl border-4 border-blue-200 bg-blue-50 p-6">
			<h2 class="mb-6 text-center text-2xl font-bold text-blue-800">BLUE TEAM</h2>

			<div class="mb-6">
				<div class="mb-2 flex items-center justify-between">
					<h3 class="font-semibold text-blue-900">Spymaster</h3>
					{@render joinButtons('BLUE', 'SPYMASTER', 'bg-blue-200 text-blue-800 hover:bg-blue-300')}
				</div>
				<div class="min-h-[60px] rounded bg-white p-4 shadow-sm">
					{@render playerList('BLUE', 'SPYMASTER', 'bg-blue-500')}
				</div>
			</div>

			<div>
				<div class="mb-2 flex items-center justify-between">
					<h3 class="font-semibold text-blue-900">Operatives</h3>
					{@render joinButtons('BLUE', 'OPERATIVE', 'bg-blue-200 text-blue-800 hover:bg-blue-300')}
				</div>
				<div class="min-h-[100px] rounded bg-white p-4 shadow-sm">
					{@render playerList('BLUE', 'OPERATIVE', 'bg-blue-500')}
				</div>
			</div>
		</div>
	</div>

	<div class="mt-8 text-center text-gray-500">
		{#if canStart}
			{#if unassigned.length > 0}
				<p>
					{unassigned.map((p) => p.name).join(', ')}
					{unassigned.length === 1 ? 'will spectate' : 'will spectate'}. Waiting for {creatorName} to
					start the game...
				</p>
			{:else}
				<p>Waiting for {creatorName} to start the game...</p>
			{/if}
		{:else if players.length < 4}
			<p>Waiting for more players...</p>
		{:else}
			<p>Waiting for all roles to be filled...</p>
		{/if}
	</div>
</div>
