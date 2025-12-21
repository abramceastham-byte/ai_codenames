<script lang="ts">
	import { gameStore } from '$lib/game.svelte';
	import { AGENT_RED, AGENT_BLUE } from '$lib/types';

	const { game, lastClue } = $derived(gameStore);

	const scores = $derived.by(() => {
		if (!game) return { red: 0, blue: 0 };
		const cards = game.state.board.cards;
		// Count unrevealed agents
		let redLeft = 8;
		let blueLeft = 8;
		switch (game.state.starting_team) {
			case 'RED':
				redLeft++;
				break;
			case 'BLUE':
				blueLeft++;
				break;
		}

		redLeft -= cards.filter((c) => c.agent === AGENT_RED && c.revealed).length;
		blueLeft -= cards.filter((c) => c.agent === AGENT_BLUE && c.revealed).length;
		return { red: redLeft, blue: blueLeft };
	});

	// Determine background color based on active team
	const teamColor = $derived(
		game?.state.active_team === 'RED' ? 'bg-red-100 border-red-200' : 'bg-blue-100 border-blue-200'
	);
	const teamText = $derived(game?.state.active_team === 'RED' ? 'text-red-800' : 'text-blue-800');
</script>

<div class="mb-6 grid gap-4 md:grid-cols-3">
	<!-- Red Score -->
	<div class="rounded-lg border border-red-100 bg-red-50 p-4 text-center">
		<div class="text-sm font-bold text-red-600 uppercase">Red Agents Left</div>
		<div class="text-3xl font-bold text-red-800">{scores.red}</div>
	</div>

	<!-- Active Turn / Clue Info -->
	<div class="rounded-lg border-2 p-4 text-center {teamColor}">
		<div class="text-sm font-bold text-gray-500 uppercase">Current Turn</div>
		<div class="text-xl font-bold {teamText}">
			{game?.state.active_team}
			{game?.state.active_role}
		</div>

		{#if lastClue && lastClue.team === game?.state.active_team}
			<div class="mt-2 border-t border-gray-300/50 pt-2">
				<span class="text-sm text-gray-600">Current Clue:</span>
				<div class="font-mono text-lg font-bold">{lastClue.word} ({lastClue.count})</div>
			</div>
		{/if}
	</div>

	<!-- Blue Score -->
	<div class="rounded-lg border border-blue-100 bg-blue-50 p-4 text-center">
		<div class="text-sm font-bold text-blue-600 uppercase">Blue Agents Left</div>
		<div class="text-3xl font-bold text-blue-800">{scores.blue}</div>
	</div>
</div>
