<script lang="ts">
	import { gameStore } from '$lib/game.svelte';

	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let loading = $state(false);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		loading = true;
		try {
			await gameStore.login();
		} finally {
			loading = false;
		}
	}
</script>

<div class="flex min-h-screen items-center justify-center bg-stone-100">
	<div class="w-full max-w-md rounded-lg bg-white p-8 shadow-md">
		<h1 class="mb-6 text-center text-3xl font-bold text-gray-800">Codenames</h1>

		<form onsubmit={handleSubmit} class="space-y-4">
			<p class="text-center text-sm text-gray-600">
				You'll be assigned a random agent name when you join.
			</p>

			<button
				type="submit"
				disabled={loading}
				class="w-full rounded-md bg-blue-600 px-4 py-2 text-white hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 focus:outline-none disabled:opacity-50"
			>
				{#if loading}
					Joining…
				{:else if data.hasRedirectToGame}
					Join Game
				{:else}
					Enter Lobby
				{/if}
			</button>
		</form>
	</div>
</div>
