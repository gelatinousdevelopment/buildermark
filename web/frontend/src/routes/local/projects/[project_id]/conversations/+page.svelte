<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getProject } from '$lib/api';
	import { stars, shortId } from '$lib/utils';
	import type { ProjectDetail } from '$lib/types';

	let project: ProjectDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	onMount(async () => {
		try {
			const projectId = page.params.project_id;
			if (!projectId) throw new Error('Missing project ID');
			project = await getProject(projectId);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load project';
		} finally {
			loading = false;
		}
	});
</script>

{#if loading}
	<p class="loading">Loading project...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if !project}
	<p>Project not found.</p>
{:else}
	<div class="project-section">
		{#if project.conversations.length === 0}
			<p>No conversations.</p>
		{:else}
			<table class="data">
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
								<a
									href={resolve('/local/projects/[project_id]/conversations/[id]', {
										project_id: project.id,
										id: conv.id
									})}
								>
									{conv.title || shortId(conv.id)}
								</a>
							</td>
							<td>{conv.agent}</td>
							<td class="ratings">
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
{/if}

<style>
	.project-section {
		margin-bottom: 2rem;
	}

	.ratings {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
</style>
