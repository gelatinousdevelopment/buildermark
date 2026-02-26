<script lang="ts">
	import { onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import { listProjectCommitsPage } from '$lib/api';
	import DailyCommitsChart from '$lib/charts/DailyCommitsChart.svelte';
	import type { DailyCommitSummary } from '$lib/types';
	import { navStore } from '$lib/stores/nav.svelte';
	import { layoutStore } from '$lib/stores/layout.svelte';

	layoutStore.hideContainer = true;
	onDestroy(() => {
		layoutStore.hideContainer = false;
	});

	const PRESET_DAYS = [7, 14, 30, 60, 90, 180, 365] as const;
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
		const now = new Date();
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
			'',
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
</script>

<div class="outer">
	<div class="insights-page">
		<div class="filters">
			<div><h1>{navStore.projectName}</h1></div>

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

			<div class="preset-range">
				{#each PRESET_DAYS as days (days)}
					<button
						type="button"
						class:active={selectedRange.mode === 'preset' && selectedRange.presetDays === days}
						onclick={() => selectPreset(days)}>{days}d</button
					>
				{/each}
			</div>
		</div>

		<div class="chart-panel">
			{#if loading}
				<p class="status">Loading insights...</p>
			{:else if error}
				<p class="status error">{error}</p>
			{:else if dailySummary.length === 0}
				<p class="status">No commit data in the selected range.</p>
			{:else}
				<DailyCommitsChart {dailySummary} {branch} {projectId} enableDateSelection={false}  />
			{/if}
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
		gap: 1rem;
		margin: 0;

		max-width: 1100px;
		margin: 0 auto;

		background: var(--color-background-content);
		border-radius: 10px;
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
		gap: 1.5rem;
		padding: 1rem;
		border-bottom: 0.5px solid var(--color-divider);
		justify-content: space-between;
	}

	.filters > div {
		flex: 1;
	}

	.filters h1 {
		margin: 0;
		font-size: 1.4rem;
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
		gap: 0rem;
		margin-left: auto;
		justify-content: flex-end;
	}

	.preset-range button {
		font-size: 1rem;
		font-weight: 600;
		padding: 0.35rem 0.6rem;
		border: none;
		border-radius: 4px;
		background: var(--color-background-surface);
		color: var(--color-text-secondary);
		cursor: pointer;
		line-height: 1.2;
	}

	.preset-range button:hover {
		background: var(--color-background-elevated);
		color: var(--color-text);
	}

	.preset-range button.active {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
	}

	.chart-panel {
		min-height: 10rem;
		padding: 0 1rem 0.5rem 1rem;
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
