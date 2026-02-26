<script lang="ts">
	import { resolve } from '$app/paths';
	import { tick } from 'svelte';
	import { SvelteSet, SvelteMap } from 'svelte/reactivity';
	import { agentColor, MANUAL_COLOR } from './chartColors';
	import Popover from '$lib/components/Popover.svelte';
	import Icon from '$lib/Icon.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import type { DailyCommitSummary } from '$lib/types';

	interface Props {
		dailySummary: DailyCommitSummary[];
		branch: string;
		projectId?: string;
		compact?: boolean;
		selectedDate?: string | null;
		onDateSelect?: (date: string | null) => void;
	}

	let {
		dailySummary,
		branch,
		projectId,
		compact = false,
		selectedDate = null,
		onDateSelect
	}: Props = $props();
	let scaleByLines = $derived(settingsStore.commitsChartScaleByLines);
	const popoverId = `dc-menu-${Math.random().toString(36).slice(2, 8)}`;
	let menuBtn: HTMLButtonElement | undefined;
	let menuBody: HTMLDivElement | undefined;
	let scrollWrap: HTMLDivElement | undefined;
	let lastAutoScrollKey = '';

	interface Segment {
		key: string;
		percent: number;
		color: string;
		lines: number;
	}

	interface ColumnDay {
		date: string;
		total: number;
		segments: Segment[];
		summary: DailyCommitSummary;
	}

	interface KeySegment {
		key: string;
		percent: number;
		color: string;
	}

	let agentNames = $derived.by(() => {
		const names = new SvelteSet<string>();
		for (const day of dailySummary) {
			if (day.agentSegments) {
				for (const seg of day.agentSegments) {
					names.add(seg.agent);
				}
			}
		}
		return [...names].sort();
	});

	let agentColorMap = $derived.by(() => {
		const map = new SvelteMap<string, string>();
		for (let i = 0; i < agentNames.length; i++) {
			map.set(agentNames[i], agentColor(agentNames[i], i));
		}
		map.set('manual', MANUAL_COLOR);
		return map;
	});

	let columns = $derived.by((): ColumnDay[] => {
		return dailySummary.map((day) => {
			if (day.linesTotal === 0) {
				return { date: day.date, total: 0, segments: [], summary: day };
			}

			const agentLines: Record<string, number> = {};
			let agentLinesSum = 0;
			if (day.agentSegments) {
				for (const seg of day.agentSegments) {
					agentLines[seg.agent] = seg.linesFromAgent;
					agentLinesSum += seg.linesFromAgent;
				}
			}
			const manualLines = day.linesTotal - agentLinesSum;

			const segments: Segment[] = [];
			for (const name of agentNames) {
				const lines = agentLines[name] ?? 0;
				if (lines > 0) {
					segments.push({
						key: name,
						percent: (lines / day.linesTotal) * 100,
						color: agentColorMap.get(name) ?? '#999',
						lines
					});
				}
			}
			if (manualLines > 0) {
				segments.push({
					key: 'manual',
					percent: (manualLines / day.linesTotal) * 100,
					color: agentColorMap.get('manual') ?? '#656565',
					lines: manualLines
				});
			}

			return { date: day.date, total: day.linesTotal, segments, summary: day };
		});
	});

	let maxDayLines = $derived.by(() => {
		let max = 0;
		for (const col of columns) {
			if (col.total > max) max = col.total;
		}
		return max;
	});

	function barScale(col: ColumnDay): number {
		if (!scaleByLines || maxDayLines <= 0) return 1;
		return col.total / maxDayLines;
	}

	const historyTotalLines = $derived.by(() =>
		dailySummary.reduce((sum, day) => sum + Math.max(0, day.linesTotal), 0)
	);

	const historyLinesByAgent = $derived.by(() => {
		const byAgent = new SvelteMap<string, number>();
		for (const day of dailySummary) {
			for (const seg of day.agentSegments ?? []) {
				byAgent.set(seg.agent, (byAgent.get(seg.agent) ?? 0) + seg.linesFromAgent);
			}
		}
		return byAgent;
	});

	const historyAgentLines = $derived.by(() => {
		let total = 0;
		for (const lines of historyLinesByAgent.values()) total += lines;
		return total;
	});

	const historyManualLines = $derived.by(() => Math.max(0, historyTotalLines - historyAgentLines));

	const historyKeySegments = $derived.by((): KeySegment[] => {
		if (historyTotalLines <= 0) return [];
		const segments: KeySegment[] = [];
		for (const name of agentNames) {
			const lines = historyLinesByAgent.get(name) ?? 0;
			if (lines > 0) {
				segments.push({
					key: name,
					percent: (lines / historyTotalLines) * 100,
					color: agentColorMap.get(name) ?? '#999'
				});
			}
		}
		if (historyManualLines > 0) {
			segments.push({
				key: 'manual',
				percent: (historyManualLines / historyTotalLines) * 100,
				color: agentColorMap.get('manual') ?? '#656565'
			});
		}
		return segments;
	});

	const historyAgentPercent = $derived.by(() => {
		if (historyTotalLines <= 0) return 0;
		return (historyAgentLines / historyTotalLines) * 100;
	});

	function formatDateLong(dateStr: string): string {
		const [y, m, d] = dateStr.split('-').map(Number);
		const date = new Date(y, m - 1, d);
		return date.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' });
	}

	function formatDateLabel(dateStr: string): string {
		const [y, m, d] = dateStr.split('-').map(Number);
		if (d === 1) {
			const date = new Date(y, m - 1, d);
			return date.toLocaleDateString(undefined, { month: 'short' });
		}
		return String(d);
	}

	function formatKeyPercent(percent: number): string {
		if (percent < 1 || (percent > 99 && percent < 100)) {
			return `${percent.toFixed(1)}%`;
		}
		return `${Math.round(percent)}%`;
	}

	const autoScrollKey = $derived.by(() => {
		const first = columns[0]?.date ?? '';
		const last = columns[columns.length - 1]?.date ?? '';
		return `${branch}:${columns.length}:${first}:${last}`;
	});

	$effect(() => {
		const key = autoScrollKey;
		if (!scrollWrap || !key || key === lastAutoScrollKey) return;
		lastAutoScrollKey = key;
		void tick().then(() => {
			if (!scrollWrap) return;
			scrollWrap.scrollLeft = scrollWrap.scrollWidth;
		});
	});

	function positionMenu() {
		if (!menuBtn || !menuBody) return;
		const rect = menuBtn.getBoundingClientRect();
		menuBody.style.top = `${rect.bottom + 4}px`;
		menuBody.style.left = `${rect.left}px`;
	}
</script>

<div class="dc-layout" class:compact>
	<div class="dc-chart-area">
		<button
			class="dc-menu-btn"
			bind:this={menuBtn}
			popovertarget={popoverId}
			aria-label="Chart options"
		>
			<Icon name="chevronRight" width="12px" />
		</button>
		{#if selectedDate && onDateSelect}
			<button
				class="dc-clear-btn"
				aria-label="Clear date filter"
				onclick={() => onDateSelect?.(null)}>Clear Selection</button
			>
		{/if}
		<div
			id={popoverId}
			class="dc-menu-body"
			bind:this={menuBody}
			popover="auto"
			onbeforetoggle={(e: ToggleEvent) => {
				if (e.newState === 'open') positionMenu();
			}}
		>
			<label class="dc-menu-option">
				<input
					type="checkbox"
					checked={settingsStore.commitsChartScaleByLines}
					onchange={(e) =>
						(settingsStore.commitsChartScaleByLines = (
							e.currentTarget as HTMLInputElement
						).checked)}
				/>
				Scale bars by line count
			</label>
		</div>
		<div class="dc-wrap" bind:this={scrollWrap}>
			<div class="dc-chart">
				{#each columns as col (col.date)}
					<!-- svelte-ignore a11y_click_events_have_key_events -->
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div
						class="dc-col"
						class:dc-col-selected={selectedDate === col.date}
						onclick={() => {
							if (onDateSelect) {
								onDateSelect(selectedDate === col.date ? null : col.date);
							}
						}}
					>
						<div class="dc-bar-area">
							{#if col.total > 0}
								<Popover position="below" width="200px" padding="0" fixed={true}>
									<div class="dc-bar">
										{#each col.segments as seg (seg.key)}
											<div
												class="dc-seg"
												style="height:{seg.percent * barScale(col)}%;background:{seg.color}"
											></div>
										{/each}
									</div>
									{#snippet popover()}
										<div class="dc-popover">
											<div class="dc-popover-summary">
												<div class="dc-popover-date">{formatDateLong(col.date)}</div>
												<div class="dc-popover-lines">
													{col.total} line{col.total !== 1 ? 's' : ''}
												</div>
												<div class="dc-popover-breakdown">
													{#each col.segments as seg (seg.key)}
														<div class="dc-popover-row">
															<span class="dc-swatch" style="background:{seg.color}"></span>
															<span class="dc-popover-name">{seg.key}</span>
															<span class="dc-popover-pct">{seg.percent.toFixed(1)}%</span>
														</div>
													{/each}
												</div>
											</div>
											{#if col.summary.commits.length > 0}
												<div class="dc-popover-commits">
													{#each col.summary.commits as c (c.commitHash)}
														<a
															href={resolve(
																`/projects/${encodeURIComponent(c.projectId)}/commits/${encodeURIComponent(branch)}/${encodeURIComponent(c.commitHash)}`
															)}
															class="dc-commit-link"
														>
															{c.subject || c.commitHash.slice(0, 8)}
														</a>
													{/each}
												</div>
											{/if}
										</div>
									{/snippet}
								</Popover>
							{:else}
								<div class="dc-bar dc-bar-empty"></div>
							{/if}
						</div>
						<div class="dc-date">
							{formatDateLabel(col.date)}
						</div>
					</div>
				{/each}
			</div>
		</div>
	</div>
	<div class="dc-side">
		{#if historyKeySegments.length > 0}
			<div class="dc-key info-box">
				{#each historyKeySegments as seg (seg.key)}
					<span class="dc-key-item">
						<span class="dc-swatch" style="background:{seg.color}"></span>
						<span class="dc-key-label">{seg.key}</span>
						<span class="dc-key-pct">{formatKeyPercent(seg.percent)}</span>
					</span>
				{/each}
			</div>
		{/if}
		<div class="dc-history-agent info-box">
			<div class="title">{Math.round(historyAgentPercent)}% by agents</div>
			<div class="title" style:font-size="0.9rem">
				{Math.round(historyTotalLines * (historyAgentPercent / 100)).toLocaleString()} lines by agents
			</div>
			<div class="subtitle">last {columns.length} day{columns.length === 1 ? '' : 's'}</div>
			{#if projectId}
				<div class="subtitle">
					<a
						class="dc-more-link"
						href={resolve(`/projects/${encodeURIComponent(projectId)}/insights`)}>more...</a
					>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.dc-layout {
		display: flex;
		align-items: flex-start;
		gap: 1rem;
	}

	.dc-chart-area {
		flex: 1;
		position: relative;
		min-width: 0;
	}

	.dc-menu-btn {
		position: absolute;
		top: 6px;
		left: 6px;
		z-index: 2;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 18px;
		height: 18px;
		padding: 0;
		border: 0.5px solid var(--color-divider);
		border-radius: 4px;
		background: var(--color-background-elevated);
		cursor: pointer;
		opacity: 0;
		transition: opacity 0.15s;
	}

	.dc-menu-btn :global(.icon) {
		transform: rotate(90deg);
	}

	.dc-chart-area:hover .dc-menu-btn,
	.dc-chart-area:has(:popover-open) .dc-menu-btn {
		opacity: 1;
	}

	.dc-clear-btn {
		position: absolute;
		top: 6px;
		left: 28px;
		z-index: 2;
		display: flex;
		align-items: center;
		justify-content: center;
		height: 18px;
		padding: 0 0.5em;
		border: 0.5px solid var(--color-divider);
		border-radius: 4px;
		background: var(--color-background-elevated);
		cursor: pointer;
		font-size: 12px;
		line-height: 1;
		color: var(--color-text-secondary);
	}

	.dc-clear-btn:hover {
		background: var(--color-background-surface);
	}

	.dc-menu-body {
		position: fixed;
		inset: unset;
		margin: 0;
		background: var(--color-background-surface);
		border: 0.5px solid var(--color-divider);
		border-radius: 5px;
		box-shadow: 0 2px 8px var(--color-popover-shadow);
		padding: 0.5rem 1rem 0.5rem 0.7rem;
		white-space: nowrap;
	}

	.dc-menu-option {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.85rem;
		color: var(--color-text);
		cursor: pointer;
		user-select: none;
	}

	.dc-wrap {
		margin-top: -1px;
		max-width: 100%;
		overflow-x: auto;
		overflow-y: hidden;
		padding-bottom: 0.25rem;
		padding-top: 1px;
	}

	.dc-side {
		box-sizing: border-box;
		align-self: stretch;
		display: flex;
		flex-direction: row;
		gap: 1rem;
		align-items: stretch;
		padding-bottom: 0.5rem;
	}

	.dc-key {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.dc-key-item {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.9rem;
		color: var(--color-text-secondary);
	}

	.dc-key-label {
		max-width: 110px;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.dc-key-pct {
		color: var(--color-text-tertiary);
		font-variant-numeric: tabular-nums;
		margin-left: auto;
	}

	.dc-history-agent {
		font-size: 0.88rem;
		color: var(--color-text);
		font-variant-numeric: tabular-nums;
	}

	.dc-history-agent .title {
		font-size: 1.4em;
		font-weight: 600;
	}

	.dc-history-agent .subtitle {
		font-size: 1em;
		opacity: 0.8;
	}

	.dc-more-link {
		text-decoration: none;
		color: var(--color-link-body);
	}

	.dc-more-link:hover {
		text-decoration: underline;
	}

	.dc-chart {
		display: inline-flex;
		gap: 1px;
		align-items: stretch;
		min-width: max-content;
	}

	.dc-col {
		flex: 0 0 10px;
		min-width: 18px;
		display: flex;
		flex-direction: column;
		cursor: pointer;
		border-radius: 3px;
	}

	.dc-col-selected {
		background: var(--color-accent-muted, rgba(59, 130, 246, 0.15));
		outline: 1.5px solid var(--color-accent, #3b82f6);
		outline-offset: -1px;
		position: relative;
	}

	.dc-col-selected::after {
		inset: 0px;
		content: '';
		position: absolute;
		border-radius: 2px;
		outline: 2px solid var(--accent-color);
		outline-offset: -1px;
	}

	.dc-bar-area {
		height: 114px;
	}

	/* Popover wrapper must fill bar area */
	.dc-bar-area :global(.popover-wrap) {
		height: 100%;
	}

	.dc-bar {
		width: 100%;
		height: 100%;
		border-radius: 2px;
		overflow: hidden;
		display: flex;
		flex-direction: column-reverse;
		gap: 1px;
		background: var(--color-background-empty, #f0f0f0);
	}

	.dc-bar-empty {
		background: var(--color-background-empty, #f0f0f0);
	}

	.dc-seg {
		width: 100%;
		min-height: 0;
		flex-shrink: 0;
	}

	.dc-date {
		font-size: 0.65rem;
		color: var(--color-text-faded);
		text-align: center;
		padding-top: 0.25rem;
		height: 1rem;
		white-space: nowrap;
	}

	.dc-popover {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.dc-popover-summary {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 1rem 1rem 0.5rem 1rem;
	}

	.dc-popover-date {
		font-weight: 600;
		font-size: 0.85rem;
		color: var(--color-text-strong);
	}

	.dc-popover-lines {
		font-size: 0.8rem;
		color: var(--color-text-tertiary);
	}

	.dc-popover-breakdown {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.dc-popover-row {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.85rem;
	}

	.dc-swatch {
		display: inline-block;
		width: 10px;
		height: 10px;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.dc-popover-name {
		color: var(--color-text-strong);
	}

	.dc-popover-pct {
		color: var(--color-text-tertiary);
		margin-left: auto;
		padding-left: 0.75rem;
		font-variant-numeric: tabular-nums;
	}

	.dc-popover-commits {
		border-top: 1px solid var(--color-border-light);
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		max-height: 150px;
		overflow-y: auto;
		padding: 1rem;
	}

	.dc-commit-link {
		font-size: 0.8rem;
		color: var(--color-link-body);
		text-decoration: none;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 250px;
		display: block;
		min-height: 1.2em;
	}

	.dc-commit-link:hover {
		text-decoration: underline;
	}

	@media (max-width: 780px) {
		.dc-layout {
			flex-direction: column;
		}

		.dc-side {
			min-width: 0;
		}

		.dc-key {
			flex-direction: row;
			flex-wrap: wrap;
			gap: 0.5rem 0.75rem;
		}
	}

	/* Compact mode overrides */
	.dc-layout.compact .dc-bar-area {
		height: 60px;
	}

	.dc-layout.compact .dc-side {
		display: none;
	}

	.dc-layout.compact .dc-date {
		display: none;
	}

	.dc-layout.compact .dc-menu-btn {
		display: none;
	}

	.dc-layout.compact .dc-wrap {
		padding-bottom: 0;
	}
</style>
