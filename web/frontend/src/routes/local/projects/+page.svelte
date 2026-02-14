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
	<h2>Projects</h2>
	<table class="data">
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
	</table>
{/if}
