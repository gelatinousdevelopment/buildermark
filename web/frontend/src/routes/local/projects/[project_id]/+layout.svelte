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
		<div class="nav">
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
		{/if}
	{/if}
</div>

{@render children()}

<style>
	.project-header {
		margin-bottom: 1rem;
	}

	.nav {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.nav .link {
		font-size: 0.9rem;
		text-decoration: none;
		padding: 0.4rem 0.6rem;
		border-radius: 3px;
	}

	.nav .link.selected {
		background: var(--accent-color-ultralight);
	}

	.project-header h2 {
		font-size: 1.1rem;
		margin: 0 0.5rem 0 0;
		color: #333;
	}

	.project-path {
		font-size: 0.8rem;
		color: #999;
		margin: 0.3rem 0 0 0;
	}
</style>
