<script lang="ts">
	import { gameStore } from '$lib/game.svelte';
	import { api } from '$lib/api';
	import type { Team, Role, Player } from '$lib/types';

	const { game, players, user } = $derived(gameStore);

	function getPlayers(team: Team, role: Role): Player[] {
		return players.filter((p) => p.team === team && p.role === role);
	}

	async function joinRole(team: Team, role: Role) {
		if (!game || !user || !gameStore.user) return;
        // Need to construct PlayerID properly
		await api.assignRole(game.id, { player_type: 'HUMAN', id: gameStore.user.id }, team, role);
		// Update players list locally or wait for WS? 
        // WS usually sends updates, but we also re-fetch in api wrapper usually.
        // Actually api.assignRole returns Player[].
        const newPlayers = await api.getGamePlayers(game.id);
        gameStore.players = newPlayers;
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

    const isCreator = $derived(game && gameStore.user && game.created_by === gameStore.user.id);
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

	<div class="grid grid-cols-2 gap-8">
		<!-- RED TEAM -->
		<div class="rounded-xl border-4 border-red-200 bg-red-50 p-6">
			<h2 class="mb-6 text-center text-2xl font-bold text-red-800">RED TEAM</h2>

			<div class="mb-6">
				<div class="mb-2 flex items-center justify-between">
					<h3 class="font-semibold text-red-900">Spymaster</h3>
					<button
						onclick={() => joinRole('RED', 'SPYMASTER')}
						class="rounded-sm bg-red-200 px-2 py-1 text-xs font-bold text-red-800 hover:bg-red-300"
					>
						JOIN
					</button>
				</div>
				<div class="min-h-[60px] rounded bg-white p-4 shadow-sm">
					{#each getPlayers('RED', 'SPYMASTER') as p}
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
					<button
						onclick={() => joinRole('RED', 'OPERATIVE')}
						class="rounded-sm bg-red-200 px-2 py-1 text-xs font-bold text-red-800 hover:bg-red-300"
					>
						JOIN
					</button>
				</div>
				<div class="min-h-[100px] rounded bg-white p-4 shadow-sm">
					{#each getPlayers('RED', 'OPERATIVE') as p}
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
					<button
						onclick={() => joinRole('BLUE', 'SPYMASTER')}
						class="rounded-sm bg-blue-200 px-2 py-1 text-xs font-bold text-blue-800 hover:bg-blue-300"
					>
						JOIN
					</button>
				</div>
				<div class="min-h-[60px] rounded bg-white p-4 shadow-sm">
					{#each getPlayers('BLUE', 'SPYMASTER') as p}
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
					<button
						onclick={() => joinRole('BLUE', 'OPERATIVE')}
						class="rounded-sm bg-blue-200 px-2 py-1 text-xs font-bold text-blue-800 hover:bg-blue-300"
					>
						JOIN
					</button>
				</div>
				<div class="min-h-[100px] rounded bg-white p-4 shadow-sm">
					{#each getPlayers('BLUE', 'OPERATIVE') as p}
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
        <p>Waiting for players...</p>
    </div>
</div>
