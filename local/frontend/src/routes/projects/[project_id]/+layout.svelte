<script lang="ts">
	import { onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { navStore } from '$lib/stores/nav.svelte';
	import { layoutStore } from '$lib/stores/layout.svelte';
	import DailyCommitsChart from '$lib/charts/DailyCommitsChart.svelte';
	import { projectDateFilterStore } from '$lib/stores/projectDateFilter.svelte';
	import { projectLayoutData } from '$lib/stores/projectLayoutData.svelte';

	const DAILY_WINDOW_OPTIONS = [14, 30, 45, 60, 90];

	let { children, data } = $props();

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

	// Reset store and date filter when project changes
	$effect(() => {
		const projectId = page.params.project_id;
		if (!projectId) return;
		projectLayoutData.reset(projectId);
		projectDateFilterStore.setProjectId(projectId);
	});

	// Set nav label from best available source: child data > cache > projects list
	$effect(() => {
		const projectId = page.params.project_id;
		if (!projectId) return;
		const project = projectLayoutData.project;
		if (project) {
			const label = project.label || project.path;
			navStore.projectName = label;
			navStore.setCachedLabel(projectId, label);
			return;
		}
		const cached = navStore.getCachedLabel(projectId);
		if (cached) {
			navStore.projectName = cached;
			return;
		}
		const match = data.projects?.find((p) => p.id === projectId);
		navStore.projectName = match ? match.label || match.path : null;
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
		{#if projectLayoutData.project}
			<div class="chart-area">
				{#if projectLayoutData.dailySummary.length > 0}
					<DailyCommitsChart
						dailySummary={projectLayoutData.dailySummary}
						branch={projectLayoutData.branch}
						projectId={page.params.project_id}
						compact={false}
						selectedDate={projectDateFilterStore.selectedDate}
						windowDays={projectLayoutData.dailyWindowDays}
						windowDayOptions={DAILY_WINDOW_OPTIONS}
						onDateSelect={(date: string | null) => {
							projectDateFilterStore.selectedDate = date;
						}}
						onWindowDaysChange={(days: number) => {
							projectLayoutData.setDailyWindowDays(days);
						}}
						height={116}
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
					{projectLayoutData.project.label}
				</h3>
				<div class="detail">
					<div class="label">Local Path:</div>
					<div class="value">{projectLayoutData.project.path}</div>
				</div>
				{#if projectLayoutData.project.localUser || projectLayoutData.project.localEmail}
					<div class="detail">
						<div class="label">Local User:</div>
						<div class="value">
							{#if projectLayoutData.project.localUser && projectLayoutData.project.localEmail}
								{projectLayoutData.project.localUser} &lt;{projectLayoutData.project.localEmail}&gt;
							{:else}
								{projectLayoutData.project.localUser || projectLayoutData.project.localEmail}
							{/if}
						</div>
					</div>
				{/if}
				{#if projectLayoutData.project.remoteUrl || projectLayoutData.project.remote}
					<div class="detail">
						<div class="value">
							{#if projectLayoutData.project.remoteUrl}
								<a
									href={projectLayoutData.project.remoteUrl}
									target="_blank"
									rel="noopener noreferrer"
									class="remote-link">{projectLayoutData.project.remoteUrl}</a
								>
							{:else}
								{projectLayoutData.project.remote}
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>

<div
	class="container"
	class:content={!layoutStore.hideContainer}
	class:limited-content-width={!layoutStore.hideContainer}
	class:inset-when-limited-content-width={!layoutStore.hideContainer}
	class:fixed-height={layoutStore.fixedHeight}
	style:background={layoutStore.fixedHeight ? 'var(--color-background-content)' : ''}
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
		min-height: 127px;
	}

	.inner {
		align-items: flex-end;
		display: flex;
		gap: 1rem;
		flex: 1;
		flex-wrap: wrap-reverse;
		margin-bottom: -0.5rem;
		max-width: var(--content-width);
	}

	.inner > div {
		height: calc(108px + 2rem);
	}

	.chart-area {
		min-width: 0;
		width: 860px;
	}

	.project-header .details {
		align-items: flex-start;
		display: flex;
		flex-direction: column;
		flex: 1;
		gap: 0.5rem;
		height: 108px;
		min-width: 28rem;
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
	}

	.container.fixed-height {
		display: flex;
		flex: 1;
		flex-direction: column;
		min-height: 0;
		overflow: hidden;
	}
</style>
