<script lang="ts">
	import { page } from '$app/state';
	import { Api, type AdminGameLog } from '$lib/api';

	// This page is intentionally not linked from anywhere in the normal
	// player-facing UI (SetupView/BoardView) — it exists only for a
	// developer/admin to review AI reasoning and player types after a game,
	// gated behind the ADMIN_SECRET the server was started with.
	const gameId = page.params.gameId ?? '';

	let adminSecret = $state('');
	let log = $state<AdminGameLog | null>(null);
	let error = $state('');
	let loading = $state(false);

	async function load() {
		loading = true;
		error = '';
		try {
			log = await new Api().getAdminGameLog(gameId, adminSecret);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load admin log';
			log = null;
		} finally {
			loading = false;
		}
	}
</script>

<div class="mx-auto max-w-4xl p-6">
	<h1 class="mb-4 text-xl font-bold text-gray-800">Admin game log — {gameId}</h1>

	<form
		class="mb-6 flex items-center gap-2"
		onsubmit={(e) => {
			e.preventDefault();
			load();
		}}
	>
		<input
			type="password"
			placeholder="Admin secret"
			bind:value={adminSecret}
			class="rounded border border-gray-300 px-2 py-1 text-sm"
		/>
		<button
			type="submit"
			disabled={loading}
			class="rounded bg-gray-800 px-3 py-1 text-sm font-semibold text-white disabled:opacity-50"
		>
			{loading ? 'Loading...' : 'Load'}
		</button>
	</form>

	{#if error}
		<p class="mb-4 text-sm text-red-600">{error}</p>
	{/if}

	{#if log}
		<h2 class="mb-2 text-sm font-semibold text-gray-700">Players</h2>
		<div class="mb-6 overflow-x-auto rounded border border-gray-200">
			<table class="w-full text-sm">
				<thead>
					<tr
						class="border-b border-gray-100 bg-gray-50 text-left text-xs font-semibold text-gray-500 uppercase"
					>
						<th class="px-3 py-2">Name</th>
						<th class="px-3 py-2">Type</th>
						<th class="px-3 py-2">Team</th>
						<th class="px-3 py-2">Role</th>
						<th class="px-3 py-2">Backend</th>
					</tr>
				</thead>
				<tbody>
					{#each log.players as p, i (i)}
						<tr class="border-b border-gray-50">
							<td class="px-3 py-2 font-medium">{p.name}</td>
							<td class="px-3 py-2">{p.player_id.player_type}</td>
							<td class="px-3 py-2">{p.team}</td>
							<td class="px-3 py-2">{p.role}</td>
							<td class="px-3 py-2">{p.backend ?? ''}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<h2 class="mb-2 text-sm font-semibold text-gray-700">Reasoning</h2>
		<div class="overflow-x-auto rounded border border-gray-200">
			<table class="w-full text-sm">
				<thead>
					<tr
						class="border-b border-gray-100 bg-gray-50 text-left text-xs font-semibold text-gray-500 uppercase"
					>
						<th class="px-3 py-2">Round</th>
						<th class="px-3 py-2">Team</th>
						<th class="px-3 py-2">Role</th>
						<th class="px-3 py-2">Backend</th>
						<th class="px-3 py-2">Action</th>
						<th class="px-3 py-2">Detail</th>
						<th class="px-3 py-2">Reasoning</th>
					</tr>
				</thead>
				<tbody>
					{#each log.reasoning as e, i (i)}
						<tr class="border-b border-gray-50">
							<td class="px-3 py-2">{e.round}</td>
							<td class="px-3 py-2">{e.team}</td>
							<td class="px-3 py-2">{e.role}</td>
							<td class="px-3 py-2">{e.backend}</td>
							<td class="px-3 py-2 capitalize">{e.action}</td>
							<td class="px-3 py-2 font-medium">{e.detail}</td>
							<td class="px-3 py-2 text-gray-600">{e.reasoning}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
