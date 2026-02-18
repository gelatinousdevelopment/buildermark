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
				<div class="meta">
					<div class="label">{project.label || project.path}</div>
					<div class="path">{project.path}</div>
				</div>
				<div class="content">
					<div class="column conversations"><div class="heading">Agent Conversations</div></div>
					<div class="column commits"><div class="heading">Git Commits</div></div>
				</div>
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

	.project .content {
		align-items: stretch;
		background: #fbfbfb;
		border-radius: 12px;
		border: 0.5px solid #ccc;
		display: flex;
		flex-direction: row;
		justify-content: space-between;
		padding: 0;
	}

	@media (max-width: 1023px) {
		.project .content {
			flex-direction: column;
		}
	}

	.project:has(.content:hover) {
		.meta .label {
			opacity: 1;
			color: var(--accent-color);
		}

		.content .column .heading {
			opacity: 0.75;
		}
	}

	.project:has(.content) .meta .label::before {
		background: var(--accent-color-ultralight);
		border-radius: 5px;
		content: '';
		inset: 0px auto 0px 0px;
		position: absolute;
		transition: width 150ms ease-in-out;
		width: 0%;
		z-index: -1;
	}

	.project:has(.content:hover) .meta .label::before {
		width: 100%;
	}

	.project .content:hover {
		background: #f8f8f8;
		border-color: #bbb;
	}

	.project .column {
		min-height: 18rem;
		padding: 1rem;
	}

	.project .column .heading {
		font-weight: 600;
		text-transform: uppercase;
		font-size: 0.9rem;
		opacity: 0.5;
	}

	.meta {
		display: flex;
		gap: 1rem;
		align-items: flex-end;
		justify-content: space-between;
		padding: 0.5rem 1rem 1rem 1rem;
	}

	.meta .label {
		box-sizing: border-box;
		font-size: 1.8rem;
		font-weight: 300;
		letter-spacing: 0.03rem;
		opacity: 0.7;
		padding: 0.2rem 0.8rem;
		margin: -0.2rem -0.8rem;
		border: 1px solid transparent;
		border-radius: 5px;
		position: relative;
	}

	.meta .path {
		font-size: 0.9rem;
		font-weight: 400;
		opacity: 0.5;
	}

	.conversations {
		flex: 2;
	}

	.commits {
		border-left: 0.5px solid var(--color-divider);
		flex: 2;
	}

	@media (max-width: 1023px) {
		.commits {
			border-left: 0;
			border-top: 0.5px solid var(--color-divider);
		}
	}
</style>
