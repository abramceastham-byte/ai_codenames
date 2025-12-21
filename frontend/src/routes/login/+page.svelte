<script lang="ts">
	import { gameStore } from '$lib/game.svelte';

	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let name = $state('');

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (name.trim()) {
			await gameStore.login(name.trim());
		}
	}
</script>

<div class="flex min-h-screen items-center justify-center bg-stone-100">
	<div class="w-full max-w-md rounded-lg bg-white p-8 shadow-md">
		<h1 class="mb-6 text-center text-3xl font-bold text-gray-800">Codenames</h1>

		<form onsubmit={handleSubmit} class="space-y-4">
			<div>
				<label for="name" class="block text-sm font-medium text-gray-700">Enter your name</label>
				<input
					type="text"
					id="name"
					bind:value={name}
					class="mt-1 block w-full rounded-md border border-gray-300 p-2 shadow-sm focus:border-blue-500 focus:ring-blue-500 focus:outline-none"
					placeholder="Agent Name"
					required
				/>
			</div>

			<button
				type="submit"
				class="w-full rounded-md bg-blue-600 px-4 py-2 text-white hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 focus:outline-none"
			>
				{#if data.hasRedirect}
					Join Game
				{:else}
					Enter Lobby
				{/if}
			</button>
		</form>
	</div>
</div>
