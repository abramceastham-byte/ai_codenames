<script lang="ts">
	import { Api } from '$lib/api';
	import { gameStore } from '$lib/game.svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';

	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const pendingGames = $derived(data.pendingGames);
	const api = new Api();

	let privateGame = $state(false);
	let joinId = $state('');
	let loading = $state(false);

	async function createGame() {
		loading = true;
		try {
			const res = await api.createGame(privateGame);
			await goto(resolve(`/game/${res.id}`));
		} catch (e) {
			alert('Failed to create game: ' + e);
		} finally {
			loading = false;
		}
	}

	async function joinGame(id: string) {
		if (!id) return;
		loading = true;
		try {
			await goto(resolve(`/game/${id}`));
		} catch (e) {
			alert('Failed to join game: ' + e);
		} finally {
			loading = false;
		}
	}
</script>

<div class="min-h-screen bg-stone-100 p-8">
	<div class="mx-auto max-w-4xl">
		<header class="mb-8 flex items-center justify-between">
			<h1 class="text-3xl font-bold text-gray-900">Lobby</h1>
			<div class="text-gray-600">
				Logged in as <span class="font-semibold text-gray-900">{gameStore.user?.name}</span>
			</div>
		</header>

		<div class="grid gap-6 md:grid-cols-2">
			<!-- Join Game Section -->
			<div class="rounded-lg bg-white p-6 shadow">
				<h2 class="mb-4 text-xl font-semibold text-gray-800">Join Game</h2>
				<div class="flex gap-2">
					<input
						type="text"
						bind:value={joinId}
						placeholder="Game ID"
						class="flex-1 rounded border border-gray-300 p-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none"
					/>
					<button
						onclick={() => joinGame(joinId)}
						disabled={loading}
						class="rounded bg-green-600 px-4 py-2 text-white hover:bg-green-700 disabled:opacity-50"
					>
						Join
					</button>
				</div>

				{#if pendingGames && pendingGames.length > 0}
					<div class="mt-6">
						<h3 class="mb-2 text-sm font-medium text-gray-500">Recently created lobbies</h3>
						<ul class="space-y-2">
							{#each pendingGames as gameId (gameId)}
								<li>
									<button
										onclick={() => joinGame(gameId)}
										class="w-full rounded border border-gray-200 bg-gray-50 px-3 py-2 text-left hover:bg-gray-100"
									>
										{gameId}
									</button>
								</li>
							{/each}
						</ul>
					</div>
				{:else}
					<p class="mt-4 text-sm text-gray-500">No recent lobbies found.</p>
				{/if}
			</div>

			<!-- Create Game Section -->
			<div class="rounded-lg bg-white p-6 shadow">
				<h2 class="mb-4 text-xl font-semibold text-gray-800">New Game</h2>
				<p class="mb-4 text-gray-600">Start a new match</p>
				<div class="my-2">
					<input name="private" type="checkbox" bind:checked={privateGame} />
					<label for="private">Private?</label>
				</div>
				<button
					onclick={createGame}
					disabled={loading}
					class="w-full rounded bg-blue-600 px-4 py-3 text-lg font-medium text-white hover:bg-blue-700 disabled:opacity-50"
				>
					Create Game
				</button>
			</div>
		</div>
	</div>
</div>
