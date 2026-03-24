<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import {
		listProjectCommitsPage,
		getProjectDailyActivity,
		getProjectRatingsByAgent,
		getProjectFileTypeCoverage
	} from '$lib/api';
	import DailyCommitsChart from '$lib/charts/DailyCommitsChart.svelte';
	import DailyActivityChart from '$lib/charts/DailyActivityChart.svelte';
	import RatingsByAgentChart from '$lib/charts/RatingsByAgentChart.svelte';
	import FileTypeCoverageChart from '$lib/charts/FileTypeCoverageChart.svelte';
	import type {
		DailyCommitSummary,
		DailyActivityRow,
		AgentRatingDistribution,
		FileTypeCoverage
	} from '$lib/types';
	import { referenceNowDate } from '$lib/utils';
	import Icon from '$lib/Icon.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { navStore } from '$lib/stores/nav.svelte';
	import { layoutStore } from '$lib/stores/layout.svelte';

	onMount(() => {
		layoutStore.hideContainer = true;
	});
	onDestroy(() => {
		layoutStore.hideContainer = false;
	});

	const PRESET_DAYS = [7, 14, 30, 45, 60, 90, 180, 365] as const;
	const DAY_IN_MS = 24 * 60 * 60 * 1000;
	const DEFAULT_PRESET_DAYS = 30;

	type PresetDays = (typeof PRESET_DAYS)[number];

	interface RangeSelection {
		mode: 'preset' | 'custom';
		presetDays: PresetDays;
		startDate: string;
		endDate: string;
	}

	const projectId = $derived(page.params.project_id ?? '');

	let dailySummary: DailyCommitSummary[] = $state([]);
	let branch = $state('');
	let loading = $state(false);
	let error: string | null = $state(null);
	let lastLoadKey = '';
	let requestToken = 0;

	let dailyActivity: DailyActivityRow[] = $state([]);
	let activityLoading = $state(false);
	let activityError: string | null = $state(null);
	let lastActivityLoadKey = '';
	let activityRequestToken = 0;
	let countChildConversationsSeparately = $derived(
		settingsStore.activityChartCountChildConversationsSeparately
	);

	let ratingsByAgent: AgentRatingDistribution[] = $state([]);
	let ratingsLoading = $state(false);
	let ratingsError: string | null = $state(null);
	let lastRatingsLoadKey = '';
	let ratingsRequestToken = 0;

	let fileTypeCoverage: FileTypeCoverage[] = $state([]);
	let fileTypeCoverageLoading = $state(false);
	let fileTypeCoverageError: string | null = $state(null);
	let lastFileTypeCoverageLoadKey = '';
	let fileTypeCoverageRequestToken = 0;

	function toYMD(date: Date): string {
		const y = date.getFullYear();
		const m = String(date.getMonth() + 1).padStart(2, '0');
		const d = String(date.getDate()).padStart(2, '0');
		return `${y}-${m}-${d}`;
	}

	function parseYMD(value: string): Date | null {
		const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
		if (!match) return null;
		const y = Number(match[1]);
		const m = Number(match[2]);
		const d = Number(match[3]);
		const date = new Date(y, m - 1, d);
		if (date.getFullYear() !== y || date.getMonth() !== m - 1 || date.getDate() !== d) {
			return null;
		}
		return date;
	}

	function isPresetDays(value: number): value is PresetDays {
		return PRESET_DAYS.includes(value as PresetDays);
	}

	function deriveRangeFromSearch(searchParams: URLSearchParams): RangeSelection {
		const fromRaw = (searchParams.get('from') ?? '').trim();
		const toRaw = (searchParams.get('to') ?? '').trim();
		const fromDate = parseYMD(fromRaw);
		const toDate = parseYMD(toRaw);

		if (fromDate && toDate) {
			const fromMs = fromDate.getTime();
			const toMs = toDate.getTime();
			if (fromMs <= toMs) {
				return {
					mode: 'custom',
					presetDays: DEFAULT_PRESET_DAYS,
					startDate: fromRaw,
					endDate: toRaw
				};
			}
			return {
				mode: 'custom',
				presetDays: DEFAULT_PRESET_DAYS,
				startDate: toRaw,
				endDate: fromRaw
			};
		}

		const rangeRaw = (searchParams.get('range') ?? '').trim().toLowerCase();
		const parsedDays = Number.parseInt(rangeRaw.replace(/d$/, ''), 10);
		const presetDays = isPresetDays(parsedDays) ? parsedDays : DEFAULT_PRESET_DAYS;
		const now = referenceNowDate();
		const endDate = new Date(now.getFullYear(), now.getMonth(), now.getDate());
		const startDate = new Date(
			endDate.getFullYear(),
			endDate.getMonth(),
			endDate.getDate() - (presetDays - 1)
		);
		return {
			mode: 'preset',
			presetDays,
			startDate: toYMD(startDate),
			endDate: toYMD(endDate)
		};
	}

	function localDayStartMs(value: string): number {
		const date = parseYMD(value);
		if (!date) return 0;
		return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
	}

	function updateUrl(updates: Record<string, string | null>) {
		if (!projectId) return;
		const params = new SvelteURLSearchParams(page.url.searchParams);
		for (const [key, value] of Object.entries(updates)) {
			if (value === null || value === '') {
				params.delete(key);
			} else {
				params.set(key, value);
			}
		}
		const query = params.toString();
		void goto(
			resolve(`/projects/${encodeURIComponent(projectId)}/insights${query ? `?${query}` : ''}`),
			{
				replaceState: true,
				noScroll: true,
				keepFocus: true
			}
		);
	}

	function selectPreset(days: PresetDays) {
		updateUrl({ range: `${days}d`, from: null, to: null });
	}

	function presetLabel(days: PresetDays): string {
		if (days === 365) return 'last 1 year';
		return `last ${days} days`;
	}

	function setCustomRange(nextFrom: string, nextTo: string) {
		const fromDate = parseYMD(nextFrom);
		const toDate = parseYMD(nextTo);
		if (!fromDate || !toDate) return;

		let from = nextFrom;
		let to = nextTo;
		if (fromDate.getTime() > toDate.getTime()) {
			from = nextTo;
			to = nextFrom;
		}

		updateUrl({ range: null, from, to });
	}

	const selectedRange = $derived.by(() => deriveRangeFromSearch(page.url.searchParams));
	const selectedPresetValue = $derived.by(() =>
		selectedRange.mode === 'custom' ? 'custom' : String(selectedRange.presetDays)
	);
	const requestRange = $derived.by(() => {
		const startMs = localDayStartMs(selectedRange.startDate);
		const endInclusiveMs = localDayStartMs(selectedRange.endDate);
		const endExclusiveMs = endInclusiveMs + DAY_IN_MS;
		const dayCount = Math.floor((endInclusiveMs - startMs) / DAY_IN_MS) + 1;
		return {
			startMs,
			endExclusiveMs,
			dailyWindowDays: Math.max(1, dayCount),
			dailyWindowEndMs: endInclusiveMs
		};
	});

	$effect(() => {
		if (!projectId) return;
		const key = `${projectId}:${requestRange.startMs}:${requestRange.endExclusiveMs}:${requestRange.dailyWindowDays}:${requestRange.dailyWindowEndMs}`;
		if (key === lastLoadKey) return;
		lastLoadKey = key;
		const myToken = ++requestToken;
		loading = true;
		error = null;
		void listProjectCommitsPage(
			projectId,
			1,
			'',
			1,
			'@me+agents',
			'',
			'',
			requestRange.startMs,
			requestRange.endExclusiveMs,
			requestRange.dailyWindowDays,
			requestRange.dailyWindowEndMs
		)
			.then((resp) => {
				if (myToken !== requestToken) return;
				dailySummary = resp.dailySummary ?? [];
				branch = resp.branch;
			})
			.catch((e) => {
				if (myToken !== requestToken) return;
				error = e instanceof Error ? e.message : 'Failed to load insights';
				dailySummary = [];
			})
			.finally(() => {
				if (myToken !== requestToken) return;
				loading = false;
			});
	});

	$effect(() => {
		if (!projectId) return;
		const key = `activity:${projectId}:${requestRange.startMs}:${requestRange.endExclusiveMs}:${countChildConversationsSeparately}`;
		if (key === lastActivityLoadKey) return;
		lastActivityLoadKey = key;
		const myToken = ++activityRequestToken;
		// activityLoading = true;
		activityError = null;
		void getProjectDailyActivity(
			projectId,
			requestRange.startMs,
			requestRange.endExclusiveMs,
			countChildConversationsSeparately
		)
			.then((rows) => {
				if (myToken !== activityRequestToken) return;
				dailyActivity = rows ?? [];
			})
			.catch((e) => {
				if (myToken !== activityRequestToken) return;
				activityError = e instanceof Error ? e.message : 'Failed to load activity';
				dailyActivity = [];
			})
			.finally(() => {
				if (myToken !== activityRequestToken) return;
				activityLoading = false;
			});
	});

	$effect(() => {
		if (!projectId) return;
		const key = `ratings:${projectId}:${requestRange.startMs}:${requestRange.endExclusiveMs}`;
		if (key === lastRatingsLoadKey) return;
		lastRatingsLoadKey = key;
		const myToken = ++ratingsRequestToken;
		ratingsLoading = true;
		ratingsError = null;
		void getProjectRatingsByAgent(projectId, requestRange.startMs, requestRange.endExclusiveMs)
			.then((rows) => {
				if (myToken !== ratingsRequestToken) return;
				ratingsByAgent = rows ?? [];
			})
			.catch((e) => {
				if (myToken !== ratingsRequestToken) return;
				ratingsError = e instanceof Error ? e.message : 'Failed to load ratings';
				ratingsByAgent = [];
			})
			.finally(() => {
				if (myToken !== ratingsRequestToken) return;
				ratingsLoading = false;
			});
	});

	$effect(() => {
		if (!projectId) return;
		const key = `ftcov:${projectId}:${requestRange.startMs}:${requestRange.endExclusiveMs}`;
		if (key === lastFileTypeCoverageLoadKey) return;
		lastFileTypeCoverageLoadKey = key;
		const myToken = ++fileTypeCoverageRequestToken;
		fileTypeCoverageLoading = true;
		fileTypeCoverageError = null;
		void getProjectFileTypeCoverage(projectId, requestRange.startMs, requestRange.endExclusiveMs)
			.then((rows) => {
				if (myToken !== fileTypeCoverageRequestToken) return;
				fileTypeCoverage = rows ?? [];
			})
			.catch((e) => {
				if (myToken !== fileTypeCoverageRequestToken) return;
				fileTypeCoverageError =
					e instanceof Error ? e.message : 'Failed to load file type coverage';
				fileTypeCoverage = [];
			})
			.finally(() => {
				if (myToken !== fileTypeCoverageRequestToken) return;
				fileTypeCoverageLoading = false;
			});
	});
</script>

<div class="outer">
	<div class="insights-page">
		<div class="filters">
			<div>
				<h1>{navStore.projectName}</h1>
				<span class="branch-label"><Icon name="branch" width="12px" />{branch ? branch : '–'}</span>
			</div>

			<div class="date-range">
				<label class="date-picker">
					<input
						type="date"
						value={selectedRange.startDate}
						onchange={(e) =>
							setCustomRange((e.currentTarget as HTMLInputElement).value, selectedRange.endDate)}
					/>
				</label>
				<label class="date-picker">
					<span>–</span>
					<input
						type="date"
						value={selectedRange.endDate}
						onchange={(e) =>
							setCustomRange(selectedRange.startDate, (e.currentTarget as HTMLInputElement).value)}
					/>
				</label>
			</div>

			<label class="preset-range">
				<select
					aria-label="Preset date range"
					onchange={(e) => {
						const value = (e.currentTarget as HTMLSelectElement).value;
						const days = Number(value);
						if (isPresetDays(days)) {
							selectPreset(days);
						}
					}}
				>
					{#if selectedRange.mode === 'custom'}
						<option value="custom" selected={selectedPresetValue === 'custom'}>custom range</option>
					{/if}
					{#each PRESET_DAYS as days (days)}
						<option value={days} selected={String(days) === selectedPresetValue}>
							{presetLabel(days)}
						</option>
					{/each}
				</select>
			</label>
		</div>

		<div class="charts">
			<div class="chart-panel" style:min-height="174px">
				<h2 class="chart-heading">Agent Attribution</h2>
				{#if loading}
					<p class="status">Loading insights...</p>
				{:else if error}
					<p class="status error">{error}</p>
				{:else if dailySummary.length === 0}
					<p class="status">No commit data in the selected range.</p>
				{:else}
					<DailyCommitsChart
						{dailySummary}
						{branch}
						{projectId}
						enableDateSelection={false}
						showMoreLink={false}
						height={130}
					/>
				{/if}
			</div>

			<div class="chart-panel" style:min-height="171px">
				<h2 class="chart-heading">Conversations</h2>
				{#if activityLoading}
					<p class="status">Loading activity...</p>
				{:else if activityError}
					<p class="status error">{activityError}</p>
				{:else if dailyActivity.length === 0}
					<p class="status">No activity data in the selected range.</p>
				{:else}
					<DailyActivityChart {dailyActivity} height={130} />
				{/if}
			</div>

			<div class="chart-panel">
				<h2 class="chart-heading">Ratings by Agent</h2>
				{#if ratingsLoading}
					<p class="status">Loading ratings...</p>
				{:else if ratingsError}
					<p class="status error">{ratingsError}</p>
				{:else}
					<RatingsByAgentChart agents={ratingsByAgent} />
				{/if}
			</div>

			<div class="chart-panel">
				<h2 class="chart-heading">Agent Attribution by File Type</h2>
				{#if fileTypeCoverageLoading}
					<p class="status">Loading file type coverage...</p>
				{:else if fileTypeCoverageError}
					<p class="status error">{fileTypeCoverageError}</p>
				{:else}
					<FileTypeCoverageChart data={fileTypeCoverage} />
				{/if}
			</div>
		</div>
	</div>
</div>

<style>
	.outer {
		flex: 1;
	}

	.insights-page {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin: 0;

		max-width: 1100px;
		margin: 0 auto;

		background: var(--color-background-content);
		border-radius: var(--content-section-border-radius);
		border: 0.5px solid var(--color-divider);
		box-sizing: border-box;
		margin: 1.5rem auto;
		width: 100%;
	}

	@media (max-width: 1100px) {
		.insights-page {
			border-width: 0 0 0.5px 0;
			margin: 0 auto;
			border-radius: 0;
		}
	}

	.filters {
		box-sizing: border-box;
		gap: 1.5rem;
		padding: 1rem;
		border-bottom: 0.5px solid var(--color-divider);
		justify-content: space-between;
		min-height: 66px;
	}

	.filters > * {
		flex: 1 1 0;
	}

	.filters h1 {
		font-size: 1.4rem;
		margin: 0;
		padding: 0;
	}

	.branch-label {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 1rem;
		color: var(--color-text-secondary);
		margin-top: 0.15rem;
	}

	.filters .date-range {
		display: flex;
		align-items: center;
		gap: 0.8rem;
		justify-content: center;
	}

	.filters .date-picker {
		display: flex;
		align-items: center;
		gap: 0.8rem;
		font-size: 1.2rem;
		font-weight: 600;
	}

	.filters .date-picker input[type='date'] {
		font-size: 1.2rem;
		padding: 0.5rem 1rem;
	}

	.preset-range {
		display: flex;
		margin-left: auto;
		justify-content: flex-end;
	}

	.preset-range select {
		font-size: 1rem;
		font-weight: 600;
		padding: 0.45rem 2.2rem 0.45rem 0.8rem;
		border: 0.5px solid var(--color-divider);
		border-radius: 6px;
		background: var(--color-background-surface);
		color: var(--color-text-secondary);
		cursor: pointer;
		line-height: 1.2;
	}

	.charts {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.5rem 0;
	}

	.chart-panel {
		padding: 0 1rem 0.5rem 1rem;
	}

	.chart-heading {
		margin: 0rem 0 0.75rem 0;
		font-size: 1rem;
		font-weight: 600;
		color: var(--color-text-secondary);
	}

	.status {
		margin: 1rem 0;
		color: var(--color-text-secondary);
	}

	.status.error {
		color: var(--color-danger, #b00020);
	}

	@media (max-width: 980px) {
		.filters {
			flex-wrap: wrap;
		}

		.preset-range {
			flex-wrap: wrap;
			margin-left: 0;
		}
	}
</style>
