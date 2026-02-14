<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getProject } from '$lib/api';
	import type { ProjectDetail } from '$lib/types';

	let { children } = $props();

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

<div class="project-header">
	{#if loading}
		<p class="loading">Loading project...</p>
	{:else if error}
		<p class="error">{error}</p>
	{:else if project}
		<h2>{project.label || project.path}</h2>
		{#if project.label}
			<p class="project-path">{project.path}</p>
		{/if}
	{/if}
</div>

{@render children()}

<style>
	.project-header {
		margin-bottom: 1rem;
	}

	.project-header h2 {
		font-size: 1.1rem;
		margin: 0;
		color: #333;
	}

	.project-path {
		font-size: 0.8rem;
		color: #999;
		margin: 0.3rem 0 0 0;
	}
</style>
