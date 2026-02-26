<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { getProject, listProjectCommitsPage } from '$lib/api';
	import type { ProjectDetail, DailyCommitSummary } from '$lib/types';
	import { navStore } from '$lib/stores/nav.svelte';
	import { layoutStore } from '$lib/stores/layout.svelte';
	import DailyCommitsChart from '$lib/charts/DailyCommitsChart.svelte';
	import { projectDateFilterStore } from '$lib/stores/projectDateFilter.svelte';

	let { children } = $props();

	let project: ProjectDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let dailySummary: DailyCommitSummary[] = $state([]);
	let branch: string = $state('');

	onMount(async () => {
		try {
			const projectId = page.params.project_id;
			if (!projectId) throw new Error('Missing project ID');
			projectDateFilterStore.setProjectId(projectId);
			navStore.projectName = navStore.getCachedLabel(projectId) ?? null;
			project = await getProject(projectId);
			const label = project.label || project.path;
			navStore.projectName = label;
			navStore.setCachedLabel(projectId, label);

			try {
				const resp = await listProjectCommitsPage(projectId, 1, '', 1);
				dailySummary = resp.dailySummary ?? [];
				branch = resp.branch;
			} catch {
				// chart data is non-critical
			}
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
	<div class="inner">
		{#if loading}
			<p class="loading">Loading project...</p>
		{:else if error}
			<p class="error">{error}</p>
		{:else if project}
			<div class="chart-area">
				{#if dailySummary.length > 0}
					<DailyCommitsChart
						{dailySummary}
						{branch}
						projectId={page.params.project_id}
						compact={false}
						selectedDate={projectDateFilterStore.selectedDate}
						onDateSelect={(date) => {
							projectDateFilterStore.selectedDate = date;
						}}
					/>
				{/if}
			</div>
			<div class="details info-box">
				<h3>{project.label}</h3>
				<div class="project-path">
					<div class="value">{project.path}</div>
				</div>
				{#if project.localUser || project.localEmail}
					<div class="project-path">
						<div class="value">
							{#if project.localUser && project.localEmail}
								{project.localUser} &lt;{project.localEmail}&gt;
							{:else}
								{project.localUser || project.localEmail}
							{/if}
						</div>
					</div>
				{/if}
				{#if project.remoteUrl || project.remote}
					<div class="project-path">
						<div class="value">
							{#if project.remoteUrl}
								<a
									href={project.remoteUrl}
									target="_blank"
									rel="noopener noreferrer"
									class="remote-link">{project.remoteUrl}</a
								>
							{:else}
								{project.remote}
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>

<div
	class="content limited-content-width inset-when-limited-content-width"
	class:fixed-height={layoutStore.fixedHeight}
>
	{@render children()}
</div>

<style>
	.project-header {
		align-items: stretch;
		background: var(--color-background-project-header);
		border-bottom: 0.5px solid var(--color-divider);
		display: flex;
		flex-direction: row;
		gap: 1rem;
		justify-content: center;
		padding: 1rem;
	}

	.inner {
		display: flex;
		gap: 1rem;
		flex: 1;
		flex-wrap: wrap;
		max-width: var(--content-width);
	}

	.inner > * {
		height: calc(108px + 2rem);
	}

	.chart-area {
		margin-bottom: -0.5rem;
		min-width: 0;
	}

	.project-header .details {
		align-items: flex-start;
		display: flex;
		flex-direction: column;
		flex: 1;
		gap: 0.5rem;
		height: 108px;
		min-width: 20rem;
	}

	h3 {
		font-size: 2rem;
		font-weight: 300;
		letter-spacing: 0.03rem;
		margin: 0;
	}

	.project-path {
		display: flex;
		gap: 0.3rem;
		font-size: 0.9rem;
		margin: 0;
	}

	.remote-link {
		text-decoration: none;
		color: var(--color-link-body);
	}

	.remote-link:hover {
		text-decoration: underline;
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
