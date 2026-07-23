<script lang="ts">
	import { gameStore } from '$lib/game.svelte';

	const { history } = $derived(gameStore);

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

	function formatMs(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}

	function downloadCSV() {
		const rows = [
			['round', 'team', 'type', 'detail', 'result', 'duration_ms'],
			...history.map((e) => [e.round, e.team, e.type, e.detail, e.result, e.durationMs])
		];
		const csv = rows.map((r) => r.join(',')).join('\n');
		const a = document.createElement('a');
		a.href = URL.createObjectURL(new Blob([csv], { type: 'text/csv' }));
		a.download = 'game-log.csv';
		a.click();
	}
</script>

{#if history.length > 0}
	<div class="mt-6 overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
		<div class="flex items-center justify-between border-b border-gray-100 bg-gray-50 px-4 py-3">
			<h3 class="text-sm font-semibold text-gray-700">Game Log</h3>
			<button
				onclick={downloadCSV}
				class="rounded bg-gray-200 px-2 py-1 text-xs font-semibold text-gray-700 hover:bg-gray-300"
			>
				Download CSV
			</button>
		</div>
		<div class="overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr
						class="border-b border-gray-100 text-left text-xs font-semibold tracking-wide text-gray-500 uppercase"
					>
						<th class="px-3 py-2">Round</th>
						<th class="px-3 py-2">Team</th>
						<th class="px-3 py-2">Type</th>
						<th class="px-3 py-2">Detail</th>
						<th class="px-3 py-2">Result</th>
						<th class="px-3 py-2">Time</th>
					</tr>
				</thead>
				<tbody>
					{#each history as entry, i (i)}
						<tr
							class="border-b border-gray-50 transition-colors hover:bg-gray-50"
							class:bg-red-50={entry.team === 'RED'}
							class:bg-blue-50={entry.team === 'BLUE'}
						>
							<td class="px-3 py-2 font-mono text-gray-500">{entry.round}</td>
							<td class="px-3 py-2">
								<span
									class="font-bold"
									class:text-red-600={entry.team === 'RED'}
									class:text-blue-600={entry.team === 'BLUE'}
								>
									{entry.team}
								</span>
							</td>
							<td class="px-3 py-2 text-gray-600 capitalize">{entry.type}</td>
							<td class="px-3 py-2 font-medium">{entry.detail}</td>
							<td class="px-3 py-2">
								{#if entry.result}
									<span
										class="rounded px-1.5 py-0.5 text-xs font-semibold capitalize {resultColor(
											entry.result
										)}"
									>
										{entry.result}
									</span>
								{/if}
							</td>
							<td class="px-3 py-2 font-mono text-xs text-gray-500">{formatMs(entry.durationMs)}</td
							>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
{/if}
