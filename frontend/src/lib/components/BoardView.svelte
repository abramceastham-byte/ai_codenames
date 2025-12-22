<script lang="ts">
	import { gameStore } from '$lib/game.svelte';
	import { Api } from '$lib/api';
	import CardComponent from './Card.svelte';
	import GameControls from './GameControls.svelte';
	import GameInfo from './GameInfo.svelte';
	import type { Card, PlayerVote } from '$lib/types';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';

	const { game, myPlayer, isMyTurn } = $derived(gameStore);
	const api = new Api();

	// Check if spymaster view
	const isSpymaster = $derived(myPlayer?.role === 'SPYMASTER');

	// Track tentative guess
	let tentativeGuess = $state<string | null>(null);

	async function handleCardClick(card: Card) {
		if (!game || !isMyTurn || myPlayer?.role !== 'OPERATIVE' || card.revealed) return;

		try {
			// If clicking the same card as tentative, confirm it
			if (tentativeGuess === card.codeword) {
				await api.sendGuess(game.id, card.codeword, true);
				tentativeGuess = null;
			} else {
				// Otherwise, send as tentative guess
				await api.sendGuess(game.id, card.codeword, false);
				tentativeGuess = card.codeword;
			}
		} catch (e) {
			alert('Failed to submit guess: ' + e);
		}
	}

	const isGameOver = $derived(game?.status === 'FINISHED');

	const redWon = $derived(game?.state.winning_team === 'RED');
	const blueWon = $derived(game?.state.winning_team === 'BLUE');
	const perCardVotes = $derived.by(() => {
		const perCardV = new Map<string, PlayerVote[]>()
		gameStore.votes.forEach((pv) => {
			const votes = perCardV.get(pv.guess) ?? []
			votes.push(pv)
			perCardV.set(pv.guess, votes)
		})
		return perCardV
	})
</script>

<div class="mx-auto max-w-6xl p-4">
	<div class="mb-4 flex items-center justify-between">
		<button
			onclick={() => goto(resolve('/'))}
			class="font-medium text-gray-500 hover:text-gray-800"
		>
			&larr; Lobby
		</button>
		<div class="text-sm text-gray-400">Game ID: {game?.id}</div>
	</div>

	<GameInfo />

	<div class="mb-8 grid grid-cols-5 gap-1 sm:gap-4">
		{#if game?.state.board.cards}
			{#each game.state.board.cards as card (card.codeword)}
				<CardComponent
					{card}
					{isSpymaster}
					{isGameOver}
					isTentative={tentativeGuess === card.codeword}
					votes={perCardVotes.get(card.codeword) || []}
					onClick={() => handleCardClick(card)}
				/>
			{/each}
		{/if}
	</div>

	<div class="sticky bottom-4 mx-auto max-w-2xl">
		{#if isGameOver}
			<div class="rounded-lg bg-stone-800 p-6 text-center text-white shadow-xl">
				<h2 class="mb-2 text-3xl font-bold">
					Game over, <span class:text-red-600={redWon} class:text-blue-600={blueWon}>
						{game?.state.winning_team} wins!</span
					>
				</h2>
				<!-- We assume winning team is handled elsewhere or inferable, but for now simple message -->
				<button
					onclick={() => goto(resolve('/'))}
					class="mt-4 rounded bg-white px-6 py-2 font-bold text-stone-900 hover:bg-gray-200"
				>
					Back to Lobby
				</button>
			</div>
		{:else}
			<GameControls />
		{/if}
	</div>
</div>
