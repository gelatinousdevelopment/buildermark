<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { listProjects, getProject } from '$lib/api';
	import { stars, shortId } from '$lib/utils';
	import type { ProjectDetail } from '$lib/types';

	let projects: ProjectDetail[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	onMount(async () => {
		try {
			const list = await listProjects();
			const details = await Promise.all(list.map((p) => getProject(p.id)));
			projects = details;
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
	<p>No projects found.</p>
{:else}
	{#each projects as project (project.id)}
		<div class="project-section">
			<h2>{project.path}</h2>
			{#if project.conversations.length === 0}
				<p>No conversations.</p>
			{:else}
				<table>
					<thead>
						<tr>
							<th>Conversation</th>
							<th>Agent</th>
							<th>Ratings</th>
						</tr>
					</thead>
					<tbody>
						{#each project.conversations as conv (conv.id)}
							<tr>
								<td>
									<a href={resolve('/dashboard/conversations/[id]', { id: conv.id })}>
										{shortId(conv.id)}
									</a>
								</td>
								<td>{conv.agent}</td>
								<td>
									{#if conv.ratings.length > 0}
										{#each conv.ratings as r (r.id)}
											<span title={r.analysis ? `${r.note}\n\nAnalysis: ${r.analysis}` : r.note}
												>{stars(r.rating)}</span
											>
										{/each}
									{:else}
										—
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	{/each}
{/if}
