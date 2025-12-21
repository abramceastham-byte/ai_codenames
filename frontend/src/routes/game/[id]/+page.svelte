<script lang="ts">
	import { gameStore } from '$lib/game.svelte';
	import { onDestroy } from 'svelte';
	import SetupView from '$lib/components/SetupView.svelte';
	import BoardView from '$lib/components/BoardView.svelte';
	import { resolve } from '$app/paths';

	onDestroy(() => {
		if (gameStore.ws) {
			gameStore.ws.close();
		}
	});
</script>

<div class="min-h-screen bg-stone-100">
	{#if gameStore.error}
		<div class="p-8 text-center text-red-600">
			<h2 class="text-2xl font-bold">Error</h2>
			<p>{gameStore.error}</p>
			<a href={resolve('/')} class="mt-4 inline-block text-blue-600 hover:underline"
				>Back to Lobby</a
			>
		</div>
	{:else if !gameStore.game}
		<div class="flex h-screen items-center justify-center">
			<div class="text-xl text-gray-500">Loading game...</div>
		</div>
	{:else if gameStore.game.status === 'PENDING'}
		<SetupView />
	{:else}
		<BoardView />
	{/if}
</div>
