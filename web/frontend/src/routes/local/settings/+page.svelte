<script lang="ts">
	import { onMount } from 'svelte';
	import { listProjects, setProjectIgnored } from '$lib/api';
	import type { Project } from '$lib/types';

	type ProjectSetting = {
		project: Project;
		tracked: boolean;
		saving: boolean;
		error: string | null;
	};

	let rows: ProjectSetting[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	function projectName(project: Project): string {
		return project.label || project.path;
	}

	function sortProjects(projects: Project[]): Project[] {
		return projects.slice().sort((a, b) => projectName(a).localeCompare(projectName(b)));
	}

	onMount(async () => {
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
			error = e instanceof Error ? e.message : 'Failed to load projects';
		} finally {
			loading = false;
		}
	});

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

<div class="settings">
	<h1>Settings</h1>
	<h2>Project Tracking</h2>

	{#if loading}
		<p>Loading projects...</p>
	{:else if error}
		<p class="error">{error}</p>
	{:else if rows.length === 0}
		<p>No projects found.</p>
	{:else}
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
	{/if}
</div>

<style>
	.settings {
		background: var(--color-background-content);
		padding: 1rem;
		flex: 1;
	}

	h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	h2 {
		margin-top: 1rem;
		font-size: 1rem;
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
