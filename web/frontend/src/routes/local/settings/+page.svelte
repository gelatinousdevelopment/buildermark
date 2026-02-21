<script lang="ts">
	import { onMount } from 'svelte';
	import { getLocalSettings, listProjects, setProjectIgnored } from '$lib/api';
	import type { LocalSettings, Project } from '$lib/types';

	type ProjectSetting = {
		project: Project;
		tracked: boolean;
		saving: boolean;
		error: string | null;
	};

	let rows: ProjectSetting[] = $state([]);
	let loadingProjects = $state(true);
	let projectError: string | null = $state(null);

	let localSettingsLoading = $state(true);
	let localSettingsError: string | null = $state(null);
	let localSettings: LocalSettings | null = $state(null);
	let showSearchPaths = $state(false);

	function projectName(project: Project): string {
		return project.label || project.path;
	}

	function sortProjects(projects: Project[]): Project[] {
		return projects.slice().sort((a, b) => projectName(a).localeCompare(projectName(b)));
	}

	onMount(async () => {
		await Promise.all([loadProjects(), loadLocalSettings()]);
	});

	async function loadProjects() {
		try {
			const [trackedProjects, ignoredProjects] = await Promise.all([
				listProjects(false),
				listProjects(true)
			]);
			const tracked = sortProjects(trackedProjects).map((project) => ({
				project,
				tracked: true,
				saving: false,
				error: null
			}));
			const ignored = sortProjects(ignoredProjects).map((project) => ({
				project,
				tracked: false,
				saving: false,
				error: null
			}));
			rows = [...tracked, ...ignored].sort((a, b) =>
				projectName(a.project).localeCompare(projectName(b.project))
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

	async function setTracked(projectId: string, tracked: boolean) {
		const rowIndex = rows.findIndex((row) => row.project.id === projectId);
		if (rowIndex < 0) return;

		const previousTracked = rows[rowIndex].tracked;
		rows[rowIndex] = { ...rows[rowIndex], tracked, saving: true, error: null };
		rows = rows.slice();

		try {
			await setProjectIgnored(projectId, !tracked);
		} catch (e) {
			rows[rowIndex] = {
				...rows[rowIndex],
				tracked: previousTracked,
				error: e instanceof Error ? e.message : 'Failed to update tracking state'
			};
		} finally {
			rows[rowIndex] = { ...rows[rowIndex], saving: false };
			rows = rows.slice();
		}
	}
</script>

<div class="settings limited-content-width inset-when-limited-content-width">
	<h1>Global Settings</h1>

	{#if loadingProjects}
		<p>Loading projects...</p>
	{:else if projectError}
		<p class="error">{projectError}</p>
	{:else if rows.length === 0}
		<p>No projects found.</p>
	{:else}
		<div>
			<h2>Project Tracking</h2>
			<table class="project-table">
				<thead>
					<tr>
						<th></th>
						<th>Project</th>
						<th>Path</th>
					</tr>
				</thead>
				<tbody>
					{#each rows as row (row.project.id)}
						<tr>
							<td>
								<input
									type="checkbox"
									checked={row.tracked}
									disabled={row.saving}
									onchange={(event) =>
										setTracked(row.project.id, (event.currentTarget as HTMLInputElement).checked)}
								/>
							</td>
							<td class="project-name-cell" title={projectName(row.project)}
								>{projectName(row.project)}</td
							>
							<td class="project-path">
								{#if row.saving}
									<span class="status">Saving...</span>
								{:else if row.error}
									<span class="error">{row.error}</span>
								{:else}
									{row.project.path}
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

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

	.toggle {
		margin-top: 0.25rem;
		border: 1px solid #ccc;
		background: #f7f7f7;
		border-radius: 6px;
		padding: 0.35rem 0.6rem;
		font-size: 0.85rem;
		cursor: pointer;
	}

	.muted {
		color: #777;
		font-size: 0.85rem;
	}

	.search-paths {
		margin: 0.75rem 0 0;
		padding: 0 0 0 1rem;
	}

	.search-paths li {
		margin-bottom: 0.4rem;
	}

	.agent {
		display: inline-block;
		min-width: 4.5rem;
		font-weight: 600;
		text-transform: lowercase;
	}

	.project-table {
		width: 100%;
		border-collapse: collapse;
		table-layout: fixed;
		background: #fff;
		border: 1px solid #ddd;
		border-radius: 8px;
		overflow: hidden;
	}

	.project-table th,
	.project-table td {
		padding: 0.45rem 0.6rem;
		border-bottom: 1px solid #eee;
		text-align: left;
		font-size: 0.9rem;
		vertical-align: middle;
	}

	.project-table th {
		font-size: 0.8rem;
		color: #666;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.02em;
	}

	.project-table tbody tr:last-child td {
		border-bottom: none;
	}

	.project-table th:nth-child(1),
	.project-table td:nth-child(1) {
		width: 1.5rem;
	}

	.project-table th:nth-child(2),
	.project-table td:nth-child(2) {
		width: 10rem;
	}

	.project-name-cell {
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.project-path {
		color: #777;
		font-size: 0.85rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.status {
		font-size: 0.85rem;
		color: #666;
	}

	.error {
		color: #b00020;
		font-size: 0.85rem;
	}

	@media (max-width: 800px) {
		.project-table th:nth-child(2),
		.project-table td:nth-child(2) {
			width: 10rem;
		}
	}
</style>
