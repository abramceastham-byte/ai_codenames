<script lang="ts">
	// The game creator's switch for the clue hold, which withholds every
	// spymaster's clue until the clue phase reaches its configured length so
	// response speed can't reveal who is an AI.
	//
	// Rendered only for the creator, and the state is read from a creator-only
	// endpoint: an operative who knew the hold was off would know that a clue
	// arriving in three seconds came from a bot.
	import { gameStore } from '$lib/game.svelte';
	import { Api } from '$lib/api';

	const { game, user } = $derived(gameStore);
	const api = new Api();

	let enabled = $state<boolean | null>(null);
	let delayMs = $state(0);
	let busy = $state(false);
	let error = $state<string | null>(null);

	const isCreator = $derived(!!game && !!user && game.created_by === user.id);
	// A server started with --clue_delay=0 has no duration to hold for, so
	// there's nothing to switch.
	const available = $derived(enabled !== null && delayMs > 0);
	const seconds = $derived(Math.round(delayMs / 1000));

	$effect(() => {
		if (!isCreator || !game) return;
		api
			.getClueHold(game.id)
			.then((h) => {
				enabled = h.enabled;
				delayMs = h.delay_ms;
			})
			.catch((e) => {
				error = String(e);
			});
	});

	async function toggle() {
		if (!game || busy) return;
		busy = true;
		error = null;
		try {
			const h = await api.setClueHold(game.id, !enabled);
			enabled = h.enabled;
			delayMs = h.delay_ms;
		} catch (e) {
			error = String(e);
		} finally {
			busy = false;
		}
	}
</script>

{#if isCreator && available}
	<div
		class="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-indigo-200 bg-indigo-50 p-3"
	>
		<div>
			<div class="text-sm font-bold text-indigo-900 uppercase">Clue hold — creator only</div>
			<div class="text-sm text-indigo-700">
				{#if enabled}
					Clues are withheld until {seconds}s into each clue phase, so a fast answer doesn't give
					the spymaster away.
				{:else}
					Clues go out the moment they're given. Response times are visible to everyone.
				{/if}
			</div>
			{#if error}
				<div class="mt-1 text-sm text-red-600">{error}</div>
			{/if}
		</div>
		<button
			onclick={toggle}
			disabled={busy}
			aria-pressed={enabled}
			class="rounded px-4 py-2 font-bold text-white disabled:opacity-50 {enabled
				? 'bg-indigo-600 hover:bg-indigo-700'
				: 'bg-gray-500 hover:bg-gray-600'}"
		>
			{#if busy}
				Saving…
			{:else if enabled}
				Hold on ({seconds}s)
			{:else}
				Hold off
			{/if}
		</button>
	</div>
{/if}
