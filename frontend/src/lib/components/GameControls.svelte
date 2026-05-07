<script lang="ts">
	import { onDestroy } from 'svelte';
	import { gameStore } from '$lib/game.svelte';
	import { Api } from '$lib/api';
	import type { PlayerVote } from '$lib/types';

	const { game, myPlayer, isMyTurn } = $derived(gameStore);
	const api = new Api();

	let clueWord = $state('');
	let clueCount = $state(1);
	let sending = $state(false);

	// Countdown timer for turing clue phase
	let now = $state(Date.now());
	const ticker = setInterval(() => { now = Date.now(); }, 500);
	onDestroy(() => clearInterval(ticker));

	const isTuring = $derived(game?.state.game_mode === 'TURING');
	const turingPhase = $derived(game?.state.turing_phase);

	// Track clue submission start time for countdown (60s from game start in clue phase)
	const clueDeadlineMs = $derived.by(() => {
		if (!isTuring || turingPhase !== 'CLUE') return null;
		if (!gameStore.gameStartTime) return null;
		return gameStore.gameStartTime + 60_000;
	});

	const clueSecondsLeft = $derived.by(() => {
		if (!clueDeadlineMs) return 0;
		return Math.max(0, Math.ceil((clueDeadlineMs - now) / 1000));
	});

	async function submitClue() {
		if (!game || !clueWord) return;
		sending = true;
		try {
			await api.sendClue(game.id, clueWord, clueCount);
			clueWord = '';
			clueCount = 1;
			gameStore.turingClueSubmitted = true;
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

	async function submitTuringVote(team: string) {
		if (!game) return;
		sending = true;
		try {
			await api.sendTuringVote(game.id, team);
			gameStore.turingVoteSubmitted = true;
		} catch (e) {
			alert('Failed to submit vote: ' + e);
		} finally {
			sending = false;
		}
	}

	async function revealResult() {
		if (!game) return;
		await api.revealTuringResult(game.id);
	}

	const isCreator = $derived(game && gameStore.user && game.created_by === gameStore.user.id);

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

	// Which clue is currently active in turing mode
	const activeTuringClue = $derived.by(() => {
		if (!isTuring || !game) return null;
		const phase = game.state.turing_phase;
		const clues = game.state.clues;
		if (phase === 'GUESS_RED') {
			return clues.find((c) => c.team === 'RED') ?? null;
		}
		if (phase === 'GUESS_BLUE') {
			return clues.find((c) => c.team === 'BLUE') ?? null;
		}
		return null;
	});
</script>

<div class="rounded-lg bg-white p-4 shadow-md">
	{#if isTuring}
		{#if turingPhase === 'CLUE'}
			<!-- Turing clue submission phase -->
			<div class="mb-2 flex items-center justify-between">
				<span class="text-sm font-semibold text-purple-700">Clue Phase — {clueSecondsLeft}s remaining</span>
				<div class="h-2 w-32 rounded-full bg-gray-200">
					<div
						class="h-2 rounded-full bg-purple-500 transition-all"
						style="width: {Math.min(100, (clueSecondsLeft / 60) * 100)}%"
					></div>
				</div>
			</div>
			{#if myPlayer?.role === 'SPYMASTER' && isMyTurn}
				<form
					class="flex flex-col gap-4 sm:flex-row sm:items-end"
					onsubmit={(e) => { e.preventDefault(); submitClue(); }}
				>
					<div class="flex-1">
						<label class="mb-1 block text-sm font-medium text-gray-700" for="clue-word">Your Clue</label>
						<input
							id="clue-word"
							type="text"
							bind:value={clueWord}
							class="w-full rounded border border-gray-300 p-2 uppercase focus:border-purple-500 focus:ring-1 focus:ring-purple-500 focus:outline-none"
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
							class="w-full rounded border border-gray-300 p-2 focus:border-purple-500 focus:ring-1 focus:ring-purple-500 focus:outline-none"
							required
						/>
					</div>
					<button
						type="submit"
						disabled={sending}
						class="rounded bg-purple-600 px-6 py-2 font-bold text-white hover:bg-purple-700 disabled:opacity-50"
					>
						Submit Clue
					</button>
				</form>
			{:else if myPlayer?.role === 'SPYMASTER'}
				<div class="text-center text-green-700 font-semibold">✓ Clue submitted — waiting for the other spymaster...</div>
			{:else}
				<div class="text-center text-gray-500">Both spymasters are writing their clues. Reveal in {clueSecondsLeft}s...</div>
			{/if}

		{:else if turingPhase === 'GUESS_RED' || turingPhase === 'GUESS_BLUE'}
			<!-- Turing guess phase -->
			{#if !isMyTurn}
				<div class="text-center text-gray-500">
					Guessing for <span class:text-red-700={turingPhase === 'GUESS_RED'} class:text-blue-700={turingPhase === 'GUESS_BLUE'} class="font-bold">
						{turingPhase === 'GUESS_RED' ? 'RED' : 'BLUE'} clue
					</span>...
				</div>
			{:else}
				<div class="flex items-center justify-between">
					<div class="text-sm font-medium text-gray-700">
						Guessing for
						<span class:text-red-700={turingPhase === 'GUESS_RED'} class:text-blue-700={turingPhase === 'GUESS_BLUE'} class="font-bold">
							{activeTuringClue ? `"${activeTuringClue.clue.word}" (${activeTuringClue.clue.count})` : (turingPhase === 'GUESS_RED' ? 'RED' : 'BLUE')}
						</span>
						— {game?.state.num_guesses_left} {game?.state.num_guesses_left === 1 ? 'guess' : 'guesses'} left
					</div>
					<div class="flex items-center">
						<button onclick={endTurn} class="text-red-600 hover:underline text-sm">End Turn</button>
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

		{:else if turingPhase === 'VOTE'}
			<!-- Turing vote phase -->
			<div class="text-center">
				<p class="mb-3 font-semibold text-gray-800">Which spymaster was the AI?</p>
				{#if isMyTurn}
					<div class="flex justify-center gap-4">
						<button
							onclick={() => submitTuringVote('RED')}
							disabled={sending}
							class="rounded-lg bg-red-600 px-6 py-2 font-bold text-white hover:bg-red-700 disabled:opacity-50"
						>
							RED was AI
						</button>
						<button
							onclick={() => submitTuringVote('BLUE')}
							disabled={sending}
							class="rounded-lg bg-blue-600 px-6 py-2 font-bold text-white hover:bg-blue-700 disabled:opacity-50"
						>
							BLUE was AI
						</button>
					</div>
				{:else if myPlayer?.role === 'OPERATIVE'}
					<p class="text-green-700 font-semibold">✓ Vote submitted</p>
				{/if}

				<!-- Show votes so far -->
				{#if gameStore.turingVotes.size > 0}
					<div class="mt-3 flex justify-center gap-4 text-sm text-gray-600">
						<span>RED: {[...gameStore.turingVotes.values()].filter(v => v.suspectedAITeam === 'RED').length}</span>
						<span>BLUE: {[...gameStore.turingVotes.values()].filter(v => v.suspectedAITeam === 'BLUE').length}</span>
					</div>
				{/if}

				{#if isCreator}
					<button
						onclick={revealResult}
						class="mt-3 rounded bg-gray-800 px-5 py-1.5 text-sm font-bold text-white hover:bg-gray-900"
					>
						Reveal Result
					</button>
				{/if}
			</div>
		{/if}

	{:else}
		<!-- Standard mode controls -->
		{#if !isMyTurn}
			<div class="text-center text-gray-500">
				Waiting for {game?.state.active_team}
				{game?.state.active_role}...
			</div>
		{:else if myPlayer?.role === 'SPYMASTER'}
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
						class="w-full rounded border border-gray-300 p-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none uppercase"
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
	{/if}
</div>
