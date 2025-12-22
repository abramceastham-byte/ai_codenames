<script lang="ts">
	import type { Card, PlayerVote } from '$lib/types';
	import { AGENT_RED, AGENT_BLUE, AGENT_ASSASSIN, AGENT_BYSTANDER } from '$lib/types';

	interface Props {
		card: Card;
		isSpymaster: boolean;
		isGameOver: boolean;
		isTentative?: boolean;
		votes?: PlayerVote[];
		onClick: () => void;
	}

	let { card, isSpymaster, onClick, isGameOver, isTentative = false, votes = [] }: Props = $props();

	$effect(() => {
		if (votes.length > 0) {
			console.log('VOTES', votes);
		}
	});

	// Determine visual style based on state
	// If revealed: Show actual agent color
	// If spymaster: Show actual agent color (maybe slightly muted/tinted)
	// If operative & not revealed: Show neutral/unknown style

	function getBgColor() {
		if (card.revealed) {
			switch (card.agent) {
				case AGENT_RED:
					return 'bg-red-500 text-white border-red-700';
				case AGENT_BLUE:
					return 'bg-blue-500 text-white border-blue-700';
				case AGENT_ASSASSIN:
					return 'bg-stone-900 text-white border-black';
				case AGENT_BYSTANDER:
					return 'bg-amber-100 text-amber-900 border-amber-200';
				default:
					return 'bg-white';
			}
		}

		if (isSpymaster || isGameOver) {
			switch (card.agent) {
				case AGENT_RED:
					return 'bg-red-100 text-red-900 border-red-200';
				case AGENT_BLUE:
					return 'bg-blue-100 text-blue-900 border-blue-200';
				case AGENT_ASSASSIN:
					return 'bg-stone-300 text-stone-900 border-stone-400';
				case AGENT_BYSTANDER:
					return 'bg-amber-50 text-amber-900 border-amber-100';
				default:
					return 'bg-white';
			}
		}

		return 'bg-white hover:bg-gray-50 text-gray-800 border-gray-200';
	}

	const classes = $derived(getBgColor());
	const revealedClasses = $derived(
		card.revealed ? '' : 'cursor-pointer hover:-translate-y-0.5 hover:shadow-md'
	);
	const tentativeClasses = $derived(
		isTentative && !card.revealed ? 'ring-4 ring-yellow-400 -translate-y-1 shadow-lg' : ''
	);
</script>

<div class="relative w-full">
	<button
		class="flex aspect-[4/3] w-full flex-col items-center justify-center rounded-lg border-2 transition-all duration-200 sm:p-2 sm:shadow-sm {classes} {revealedClasses} {tentativeClasses}"
		onclick={onClick}
		disabled={card.revealed}
	>
		{#if isTentative && !card.revealed}
			<span class="mb-1 hidden text-xs font-semibold text-yellow-600 sm:block">
				TAP TO CONFIRM
			</span>
		{/if}
		{#if card.revealed}
			<!-- Maybe an icon for the type? -->
			<span class="mb-1 hidden text-xs font-bold tracking-wider uppercase opacity-75 sm:block">
				{#if card.agent === AGENT_RED}RED AGENT
				{:else if card.agent === AGENT_BLUE}BLUE AGENT
				{:else if card.agent === AGENT_ASSASSIN}ASSASSIN
				{:else if card.agent === AGENT_BYSTANDER}BYSTANDER
				{/if}
			</span>
		{/if}
		<span
			class="w-full text-center text-xs leading-tight break-words uppercase sm:text-lg sm:font-bold lg:text-xl"
		>
			{card.codeword.replaceAll('_', ' ')}
		</span>
	</button>

	{#if votes.length > 0 && !card.revealed}
		<div class="absolute right-0 bottom-1 left-0 flex justify-center gap-1">
			{#each votes as vote (vote.playerId.id)}
				<div class="group relative">
					<div
						class="h-2 w-2 rounded-full transition-all duration-200 {vote.confirmed
							? 'bg-yellow-500'
							: 'border border-yellow-500 bg-transparent'}"
					></div>
					<div
						class="pointer-events-none absolute bottom-full left-1/2 z-10 mb-1 -translate-x-1/2 rounded bg-gray-900 px-2 py-1 text-xs whitespace-nowrap text-white opacity-0 transition-opacity group-hover:opacity-100"
					>
						{vote.playerName}
						{#if !vote.confirmed}tentatively
						{/if} votes for {card.codeword}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
