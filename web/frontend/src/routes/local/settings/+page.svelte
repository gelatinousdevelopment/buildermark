<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { getLocalSettings, listProjects, scanHistory } from '$lib/api';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import type { LocalSettings, Project } from '$lib/types';

	let projects: Project[] = $state([]);
	let loadingProjects = $state(true);
	let projectError: string | null = $state(null);

	let localSettingsLoading = $state(true);
	let localSettingsError: string | null = $state(null);
	let localSettings: LocalSettings | null = $state(null);
	let historyImportDays = $state('7');
	let importingHistory = $state(false);
	let historyImportError: string | null = $state(null);
	let historyImportResult: string | null = $state(null);

	const historyImportDayOptions = ['7', '14', '30', '60', '90', '180', '365', 'all'];

	let importStatusMessage = $derived(
		websocketStore.importStatus?.state === 'running' ? websocketStore.importStatus.message : null
	);

	function projectName(project: { label: string; path: string }): string {
		return project.label || project.path;
	}

	onMount(async () => {
		await Promise.all([loadProjects(), loadLocalSettings()]);
	});

	async function loadProjects() {
		loadingProjects = true;
		projectError = null;
		try {
			projects = (await listProjects(false)).sort((a, b) =>
				projectName(a).localeCompare(projectName(b))
			);
		} catch (e) {
			projectError = e instanceof Error ? e.message : 'Failed to load projects';
		} finally {
			loadingProjects = false;
		}
	}

	async function loadLocalSettings() {
		try {
			localSettings = await getLocalSettings();
		} catch (e) {
			localSettingsError = e instanceof Error ? e.message : 'Failed to load local settings';
		} finally {
			localSettingsLoading = false;
		}
	}

	function historyImportTimeframe(days: string): string {
		if (days === 'all') {
			return '876000h';
		}
		return `${Number(days) * 24}h`;
	}

	function historyOptionLabel(days: string): string {
		if (days === 'all') {
			return 'All';
		}
		return `${days} days`;
	}

	async function importHistory() {
		if (importingHistory) return;
		importingHistory = true;
		historyImportError = null;
		historyImportResult = null;
		websocketStore.clearImportStatus();
		try {
			await scanHistory(historyImportTimeframe(historyImportDays));
			const result = await websocketStore.waitForImportComplete();
			if (result.state === 'error') {
				historyImportError = result.message;
				return;
			}
			historyImportResult = `Imported ${result.entriesProcessed.toLocaleString()} entries.`;
		} catch (e) {
			historyImportError = e instanceof Error ? e.message : 'Failed to import history';
		} finally {
			importingHistory = false;
			websocketStore.clearImportStatus();
		}
	}
</script>

<div class="settings limited-content-width inset-when-limited-content-width">
	<h1>Global Settings</h1>

	<div class="tracking-section">
		<h2>Tracked Projects</h2>
		{#if loadingProjects}
			<p>Loading projects...</p>
		{:else if projectError}
			<p class="error">{projectError}</p>
		{:else if projects.length === 0}
			<p class="muted">No tracked projects yet.</p>
		{:else}
			<ul class="project-list">
				{#each projects as project (project.id)}
					<li>
						<a
							href={resolve('/local/projects/[project_id]/settings', {
								project_id: project.id
							})}
						>
							<span class="project-name">{projectName(project)}</span>
							<span class="project-path">{project.path}</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	<div class="history-import">
		<h2>Import Conversation History</h2>
		<p class="muted">This may take a while.</p>
		<div class="history-import-controls">
			<label for="history-days-select">Import window</label>
			<select id="history-days-select" bind:value={historyImportDays} disabled={importingHistory}>
				{#each historyImportDayOptions as option (option)}
					<option value={option}>{historyOptionLabel(option)}</option>
				{/each}
			</select>
			<button class="btn-sm import-btn" onclick={importHistory} disabled={importingHistory}>
				{#if importingHistory}
					<span class="spinner" aria-hidden="true"></span>
					Importing...
				{:else}
					Import
				{/if}
			</button>
		</div>
		{#if importingHistory && importStatusMessage}
			<p class="import-status">{importStatusMessage}</p>
		{/if}
		{#if historyImportError}
			<p class="error">{historyImportError}</p>
		{:else if historyImportResult}
			<p class="status">{historyImportResult}</p>
		{/if}
	</div>

	{#if localSettingsLoading}
		<p>Loading local settings...</p>
	{:else if localSettingsError}
		<p class="error">{localSettingsError}</p>
	{:else if localSettings}
		<div class="local-info">
			<h2>Local Environment</h2>
			<p class="label">Home Folder</p>
			<p class="path">{localSettings.homePath}</p>
			<p class="label">Agent Search Paths</p>
			{#if localSettings.conversationSearchPaths.length === 0}
				<p class="muted">No agent watchers are currently registered.</p>
			{:else}
				<ul class="search-paths">
					{#each localSettings.conversationSearchPaths as entry, index (index)}
						<li>
							<span class="agent">{entry.agent}</span>
							<span class="path">{entry.path}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}
</div>

<style>
	.settings {
		background: var(--color-background-content);
		padding: 1rem;
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	h2 {
		margin-top: 1rem;
		font-size: 1rem;
	}

	.label {
		margin: 0;
		font-size: 0.8rem;
		color: #666;
		text-transform: uppercase;
		letter-spacing: 0.02em;
	}

	.path {
		font-family:
			ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New',
			monospace;
		word-break: break-all;
	}

	.muted {
		color: #777;
		font-size: 0.85rem;
	}

	.tracking-section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.project-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.project-list li a {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.45rem 0.6rem;
		border: 0.5px solid var(--color-divider);
		border-radius: 6px;
		text-decoration: none;
		color: inherit;
	}

	.project-list li a:hover {
		background: var(--accent-color-ultralight);
		border-color: var(--accent-color-divider);
	}

	.project-list .project-name {
		font-weight: 600;
		font-size: 0.9rem;
	}

	.project-list .project-path {
		font-size: 0.8rem;
		opacity: 0.6;
		font-family:
			ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New',
			monospace;
	}

	.search-paths {
		margin: 0.75rem 0 0;
		padding: 0 0 0 1rem;
	}

	.search-paths li {
		margin-bottom: 0.4rem;
	}

	.history-import-controls {
		display: flex;
		gap: 0.6rem;
		align-items: center;
		flex-wrap: wrap;
	}

	.history-import-controls select {
		padding: 0.25rem 0.5rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		background: #fff;
		color: #444;
		font-size: 0.85rem;
	}

	.import-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}

	.import-btn:disabled {
		opacity: 0.7;
		cursor: wait;
	}

	.spinner {
		width: 0.8rem;
		height: 0.8rem;
		border: 2px solid #bbb;
		border-top-color: #333;
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.import-status {
		color: #666;
		font-size: 0.85rem;
		margin: 0.3rem 0 0 0;
		animation: fade-in 200ms ease;
	}

	@keyframes fade-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}

	.agent {
		display: inline-block;
		min-width: 4.5rem;
		font-weight: 600;
		text-transform: lowercase;
	}
</style>
