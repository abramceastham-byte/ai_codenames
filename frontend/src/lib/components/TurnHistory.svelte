<script lang="ts">
	import { gameStore } from '$lib/game.svelte';
	import type { LogEntry } from '$lib/types';

	const { history } = $derived(gameStore);

	const rounds = $derived.by(() => {
		const byRound = new Map<number, LogEntry[]>();
		for (const entry of history) {
			const list = byRound.get(entry.round) ?? [];
			list.push(entry);
			byRound.set(entry.round, list);
		}
		return [...byRound.entries()].sort((a, b) => a[0] - b[0]);
	});

	function resultColor(result: string): string {
		switch (result) {
			case 'red':
				return 'bg-red-100 text-red-800';
			case 'blue':
				return 'bg-blue-100 text-blue-800';
			case 'bystander':
				return 'bg-amber-100 text-amber-800';
			case 'assassin':
				return 'bg-gray-900 text-white';
			default:
				return '';
		}
	}
</script>

{#if rounds.length > 0}
	<details class="mb-6 overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
		<summary
			class="cursor-pointer bg-gray-50 px-4 py-3 text-sm font-semibold text-gray-700 select-none hover:bg-gray-100"
		>
			Turn History ({rounds.length} round{rounds.length === 1 ? '' : 's'})
		</summary>
		<div class="max-h-96 divide-y divide-gray-100 overflow-y-auto border-t border-gray-100">
			{#each rounds as [round, entries] (round)}
				<div class="px-4 py-3">
					<div class="mb-1 text-xs font-semibold tracking-wide text-gray-400 uppercase">
						Round {round}
					</div>
					<ul class="space-y-1">
						{#each entries as entry, i (i)}
							<li class="flex items-center gap-2 text-sm">
								<span
									class="font-bold"
									class:text-red-600={entry.team === 'RED'}
									class:text-blue-600={entry.team === 'BLUE'}
								>
									{entry.team}
								</span>
								<span class="text-gray-600 capitalize">{entry.type}:</span>
								<span class="font-medium">{entry.detail}</span>
								{#if entry.result}
									<span
										class="rounded px-1.5 py-0.5 text-xs font-semibold capitalize {resultColor(
											entry.result
										)}"
									>
										{entry.result}
									</span>
								{/if}
							</li>
						{/each}
					</ul>
				</div>
			{/each}
		</div>
	</details>
{/if}
