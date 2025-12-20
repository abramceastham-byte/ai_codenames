<script lang="ts">
	import { gameStore } from '$lib/game.svelte';
	import { api } from '$lib/api';
	import CardComponent from './Card.svelte';
	import GameControls from './GameControls.svelte';
	import GameInfo from './GameInfo.svelte';
    import type { Card } from '$lib/types';
    import { goto } from '$app/navigation';

	const { game, myPlayer, isMyTurn } = $derived(gameStore);
    
    // Check if spymaster view
    const isSpymaster = $derived(myPlayer?.role === 'SPYMASTER');

	async function handleCardClick(card: Card) {
        if (!game || !isMyTurn || myPlayer?.role !== 'OPERATIVE' || card.revealed) return;
        
		try {
			await api.sendGuess(game.id, card.codeword);
		} catch (e) {
			alert('Failed to submit guess: ' + e);
		}
	}
    
    const isGameOver = $derived(game?.status === 'FINISHED');
</script>

<div class="mx-auto max-w-6xl p-4">
    <div class="mb-4 flex justify-between items-center">
        <button onclick={() => goto('/lobby')} class="text-gray-500 hover:text-gray-800 font-medium">
            &larr; Lobby
        </button>
        <div class="text-gray-400 text-sm">Game ID: {game?.id}</div>
    </div>

	<GameInfo />

	<div class="mb-8 grid grid-cols-5 gap-2 md:gap-4">
        {#if game?.state.board.cards}
            {#each game.state.board.cards as card}
                <CardComponent 
                    {card} 
                    {isSpymaster} 
                    onClick={() => handleCardClick(card)} 
                />
            {/each}
        {/if}
	</div>

	<div class="sticky bottom-4 mx-auto max-w-2xl">
        {#if isGameOver}
             <div class="rounded-lg bg-stone-800 p-6 text-center text-white shadow-xl">
                <h2 class="mb-2 text-3xl font-bold">Game Over!</h2>
                 <!-- We assume winning team is handled elsewhere or inferable, but for now simple message -->
                <button onclick={() => goto('/lobby')} class="mt-4 rounded bg-white px-6 py-2 font-bold text-stone-900 hover:bg-gray-200">
                    Back to Lobby
                </button>
            </div>
        {:else}
		    <GameControls />
        {/if}
	</div>
</div>
