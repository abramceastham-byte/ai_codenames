<script lang="ts">
	import { gameStore } from '$lib/game.svelte';
	import { Api } from '$lib/api';
	import type { PlayerVote } from '$lib/types';

	const { game, myPlayer, isMyTurn } = $derived(gameStore);
	const api = new Api();

	let clueWord = $state('');
	let clueCount = $state(1);
	let sending = $state(false);

	// The server withholds a clue until the clue phase has run its minimum
	// length, so that a fast answer doesn't give away who — or what — gave it.
	// Until then the form stays locked and shows the countdown instead.
	let releaseAtMs = $state<number | null>(null);
	let now = $state(Date.now());
	const secondsHeld = $derived(
		releaseAtMs === null ? 0 : Math.max(0, Math.ceil((releaseAtMs - now) / 1000))
	);
	const held = $derived(secondsHeld > 0);

	$effect(() => {
		if (!held) return;
		const ticker = setInterval(() => {
			now = Date.now();
		}, 250);
		return () => clearInterval(ticker);
	});

	async function submitClue() {
		if (!game || !clueWord || held) return;
		sending = true;
		try {
			const res = await api.sendClue(game.id, clueWord, clueCount);
			clueWord = '';
			clueCount = 1;
			now = Date.now();
			releaseAtMs = res.release_at_ms ?? null;
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

	const votesToEnd = $derived.by(() => {
		const vte: PlayerVote[] = [];
		gameStore.votes.forEach((pv) => {
			if (pv.guess !== '') {
				return;
			}
			vte.push(pv);
		});
		return vte;
	});
</script>

<div class="rounded-lg bg-white p-4 shadow-md">
	{#if !isMyTurn}
		<div class="text-center text-gray-500">
			Waiting for {game?.state.active_team}
			{game?.state.active_role}...
		</div>
	{:else if myPlayer?.role === 'SPYMASTER'}
		{#if held}
			<div class="mb-3 rounded border border-amber-200 bg-amber-50 p-3 text-center">
				<div class="font-medium text-amber-900">
					Clue sent — your operatives will see it in {secondsHeld}s
				</div>
				<div class="mt-1 text-sm text-amber-700">
					Clues are always released at the same point in the turn, so how fast you answered stays
					between us.
				</div>
			</div>
		{/if}
		<form
			class="flex flex-col gap-4 sm:flex-row sm:items-end"
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
					disabled={held}
					class="w-full rounded border border-gray-300 p-2 uppercase focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none disabled:bg-gray-100"
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
					disabled={held}
					class="w-full rounded border border-gray-300 p-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none disabled:bg-gray-100"
					required
				/>
			</div>
			<button
				type="submit"
				disabled={sending || held}
				class="rounded bg-indigo-600 px-6 py-2 font-bold text-white hover:bg-indigo-700 disabled:opacity-50"
			>
				{held ? `Holding (${secondsHeld}s)` : 'Give Clue'}
			</button>
		</form>
	{:else}
		<div class="flex items-center justify-between">
			<div class="text-lg font-medium text-gray-800">
				You have {game?.state.num_guesses_left}
				{#if game?.state.num_guesses_left === 1}guess{:else}guesses{/if} remaining
			</div>
			<div class="flex items-center">
				<button onclick={endTurn} class="text-red-600 hover:underline">End Turn</button>
				{#if votesToEnd.length > 0}
					<div class="ml-2 flex justify-center gap-1">
						{#each votesToEnd as vote (vote.playerId.id)}
							<div class="group relative">
								<div
									class="h-2 w-2 rounded-full transition-all duration-200 {vote.confirmed
										? 'bg-yellow-500'
										: 'border border-yellow-500 bg-transparent'}"
								></div>
								<div
									class="pointer-events-none absolute bottom-full left-1/2 z-10 mb-1 -translate-x-1/2 rounded bg-gray-900 px-2 py-1 text-xs whitespace-nowrap text-white opacity-0 transition-opacity group-hover:opacity-100"
								>
									{vote.playerName} votes to end the turn
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
			<div class="text-sm text-gray-500">Tap a card to guess.</div>
		</div>
	{/if}
</div>
