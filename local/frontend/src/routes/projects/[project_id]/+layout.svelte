<script lang="ts">
	import { onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
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
	let order = $derived(page.url.searchParams.get('order') ?? 'desc');

	function handleOrderChange(e: Event) {
		const value = (e.target as HTMLInputElement).value;
		const qs = value === 'asc' ? '?order=asc' : '';
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		goto(resolve('/projects/[project_id]', { project_id: page.params.project_id ?? '' }) + qs, {
			replaceState: true,
			keepFocus: true
		});
	}

	$effect(() => {
		const projectId = page.params.project_id;
		if (!projectId) return;

		// Reset state for the new project
		project = null;
		loading = true;
		error = null;
		dailySummary = [];
		branch = '';

		projectDateFilterStore.setProjectId(projectId);
		navStore.projectName = navStore.getCachedLabel(projectId) ?? null;

		(async () => {
			try {
				const p = await getProject(projectId);
				// Guard against stale responses if project_id changed during fetch
				if (page.params.project_id !== projectId) return;
				project = p;
				const label = p.label || p.path;
				navStore.projectName = label;
				navStore.setCachedLabel(projectId, label);

				try {
					const resp = await listProjectCommitsPage(projectId, 1, '', 1);
					if (page.params.project_id !== projectId) return;
					dailySummary = resp.dailySummary ?? [];
					branch = resp.branch;
				} catch {
					// chart data is non-critical
				}
			} catch (e) {
				if (page.params.project_id !== projectId) return;
				error = e instanceof Error ? e.message : 'Failed to load project';
			} finally {
				if (page.params.project_id === projectId) {
					loading = false;
				}
			}
		})();
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
			<div class="loading">Loading project...</div>
		{:else if error}
			<div class="error">{error}</div>
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
				<h3>
					<div class="sort-radios" style:float="right">
						<label class:selected={order === 'desc'}>
							<input
								type="radio"
								name="sort-order"
								value="desc"
								checked={order === 'desc'}
								onchange={handleOrderChange}
							/>
							Newest
						</label>
						<label class:selected={order === 'asc'}>
							<input
								type="radio"
								name="sort-order"
								value="asc"
								checked={order === 'asc'}
								onchange={handleOrderChange}
							/>
							Oldest
						</label>
					</div>
					{project.label}
				</h3>
				<div class="detail">
					<div class="label">Local Path:</div>
					<div class="value">{project.path}</div>
				</div>
				{#if project.localUser || project.localEmail}
					<div class="detail">
						<div class="label">Local User:</div>
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
					<div class="detail">
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
	class:content={!layoutStore.hideContainer}
	class:limited-content-width={!layoutStore.hideContainer}
	class:inset-when-limited-content-width={!layoutStore.hideContainer}
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
		margin-bottom: -0.5rem;
		max-width: var(--content-width);
	}

	.inner > div {
		height: calc(108px + 2rem);
	}

	.chart-area {
		min-width: 0;
	}

	.project-header .details {
		align-items: flex-start;
		display: flex;
		flex-direction: column;
		flex: 1;
		gap: 0.5rem;
		height: 108px;
		min-width: 16rem;
	}

	h3 {
		font-size: 2rem;
		font-weight: 300;
		letter-spacing: 0.03rem;
		margin: 0;
		width: 100%;
	}

	.detail {
		display: flex;
		gap: 0.3rem;
		font-size: 0.9rem;
		margin: 0;
		max-width: 100%;
	}

	.label {
		flex: 0 0 auto;
		font-weight: 600;
		opacity: 0.6;
		white-space: nowrap;
	}

	.value {
		float: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;

		flex: 1 1 auto;
		min-width: 0;
	}

	.remote-link {
		text-decoration: none;
		color: var(--color-link-body);
	}

	.remote-link:hover {
		text-decoration: underline;
	}

	.sort-radios {
		display: flex;
		gap: 0;
	}

	.sort-radios label {
		cursor: pointer;
		font-size: 0.85rem;
		padding: 0.2rem 0.5rem;
		border: 0.5px solid var(--color-border-input);
		background: var(--color-background-elevated);
	}

	.sort-radios label:first-child {
		border-radius: 4px 0 0 4px;
	}

	.sort-radios label:last-child {
		border-radius: 0 4px 4px 0;
		border-left: 0;
	}

	.sort-radios label.selected {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
		border-color: var(--accent-color);
	}

	.sort-radios label.selected + label {
		border-left: 0;
	}

	.sort-radios input {
		display: none;
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
