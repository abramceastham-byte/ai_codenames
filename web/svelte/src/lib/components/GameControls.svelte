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
        // In Codenames, ending turn is basically guessing 0 cards or passing.
        // The API might not have an explicit "end turn" endpoint?
        // Checking API... no, it's driven by guesses.
        // Usually clicking "End Turn" means stopping guessing.
        // Wait, `serveGuess` takes `confirmed`. Maybe that's related?
        // Actually, usually you switch turns by failing a guess or explicit pass.
        // Codenames rules allow passing.
        // Does the API support it?
        // `codenames/game.go` `ActionPass`?
        // I didn't see it in `web.go` handlers.
        // Let's assume for now we just guess until failure or maybe there's a "pass" guess?
        // I'll leave "End Turn" out for now if not supported, or implement later.
        // Wait, looking at `web.go` `serveGuess`:
        // It validates `req.Guess` corresponds to a card.
        // So I can't guess "pass".
        // Let me re-read `web.go`.
        
        // Actually, `game.Move` has `ActionPass`.
        // But `serveGuess` calls `Move` with `ActionGuess`.
        // There is no `servePass` handler in `web.go`.
        // That seems like a missing feature in the backend?
        // Or maybe I missed it.
        // I'll check `web.go` handlers again.
    }
    
    // Checked web.go: Handlers are:
    // serveCreateUser, serveCreateAI, serveUpdateUser, serveUser, serveCreateGame, servePendingGames,
    // serveGame, serveGamePlayers, serveRequestAI, serveJoinGame, serveAssignRole, serveStartGame,
    // serveClue, serveGuess, serveData.
    
    // Yeah, no explicit pass. That's fine, we'll stick to Spymaster Clue for now.
</script>

<div class="rounded-lg bg-white p-4 shadow-md">
	{#if !isMyTurn}
		<div class="text-center text-gray-500">
			Waiting for {game?.state.active_team} {game?.state.active_role}...
		</div>
	{:else}
		{#if myPlayer?.role === 'SPYMASTER'}
			<form class="flex items-end gap-4" onsubmit={(e) => { e.preventDefault(); submitClue(); }}>
				<div class="flex-1">
					<label class="mb-1 block text-sm font-medium text-gray-700" for="clue-word">Clue Word</label>
					<input
						id="clue-word"
						type="text"
						bind:value={clueWord}
						class="w-full rounded border border-gray-300 p-2 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
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
						class="w-full rounded border border-gray-300 p-2 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
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
                <!-- Pass button commented out until backend support confirmed -->
				<!-- <button onclick={endTurn} class="text-red-600 hover:underline">End Turn</button> -->
                <div class="text-sm text-gray-500">Tap a card to guess.</div>
			</div>
		{/if}
	{/if}
</div>
