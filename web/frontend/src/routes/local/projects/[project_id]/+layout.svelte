<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { getProject } from '$lib/api';
	import type { ProjectDetail } from '$lib/types';
	import { navStore } from '$lib/stores/nav.svelte';

	let { children } = $props();

	let project: ProjectDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	onMount(async () => {
		try {
			const projectId = page.params.project_id;
			if (!projectId) throw new Error('Missing project ID');
			project = await getProject(projectId);
			navStore.projectName = project.label || project.path;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load project';
		} finally {
			loading = false;
		}
	});

	onDestroy(() => {
		navStore.projectName = null;
	});
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -->

<div class="project-header">
	{#if loading}
		<p class="loading">Loading project...</p>
	{:else if error}
		<p class="error">{error}</p>
	{:else if project}
		<!-- <div class="nav">
			<h2>{project.label || project.path}</h2>
			<a
				href={'/local/projects/' + page.params.project_id + '/commits'}
				class="link"
				class:selected={page.url.pathname.includes('/commits')}>Commits</a
			>
			<a
				href={'/local/projects/' + page.params.project_id + '/conversations'}
				class="link"
				class:selected={page.url.pathname.includes('/conversations')}>Conversations</a
			>
		</div>
		{#if project.label}
			<p class="project-path">{project.path}</p>
		{/if} -->
		<h2>{project.label || project.path}</h2>
		<p class="project-path">{project.path}</p>
	{/if}
</div>

<div class="content">
	{@render children()}
</div>

<style>
	.project-header {
		border-bottom: 0.5px solid var(--color-divider);
		/*margin: 0 -1rem 1rem -1rem;*/
		/*padding: 0 1rem 1rem 1rem;*/
		padding: 1rem;
		/*min-height: 4rem;*/
	}

	.project-header h2 {
		font-size: 1.8rem;
		font-weight: 300;
		letter-spacing: 0.03rem;
		opacity: 0.7;
		margin: 0 0.5rem 0 0;
	}

	.project-path {
		font-size: 0.9rem;
		color: #999;
		margin: 0.3rem 0 0 0;
	}

	.content {
		background: var(--color-background-content);
		flex: 1;
		padding: 1rem;
	}
</style>
