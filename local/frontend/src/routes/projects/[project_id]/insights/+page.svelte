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
	import { toPng } from 'html-to-image';

	onMount(() => {
		layoutStore.hideContainer = true;
	});
	onDestroy(() => {
		layoutStore.hideContainer = false;
	});

	const PRESET_DAYS = [7, 14, 30, 45, 60, 90, 180, 365] as const;
	const DAY_IN_MS = 24 * 60 * 60 * 1000;
	const DEFAULT_PRESET_DAYS = 45;

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

	let showAgentAttribution = $derived(settingsStore.insightsShowAgentAttribution);
	let showConversations = $derived(settingsStore.insightsShowConversations);
	let showRatings = $derived(settingsStore.insightsShowRatings);
	let showFileTypeCoverage = $derived(settingsStore.insightsShowFileTypeCoverage);

	const sharePopoverId = `share-menu-${Math.random().toString(36).slice(2, 8)}`;
	let shareBtn: HTMLButtonElement | undefined = $state();
	let shareMenuBody: HTMLDivElement | undefined = $state();
	let outerEl: HTMLDivElement | undefined = $state();
	let capturing = $state(false);

	const hasRatingsData = $derived(
		ratingsByAgent.length > 0 && ratingsByAgent.some((a) => a.totalConversations > 0)
	);

	function positionShareMenu() {
		if (!shareBtn || !shareMenuBody) return;
		const rect = shareBtn.getBoundingClientRect();
		shareMenuBody.style.top = `${rect.bottom + 4}px`;
		shareMenuBody.style.right = `${window.innerWidth - rect.right}px`;
		shareMenuBody.style.left = 'unset';
	}

	function formatDateRange(): string {
		const start = parseYMD(selectedRange.startDate);
		const end = parseYMD(selectedRange.endDate);
		if (!start || !end) return `${selectedRange.startDate} \u2013 ${selectedRange.endDate}`;
		const fmt = (d: Date) =>
			d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
		return `${fmt(start)} \u2013 ${fmt(end)}`;
	}

	function fixSvgForCapture(root: HTMLElement) {
		const gridLines = root.querySelectorAll('.da-grid-line');
		const tickLabels = root.querySelectorAll('.da-tick-label');
		gridLines.forEach((el) => {
			const computed = getComputedStyle(el);
			el.setAttribute('stroke', computed.stroke || '#ccc');
			el.setAttribute('stroke-width', '1');
		});
		tickLabels.forEach((el) => {
			const computed = getComputedStyle(el);
			el.setAttribute('fill', computed.fill || '#999');
			el.setAttribute('font-size', '9');
		});
	}

	function restoreSvgAfterCapture(root: HTMLElement) {
		const gridLines = root.querySelectorAll('.da-grid-line');
		const tickLabels = root.querySelectorAll('.da-tick-label');
		gridLines.forEach((el) => {
			el.removeAttribute('stroke');
			el.removeAttribute('stroke-width');
		});
		tickLabels.forEach((el) => {
			el.removeAttribute('fill');
			el.removeAttribute('font-size');
		});
	}

	async function captureScreenshot(mode: 'download' | 'copy') {
		if (!outerEl || capturing) return;
		capturing = true;

		outerEl.classList.add('screenshot-mode');
		fixSvgForCapture(outerEl);

		try {
			await new Promise((r) => setTimeout(r, 150));

			const rect = outerEl.getBoundingClientRect();

			// Capture at 2× CSS resolution with integer pixel gaps.
			// 1px CSS gaps become 2px in the output — correct for 2× scale.
			const dataUrl = await toPng(outerEl, {
				width: rect.width,
				height: rect.height,
				pixelRatio: 2
			});

			if (mode === 'download') {
				const link = document.createElement('a');
				link.download = `insights-${navStore.projectName}-${selectedRange.startDate}-to-${selectedRange.endDate}.png`;
				link.href = dataUrl;
				link.click();
			} else {
				const img = new Image();
				img.src = dataUrl;
				await new Promise<void>((resolve, reject) => {
					img.onload = () => resolve();
					img.onerror = reject;
				});
				const canvas = document.createElement('canvas');
				canvas.width = img.width;
				canvas.height = img.height;
				const ctx = canvas.getContext('2d')!;
				ctx.drawImage(img, 0, 0);
				const blob = await new Promise<Blob | null>((resolve) =>
					canvas.toBlob(resolve, 'image/png')
				);
				if (blob) {
					await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
				}
			}
		} finally {
			restoreSvgAfterCapture(outerEl);
			outerEl.classList.remove('screenshot-mode');
			capturing = false;
		}
	}

	function downloadPng() {
		void captureScreenshot('download');
	}
	function copyToClipboard() {
		void captureScreenshot('copy');
	}
</script>

<div class="outer" bind:this={outerEl}>
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
				<span class="date-range-text">{formatDateRange()}</span>
			</div>

			<div class="right-section">
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
							<option value="custom" selected={selectedPresetValue === 'custom'}
								>custom range</option
							>
						{/if}
						{#each PRESET_DAYS as days (days)}
							<option value={days} selected={String(days) === selectedPresetValue}>
								{presetLabel(days)}
							</option>
						{/each}
					</select>
				</label>

				<div class="screenshot-branding">
					<span class="screenshot-branding-label">Calculated by</span>
					<span class="screenshot-branding-name">Buildermark</span>
				</div>

				<button
					class="share-btn"
					bind:this={shareBtn}
					popovertarget={sharePopoverId}
					aria-label="Share insights"
				>
					<Icon name="share" width="15px" />
				</button>

				<div
					id={sharePopoverId}
					class="share-menu-body"
					bind:this={shareMenuBody}
					popover="auto"
					onbeforetoggle={(e: ToggleEvent) => {
						if (e.newState === 'open') positionShareMenu();
					}}
				>
					<div class="share-menu-section">
						<div class="share-menu-heading">Show Charts</div>
						<label class="share-menu-option">
							<input
								type="checkbox"
								checked={showAgentAttribution}
								onchange={(e) =>
									(settingsStore.insightsShowAgentAttribution = (
										e.currentTarget as HTMLInputElement
									).checked)}
							/>
							Agent Attribution
						</label>
						<label class="share-menu-option">
							<input
								type="checkbox"
								checked={showConversations}
								onchange={(e) =>
									(settingsStore.insightsShowConversations = (
										e.currentTarget as HTMLInputElement
									).checked)}
							/>
							Conversations
						</label>
						<label class="share-menu-option" class:disabled={!hasRatingsData}>
							<input
								type="checkbox"
								checked={showRatings}
								disabled={!hasRatingsData}
								onchange={(e) =>
									(settingsStore.insightsShowRatings = (
										e.currentTarget as HTMLInputElement
									).checked)}
							/>
							Ratings by Agent
						</label>
						<label class="share-menu-option">
							<input
								type="checkbox"
								checked={showFileTypeCoverage}
								onchange={(e) =>
									(settingsStore.insightsShowFileTypeCoverage = (
										e.currentTarget as HTMLInputElement
									).checked)}
							/>
							Agent Attribution by File Type
						</label>
					</div>
					<div class="share-menu-divider"></div>
					<div class="share-menu-section">
						<div class="share-menu-heading">Screenshot</div>
						<div class="share-menu-actions">
							<button class="share-action-btn" onclick={downloadPng} disabled={capturing}>
								{capturing ? 'Capturing...' : 'Download PNG'}
							</button>
							<button class="share-action-btn" onclick={copyToClipboard} disabled={capturing}>
								Copy
							</button>
						</div>
					</div>
				</div>
			</div>
		</div>

		<div class="charts">
			{#if showAgentAttribution}
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
			{/if}

			{#if showConversations}
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
			{/if}

			{#if showRatings && hasRatingsData}
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
			{/if}

			{#if showFileTypeCoverage}
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
		flex: 1;
	}

	.filters .date-range {
		display: flex;
		align-items: center;
		gap: 0.8rem;
		justify-content: center;
		flex: 1;
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

	.filters .right-section {
		align-items: center;
		display: flex;
		flex-direction: row;
		flex: 1;
		gap: 0.5rem;
		justify-content: flex-end;
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

	.share-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 34px;
		height: 28px;
		padding: 0;
		border: 0.5px solid var(--color-divider);
		border-radius: 6px;
		background: var(--color-background-surface);
		cursor: pointer;
		flex: 0 0 auto;
		margin-left: 0.5rem;
	}

	.share-btn:hover {
		background: var(--color-background-elevated);
	}

	.share-menu-body {
		position: fixed;
		inset: unset;
		margin: 0;
		background: var(--color-background-surface);
		border: 0.5px solid var(--color-divider);
		border-radius: 5px;
		box-shadow: 0 2px 8px var(--color-popover-shadow);
		padding: 0.75rem 1rem;
		white-space: nowrap;
		min-width: 220px;
	}

	.share-menu-heading {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.03em;
		margin-bottom: 0.4rem;
	}

	.share-menu-option {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.85rem;
		color: var(--color-text);
		cursor: pointer;
		user-select: none;
		padding: 0.15rem 0;
	}

	.share-menu-option.disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.share-menu-divider {
		border-top: 0.5px solid var(--color-divider);
		margin: 0.5rem 0;
	}

	.share-menu-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}

	.share-action-btn {
		flex: 1;
		padding: 0.4rem 0.8rem;
		font-size: 0.85rem;
		font-weight: 600;
		border: 0.5px solid var(--color-divider);
		border-radius: 5px;
		background: var(--color-background-elevated);
		color: var(--color-text);
		cursor: pointer;
	}

	.share-action-btn:hover:not(:disabled) {
		background: var(--color-background-surface);
	}

	.share-action-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.date-range-text {
		display: none;
		font-size: 1.6rem;
		font-weight: 600;
		color: var(--color-text);
		white-space: nowrap;
	}

	.screenshot-branding {
		display: none;
		flex-direction: column;
		align-items: flex-end;
		justify-content: center;
		line-height: 1.2;
	}

	.screenshot-branding-label {
		font-size: 0.8rem;
		color: var(--color-text-tertiary);
	}

	.screenshot-branding-name {
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--color-text-secondary);
	}

	/* Screenshot mode: padding/background on .outer, layout untouched on .insights-page */
	:global(.outer.screenshot-mode) {
		padding: 60px;
		background: var(--color-background-content);
		width: fit-content;
	}

	:global(.outer.screenshot-mode) .insights-page {
		border: none !important;
		border-radius: 0 !important;
		margin: 0 !important;
	}

	:global(.outer.screenshot-mode) .date-picker {
		display: none !important;
	}

	:global(.outer.screenshot-mode) .date-range-text {
		display: inline !important;
	}

	:global(.outer.screenshot-mode) .preset-range {
		display: none !important;
	}

	:global(.outer.screenshot-mode) .share-btn {
		display: none !important;
	}

	:global(.outer.screenshot-mode) .share-menu-body {
		display: none !important;
	}

	:global(.outer.screenshot-mode) .screenshot-branding {
		display: flex !important;
		flex: 1 1 0;
	}

	:global(.outer.screenshot-mode) .filters {
		border-bottom: none !important;
		padding: 1rem 0 !important;
		margin: 0 1rem !important;
		border-bottom: 0.5px solid var(--color-divider) !important;
	}

	:global(.outer.screenshot-mode) .chart-panel {
		min-height: 0 !important;
	}

	/* Hide non-interactive elements in screenshot */
	:global(.outer.screenshot-mode .da-details-icon) {
		display: none !important;
	}

	:global(.outer.screenshot-mode .toggle-btn) {
		display: none !important;
	}

	/* Force integer pixel gaps for clean screenshot rendering */
	:global(.outer.screenshot-mode .dc-chart) {
		column-gap: 1px !important;
	}

	:global(.outer.screenshot-mode .dc-bar) {
		gap: 1px !important;
	}

	:global(.outer.screenshot-mode .coverage-table) {
		--bar-gap: 1px !important;
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
