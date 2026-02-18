<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { listProjects } from '$lib/api';
	import type { Project } from '$lib/types';

	let projects: Project[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	onMount(async () => {
		try {
			projects = await listProjects(false);
			projects = projects
				.filter((p) => p.gitId)
				.sort((a, b) => (a.label || a.path).localeCompare(b.label || b.path));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load projects';
		} finally {
			loading = false;
		}
	});
</script>

{#if loading}
	<p class="loading">Loading projects...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if projects.length === 0}
	<p>No tracked projects with git IDs found.</p>
{:else}
	<div class="projects">
		{#each projects as project (project.id)}
			<div class="project">
				<div class="column meta">
					<div class="label">{project.label || project.path}</div>
					<div class="path">{project.path}</div>
				</div>
				<div class="column conversations">Conversations</div>
				<div class="column commits">Commits</div>
			</div>
		{/each}
	</div>

	<!-- <table class="data">
		<thead>
			<tr>
				<th>Project</th>
				<th>Path</th>
				<th></th>
			</tr>
		</thead>
		<tbody>
			{#each projects as project (project.id)}
				<tr>
					<td>{project.label || project.path}</td>
					<td>{project.path}</td>
					<td>
						<a href={resolve('/local/projects/[project_id]/commits', { project_id: project.id })}
							>Commits</a
						>
						<a
							href={resolve('/local/projects/[project_id]/conversations', {
								project_id: project.id
							})}>Conversations</a
						>
					</td>
				</tr>
			{/each}
		</tbody>
	</table> -->
{/if}

<style>
	.projects {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.project {
		align-items: stretch;
		background: #fbfbfb;
		border-radius: 10px;
		border: 0.5px solid #ccc;
		display: flex;
		flex-direction: row;
		justify-content: space-between;
		min-height: 10rem;
		padding: 0;
	}

	.project:hover {
		background: var(--accent-color-ultralight);
		border-color: var(--accent-color);
		box-shadow: 1px 1px 3px 1px rgba(0, 0, 0, 0.1);
	}

	.project .column {
		padding: 1rem;
	}

	.meta {
		flex: 1.3;
	}

	.conversations {
		border-left: 0.5px solid var(--color-divider);
		flex: 2;
	}

	.commits {
		border-left: 0.5px solid var(--color-divider);
		flex: 2;
	}
</style>
