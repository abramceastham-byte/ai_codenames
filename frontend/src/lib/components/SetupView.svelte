<script lang="ts">
	import { gameStore } from '$lib/game.svelte';
	import { Api } from '$lib/api';
	import type { Team, Role, Player } from '$lib/types';

	const { game, players, user } = $derived(gameStore);
	const api = new Api();

	function getPlayers(team: Team, role: Role): Player[] {
		return players.filter((p) => p.team === team && p.role === role);
	}

	async function joinRole(team: Team, role: Role) {
		if (!game || !user || !gameStore.user) return;
		const newPlayers = await api.assignRole(game.id, team, role);
		gameStore.players = newPlayers;
	}

	async function addAI(team: Team, role: Role) {
		if (!game || !user || !gameStore.user) return;
		await api.requestAI(game.id, team, role);
	}

	async function startGame() {
		if (!game) return;
		await api.startGame(game.id);
	}

	async function startRandom() {
		if (!game) return;
		await api.startGame(game.id, true);
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
					class="rounded bg-purple-600 px-4 py-2 font-bold text-white hover:bg-purple-700"
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

	{#snippet joinButton(team: Team, role: Role, classes: string)}
		<button
			onclick={() => joinRole(team, role)}
			class="{classes} rounded-sm px-2 py-1 text-xs font-bold disabled:cursor-not-allowed disabled:opacity-50"
			disabled={getPlayers(team, role).length > (role === 'OPERATIVE' ? 9 : 0)}
		>
			Join
		</button>
	{/snippet}

	{#snippet aiButton(team: Team, role: Role, classes: string)}
		{#if isCreator}
			<button
				onclick={() => addAI(team, role)}
				class="{classes} rounded-sm px-2 py-1 text-xs font-bold disabled:cursor-not-allowed disabled:opacity-50"
				disabled={getPlayers(team, role).length > (role === 'OPERATIVE' ? 9 : 0)}
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
					{#each getPlayers('RED', 'SPYMASTER') as p (p.player_id.id)}
						<div class="flex items-center gap-2">
							<div class="h-2 w-2 rounded-full bg-red-500"></div>
							{p.name}
						</div>
					{/each}
				</div>
			</div>

			<div>
				<div class="mb-2 flex items-center justify-between">
					<h3 class="font-semibold text-red-900">Operatives</h3>
					{@render joinButtons('RED', 'OPERATIVE', 'bg-red-200 text-red-800 hover:bg-red-300')}
				</div>
				<div class="min-h-[100px] rounded bg-white p-4 shadow-sm">
					{#each getPlayers('RED', 'OPERATIVE') as p (p.player_id.id)}
						<div class="flex items-center gap-2">
							<div class="h-2 w-2 rounded-full bg-red-500"></div>
							{p.name}
						</div>
					{/each}
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
					{#each getPlayers('BLUE', 'SPYMASTER') as p (p.player_id.id)}
						<div class="flex items-center gap-2">
							<div class="h-2 w-2 rounded-full bg-blue-500"></div>
							{p.name}
						</div>
					{/each}
				</div>
			</div>

			<div>
				<div class="mb-2 flex items-center justify-between">
					<h3 class="font-semibold text-blue-900">Operatives</h3>
					{@render joinButtons('BLUE', 'OPERATIVE', 'bg-blue-200 text-blue-800 hover:bg-blue-300')}
				</div>
				<div class="min-h-[100px] rounded bg-white p-4 shadow-sm">
					{#each getPlayers('BLUE', 'OPERATIVE') as p (p.player_id.id)}
						<div class="flex items-center gap-2">
							<div class="h-2 w-2 rounded-full bg-blue-500"></div>
							{p.name}
						</div>
					{/each}
				</div>
			</div>
		</div>
	</div>

	<div class="mt-8 text-center text-gray-500">
		{#if unassigned.length > 0}
			{#if unassigned.length === 1}
				<p>Waiting for role to be assigned to {unassigned[0].name}</p>
			{:else}
				<p>Waiting for roles to be assigned to {unassigned.map((p) => p.name).join(', ')}</p>
			{/if}
		{:else if canStart}
			<p>Waiting for {creatorName} to start the game...</p>
		{:else if players.length < 4}
			<p>Waiting for more players...</p>
		{:else}
			<p>Waiting for all roles to be filled...</p>
		{/if}
	</div>
</div>
