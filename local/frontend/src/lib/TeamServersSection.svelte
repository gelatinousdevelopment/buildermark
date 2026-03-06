<script lang="ts">
	import { onMount } from 'svelte';
	import { listTeamServers, createTeamServer, updateTeamServer, deleteTeamServer } from '$lib/api';
	import type { TeamServer } from '$lib/types';
	import Dialog from '$lib/Dialog.svelte';

	let teamServersEnabled = false;
	let teamServers: TeamServer[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	let showFormDialog = $state(false);
	let editingServer: TeamServer | null = $state(null);
	let formLabel = $state('');
	let formUrl = $state('');
	let formApiKey = $state('');
	let saving = $state(false);
	let formError: string | null = $state(null);

	let showDeleteDialog = $state(false);
	let deletingServer: TeamServer | null = $state(null);
	let deletingLoading = $state(false);
	let deleteError: string | null = $state(null);

	onMount(() => {
		load();
	});

	async function load() {
		loading = true;
		error = null;
		try {
			teamServers = await listTeamServers();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load team servers';
		} finally {
			loading = false;
		}
	}

	function openAdd() {
		editingServer = null;
		formLabel = '';
		formUrl = '';
		formApiKey = '';
		formError = null;
		showFormDialog = true;
	}

	function openEdit(server: TeamServer) {
		editingServer = server;
		formLabel = server.label;
		formUrl = server.url;
		formApiKey = server.apiKey;
		formError = null;
		showFormDialog = true;
	}

	function closeFormDialog() {
		showFormDialog = false;
		editingServer = null;
	}

	async function save() {
		if (saving) return;
		saving = true;
		formError = null;
		try {
			if (editingServer) {
				await updateTeamServer(editingServer.id, formLabel, formUrl, formApiKey);
			} else {
				await createTeamServer(formLabel, formUrl, formApiKey);
			}
			closeFormDialog();
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Failed to save team server';
		} finally {
			saving = false;
		}
	}

	function openDelete(server: TeamServer) {
		deletingServer = server;
		deleteError = null;
		showDeleteDialog = true;
	}

	function closeDeleteDialog() {
		showDeleteDialog = false;
		deletingServer = null;
	}

	async function confirmDelete() {
		if (!deletingServer || deletingLoading) return;
		deletingLoading = true;
		deleteError = null;
		try {
			await deleteTeamServer(deletingServer.id);
			closeDeleteDialog();
			await load();
		} catch (e) {
			deleteError = e instanceof Error ? e.message : 'Failed to delete team server';
		} finally {
			deletingLoading = false;
		}
	}
</script>

<div class="team-servers-section">
	<h2>Team Servers</h2>
	{#if loading}
		<p>Loading team servers...</p>
	{:else if error}
		<p class="error">{error}</p>
	{:else}
		{#if teamServers.length > 0}
			<table class="data bordered striped hoverable" style:max-width="50rem">
				<thead>
					<tr>
						<th>Label</th>
						<th>URL</th>
						<th>API Key</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					{#each teamServers as server (server.id)}
						<tr>
							<td>{server.label}</td>
							<td class="path">{server.url}</td>
							<td class="api-key-cell">{server.apiKey ? '***' : ''}</td>
							<td class="actions-cell">
								<button class="bordered small" onclick={() => openEdit(server)}>Edit</button>
								<button class="bordered small danger-text" onclick={() => openDelete(server)}
									>Delete</button
								>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{:else}
			<p class="muted">No team servers configured.</p>
		{/if}
		<button class="bordered small add-btn" onclick={openAdd} disabled={!teamServersEnabled}
			>{teamServersEnabled ? 'Add Team Server' : 'Coming Soon'}</button
		>
	{/if}
</div>

<Dialog
	open={showFormDialog}
	title={editingServer ? 'Edit Team Server' : 'Add Team Server'}
	onclose={closeFormDialog}
>
	<label for="ts-label">Label</label>
	<input id="ts-label" type="text" bind:value={formLabel} placeholder="My Team Server" />
	<label for="ts-url">URL</label>
	<input id="ts-url" type="text" bind:value={formUrl} placeholder="https://example.com" />
	<label for="ts-api-key">API Key</label>
	<input id="ts-api-key" type="text" bind:value={formApiKey} placeholder="Optional" />
	{#if formError}
		<p class="error">{formError}</p>
	{/if}
	{#snippet actions()}
		<button class="bordered small" onclick={closeFormDialog}>Cancel</button>
		<button
			class="bordered small prominent"
			disabled={!formLabel || !formUrl || saving}
			onclick={save}
		>
			{saving ? 'Saving...' : editingServer ? 'Update' : 'Add'}
		</button>
	{/snippet}
</Dialog>

<Dialog
	open={showDeleteDialog && !!deletingServer}
	title="Delete Team Server"
	onclose={closeDeleteDialog}
>
	{#if deletingServer}
		<p>
			Delete <strong>{deletingServer.label}</strong>? Any projects using this server will be
			unlinked.
		</p>
	{/if}
	{#if deleteError}
		<p class="error">{deleteError}</p>
	{/if}
	{#snippet actions()}
		<button class="bordered small" onclick={closeDeleteDialog}>Cancel</button>
		<button class="btn-danger" disabled={deletingLoading} onclick={confirmDelete}>
			{deletingLoading ? 'Deleting...' : 'Delete'}
		</button>
	{/snippet}
</Dialog>

<style>
	.team-servers-section {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	h2 {
		color: var(--accent-color-darkest);
		margin: 0;
		font-size: 1rem;
	}

	.team-servers-section p {
		margin: 0.5rem 0;
	}

	.path {
		font-family:
			ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New',
			monospace;
		word-break: break-all;
	}

	.muted {
		color: var(--color-text-faded);
		font-size: 0.85rem;
		margin: 0;
	}

	.actions-cell {
		white-space: nowrap;
		display: flex;
		gap: 0.3rem;
	}

	.api-key-cell {
		color: var(--color-text-faded);
	}

	.danger-text {
		color: var(--color-danger);
	}

	.add-btn {
		align-self: flex-start;
	}

	.btn-danger {
		padding: 0.35rem 0.8rem;
		font-size: 0.9rem;
		font-weight: bold;
		line-height: 1.4;
		border: 1px solid var(--color-danger);
		border-radius: 3px;
		background: var(--color-danger);
		color: #fff;
		cursor: pointer;
	}

	.btn-danger:hover {
		background: var(--color-danger-hover);
		border-color: var(--color-danger-hover);
	}

	.btn-danger:disabled {
		opacity: 0.5;
		cursor: default;
	}
</style>
