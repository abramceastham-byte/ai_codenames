<script lang="ts">
	import { gameStore } from '$lib/game.svelte';
	import { api } from '$lib/api';

	const { game, myPlayer, isMyTurn } = $derived(gameStore);

	let clueWord = $state('');
	let clueCount = $state(1);
	let sending = $state(false);

	async function submitClue() {
		if (!game || !clueWord) return;
		sending = true;
		try {
			await api.sendClue(game.id, clueWord, clueCount);
			clueWord = '';
			clueCount = 1;
		} catch (e) {
			alert('Failed to send clue: ' + e);
		} finally {
			sending = false;
		}
	}

	async function endTurn() {
		if (!game) return;
		await api.sendGuess(game.id, '');
	}
</script>

<div class="rounded-lg bg-white p-4 shadow-md">
	{#if !isMyTurn}
		<div class="text-center text-gray-500">
			Waiting for {game?.state.active_team}
			{game?.state.active_role}...
		</div>
	{:else if myPlayer?.role === 'SPYMASTER'}
		<form
			class="flex items-end gap-4"
			onsubmit={(e) => {
				e.preventDefault();
				submitClue();
			}}
		>
			<div class="flex-1">
				<label class="mb-1 block text-sm font-medium text-gray-700" for="clue-word">Clue Word</label
				>
				<input
					id="clue-word"
					type="text"
					bind:value={clueWord}
					class="w-full rounded border border-gray-300 p-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none"
					placeholder="e.g. Tree"
					required
				/>
			</div>
			<div class="w-24">
				<label class="mb-1 block text-sm font-medium text-gray-700" for="clue-count">Count</label>
				<input
					id="clue-count"
					type="number"
					min="0"
					max="9"
					bind:value={clueCount}
					class="w-full rounded border border-gray-300 p-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none"
					required
				/>
			</div>
			<button
				type="submit"
				disabled={sending}
				class="rounded bg-indigo-600 px-6 py-2 font-bold text-white hover:bg-indigo-700 disabled:opacity-50"
			>
				Give Clue
			</button>
		</form>
	{:else}
		<div class="flex items-center justify-between">
			<div class="text-lg font-medium text-gray-800">It's your turn to guess!</div>
			<button onclick={endTurn} class="text-red-600 hover:underline">End Turn</button>
			<div class="text-sm text-gray-500">Tap a card to guess.</div>
		</div>
	{/if}
</div>
