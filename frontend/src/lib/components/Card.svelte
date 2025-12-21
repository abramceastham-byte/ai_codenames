<script lang="ts">
	import type { Card } from '$lib/types';
	import { AGENT_RED, AGENT_BLUE, AGENT_ASSASSIN, AGENT_BYSTANDER } from '$lib/types';

	interface Props {
		card: Card;
		isSpymaster: boolean;
		isGameOver: boolean;
		onClick: () => void;
	}

	let { card, isSpymaster, onClick, isGameOver }: Props = $props();

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
	const revealedClasses = $derived(card.revealed ? '' : 'cursor-pointer hover:-translate-y-0.5 hover:shadow-md');
</script>

<button
	class="flex aspect-[4/3] w-full flex-col items-center justify-center rounded-lg border-2 sm:p-2 sm:shadow-sm transition-all duration-200 {classes} {revealedClasses}"
	onclick={onClick}
	disabled={card.revealed}
>
	{#if card.revealed}
		<!-- Maybe an icon for the type? -->
		<span class="mb-1 text-xs font-bold tracking-wider uppercase opacity-75 hidden sm:block">
			{#if card.agent === AGENT_RED}RED AGENT
			{:else if card.agent === AGENT_BLUE}BLUE AGENT
			{:else if card.agent === AGENT_ASSASSIN}ASSASSIN
			{:else if card.agent === AGENT_BYSTANDER}BYSTANDER
			{/if}
		</span>
	{/if}
	<span
		class="w-full text-center leading-tight sm:font-bold break-words uppercase text-xs sm:text-lg lg:text-xl"
	>
		{card.codeword.replaceAll("_", " ")}
	</span>
</button>
