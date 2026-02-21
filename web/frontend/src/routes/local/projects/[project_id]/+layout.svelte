<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { getProject } from '$lib/api';
	import type { ProjectDetail } from '$lib/types';
	import { navStore } from '$lib/stores/nav.svelte';
	import { layoutStore } from '$lib/stores/layout.svelte';

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

<div
	class="project-header"
	style:display={page.url.pathname.endsWith(page.params.project_id || '') ? 'flex' : 'none'}
>
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
		<!-- <h2>{project.label || project.path}</h2> -->
		<div class="project-path">
			<div class="label">Local:</div>
			<div class="value">{project.path}</div>
		</div>
		<div class="project-path">
			<div class="label">Remote:</div>
			<div class="value">
				{#if project.remoteUrl}
					<a href={project.remoteUrl} target="_blank" rel="noopener noreferrer">{project.remote}</a>
				{:else}
					{project.remote}
				{/if}
			</div>
		</div>
	{/if}
</div>

<div
	class="content limited-content-width inset-when-limited-content-width"
	class:fixed-height={layoutStore.fixedHeight}
>
	{@render children()}
</div>

<style>
	.project-header {
		background: var(--color-background-project-header);
		border-bottom: 0.5px solid var(--color-divider);
		display: flex;
		flex-direction: column;
		gap: 0rem;
		padding: 1rem;
	}

	.project-path {
		display: flex;
		gap: 0.3rem;
		font-size: 0.9rem;
		margin: 0.3rem 0 0 0;
	}

	.project-path .label {
		font-weight: bold;
	}

	.content {
		background: var(--color-background-content);
		/*padding: 1rem;*/

		/*background: var(--color-background-content);
		border-radius: 10px;
		border: 0.5px solid var(--color-divider);
		box-sizing: border-box;
		margin: 2rem auto;
		transition: all 200ms;
		width: 100%;*/
	}

	.content.fixed-height {
		display: flex;
		flex: 1;
		flex-direction: column;
		min-height: 0;
		overflow: hidden;
	}

	/*@media (max-width: 1600px) {
		.content {
			border-width: 0 0 0.5px 0;
			margin: 0 auto;
			border-radius: 0;
		}
	}*/
</style>
