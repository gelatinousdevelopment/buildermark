<script lang="ts">
	import { onMount } from 'svelte';
	import {
		discoverImportableProjects,
		getLocalSettings,
		importProjects,
		listProjects,
		scanHistory,
		setProjectIgnored
	} from '$lib/api';
	import ProjectTrackingForm from '$lib/components/project/ProjectTrackingForm.svelte';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import type { LocalSettings, Project, ProjectTrackingOption } from '$lib/types';

	type SettingsTrackingOption = ProjectTrackingOption & {
		originalTracked: boolean;
	};

	let trackingOptions: SettingsTrackingOption[] = $state([]);
	let checkedProjectPaths: string[] = $state([]);
	let loadingProjects = $state(true);
	let projectError: string | null = $state(null);
	let trackingImportDays = $state('90');
	let savingTracking = $state(false);
	let saveTrackingError: string | null = $state(null);

	let localSettingsLoading = $state(true);
	let localSettingsError: string | null = $state(null);
	let localSettings: LocalSettings | null = $state(null);
	let historyImportDays = $state('7');
	let importingHistory = $state(false);
	let historyImportError: string | null = $state(null);
	let historyImportResult: string | null = $state(null);

	const historyImportDayOptions = ['7', '14', '30', '60', '90', '180', '365', 'all'];

	let importStatusMessage = $derived(
		websocketStore.importStatus?.state === 'running'
			? websocketStore.importStatus.message
			: null
	);

	const selectedImportablePaths = $derived(
		trackingOptions
			.filter((option) => checkedProjectPaths.includes(option.path) && option.importable)
			.map((option) => option.path)
	);
	const trackingHasChanges = $derived(
		trackingOptions.some(
			(option) => checkedProjectPaths.includes(option.path) !== option.originalTracked
		)
	);
	const trackingSubmitDisabled = $derived(
		!trackingHasChanges && selectedImportablePaths.length === 0
	);

	function projectName(project: { label: string; path: string }): string {
		return project.label || project.path;
	}

	function sortProjects(projects: Project[]): Project[] {
		return projects.slice().sort((a, b) => projectName(a).localeCompare(projectName(b)));
	}

	onMount(async () => {
		await Promise.all([loadTrackingOptions(), loadLocalSettings()]);
	});

	function setTrackingImportDays(days: string) {
		trackingImportDays = days;
	}

	function toggleSelection(projectPath: string, checked: boolean) {
		if (checked) {
			checkedProjectPaths = checkedProjectPaths.includes(projectPath)
				? checkedProjectPaths
				: [...checkedProjectPaths, projectPath];
		} else {
			checkedProjectPaths = checkedProjectPaths.filter((path) => path !== projectPath);
		}
	}

	async function loadTrackingOptions() {
		loadingProjects = true;
		projectError = null;
		try {
			const [trackedProjects, ignoredProjects, discovered] = await Promise.all([
				listProjects(false),
				listProjects(true),
				discoverImportableProjects(30)
			]);

			const byPath: Record<string, SettingsTrackingOption> = {};

			const upsertProject = (project: Project, tracked: boolean) => {
				const existing = byPath[project.path];
				const next: SettingsTrackingOption = {
					path: project.path,
					label: project.label,
					projectId: project.id,
					tracked,
					originalTracked: tracked,
					importable: false,
					missingOnDisk: true
				};
				if (!existing) {
					byPath[project.path] = next;
					return;
				}
				byPath[project.path] = {
					...existing,
					...next,
					importable: existing.importable,
					missingOnDisk: existing.missingOnDisk
				};
			};

			for (const project of sortProjects(trackedProjects)) {
				upsertProject(project, true);
			}
			for (const project of sortProjects(ignoredProjects)) {
				upsertProject(project, false);
			}

			for (const project of discovered.projects) {
				const existing = byPath[project.path];
				if (!existing) {
					byPath[project.path] = {
						path: project.path,
						label: project.label,
						projectId: project.projectId,
						tracked: project.tracked,
						originalTracked: project.tracked,
						importable: true,
						missingOnDisk: false
					};
					continue;
				}

				byPath[project.path] = {
					...existing,
					label: existing.label || project.label,
					projectId: existing.projectId || project.projectId,
					importable: true,
					missingOnDisk: false
				};
			}

			trackingOptions = Object.values(byPath).sort((a, b) =>
				projectName(a).localeCompare(projectName(b))
			);
			checkedProjectPaths = trackingOptions
				.filter((option) => option.tracked)
				.map((option) => option.path);
		} catch (e) {
			projectError = e instanceof Error ? e.message : 'Failed to load projects';
		} finally {
			loadingProjects = false;
		}
	}

	async function saveProjectTracking() {
		if (savingTracking || trackingSubmitDisabled) return;
		savingTracking = true;
		saveTrackingError = null;
		websocketStore.clearImportStatus();
		try {
			const desiredTracked = new Set(checkedProjectPaths);
			const updates = trackingOptions.filter(
				(option) => option.projectId && desiredTracked.has(option.path) !== option.originalTracked
			);

			if (updates.length > 0) {
				await Promise.all(
					updates.map((option) =>
						setProjectIgnored(option.projectId!, !desiredTracked.has(option.path))
					)
				);
			}

			if (selectedImportablePaths.length > 0) {
				await importProjects(selectedImportablePaths, trackingImportDays);
				// Import runs async on the server; wait for completion via WebSocket.
				const result = await websocketStore.waitForImportComplete();
				if (result.state === 'error') {
					saveTrackingError = result.message;
					return;
				}
			}

			await loadTrackingOptions();
		} catch (e) {
			saveTrackingError = e instanceof Error ? e.message : 'Failed to update project tracking';
		} finally {
			savingTracking = false;
			websocketStore.clearImportStatus();
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
			// About 100 years; effectively "all" for practical local history.
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
		try {
			const response = await scanHistory(historyImportTimeframe(historyImportDays));
			historyImportResult = `Imported ${response.entriesProcessed.toLocaleString()} entries.`;
		} catch (e) {
			historyImportError = e instanceof Error ? e.message : 'Failed to import history';
		} finally {
			importingHistory = false;
		}
	}
</script>

<div class="settings limited-content-width inset-when-limited-content-width">
	<h1>Global Settings</h1>

	<div class="tracking-section">
		{#if loadingProjects}
			<p>Loading projects...</p>
		{:else if projectError}
			<p class="error">{projectError}</p>
		{:else}
			<ProjectTrackingForm
				heading="Project Tracking"
				projects={trackingOptions}
				loading={loadingProjects}
				error={projectError}
				emptyMessage="No projects found yet."
				checkedPaths={checkedProjectPaths}
				selectedHistoryDays={trackingImportDays}
				historyDayOptions={historyImportDayOptions}
				saving={savingTracking}
				saveError={saveTrackingError}
				{importStatusMessage}
				submitLabel="Import Projects"
				submitDisabled={trackingSubmitDisabled}
				onToggle={toggleSelection}
				onHistoryDaysChange={setTrackingImportDays}
				onSubmit={saveProjectTracking}
			/>
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
		gap: 0.6rem;
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

	.agent {
		display: inline-block;
		min-width: 4.5rem;
		font-weight: 600;
		text-transform: lowercase;
	}
</style>
