<script lang="ts">
	import { tick } from 'svelte';
	import Popover from '$lib/components/Popover.svelte';
	import type { DailyActivityRow } from '$lib/types';

	interface Props {
		dailyActivity: DailyActivityRow[];
		fillArea?: boolean;
	}

	let { dailyActivity, fillArea = true }: Props = $props();

	const CONVERSATIONS_COLOR = 'var(--accent-color-darker)';
	const PROMPTS_COLOR = 'var(--accent-color-divider)';

	let scrollWrap: HTMLDivElement | undefined;
	let wrapEl: HTMLDivElement | undefined;
	let wrapWidth = $state(0);
	let lastAutoScrollKey = '';

	const CHART_HEIGHT = 114;
	const PADDING_TOP = 3;
	const PADDING_BOTTOM = 6;
	const DRAW_HEIGHT = CHART_HEIGHT - PADDING_TOP - PADDING_BOTTOM;

	let cols = $derived(Math.max(dailyActivity.length, 1));

	let maxValue = $derived.by(() => {
		let max = 1;
		for (const row of dailyActivity) {
			if (row.conversations > max) max = row.conversations;
			if (row.userPrompts > max) max = row.userPrompts;
		}
		return max;
	});

	function yPos(value: number): number {
		return PADDING_TOP + DRAW_HEIGHT - (value / maxValue) * DRAW_HEIGHT;
	}

	// Responsive: compute column width from container width.
	let colWidth = $derived(wrapWidth > 0 && cols > 0 ? wrapWidth / cols : 18);
	let svgWidth = $derived(wrapWidth > 0 ? wrapWidth : cols * 18);
	let denseCols = $derived(colWidth <= 12);

	function buildLinePath(getValue: (row: DailyActivityRow) => number): string {
		if (dailyActivity.length === 0) return '';
		return dailyActivity
			.map((row, i) => {
				const x = i * colWidth + colWidth / 2;
				const y = yPos(getValue(row));
				return `${i === 0 ? 'M' : 'L'}${x},${y}`;
			})
			.join(' ');
	}

	function buildFillPath(getValue: (row: DailyActivityRow) => number): string {
		if (dailyActivity.length === 0) return '';
		const baseline = PADDING_TOP + DRAW_HEIGHT;
		const firstX = colWidth / 2;
		const lastX = (dailyActivity.length - 1) * colWidth + colWidth / 2;
		const linePart = buildLinePath(getValue);
		return `${linePart} L${lastX},${baseline} L${firstX},${baseline} Z`;
	}

	let conversationsPath = $derived(buildLinePath((r) => r.conversations));
	let promptsPath = $derived(buildLinePath((r) => r.userPrompts));
	let conversationsFillPath = $derived(buildFillPath((r) => r.conversations));
	let promptsFillPath = $derived(buildFillPath((r) => r.userPrompts));

	let totalConversations = $derived(dailyActivity.reduce((s, r) => s + r.conversations, 0));
	let totalPrompts = $derived(dailyActivity.reduce((s, r) => s + r.userPrompts, 0));

	function formatDateLong(dateStr: string): string {
		const [y, m, d] = dateStr.split('-').map(Number);
		const date = new Date(y, m - 1, d);
		return date.toLocaleDateString(undefined, {
			weekday: 'short',
			month: 'short',
			day: 'numeric'
		});
	}

	function formatDateLabel(dateStr: string): string {
		const [y, m, d] = dateStr.split('-').map(Number);
		if (d === 1) {
			const date = new Date(y, m - 1, d);
			return date.toLocaleDateString(undefined, { month: 'short' });
		}
		return String(d);
	}

	function isMonthBoundary(dateStr: string): boolean {
		const [, , d] = dateStr.split('-').map(Number);
		return d === 1;
	}

	const autoScrollKey = $derived.by(() => {
		const first = dailyActivity[0]?.date ?? '';
		const last = dailyActivity[dailyActivity.length - 1]?.date ?? '';
		return `${dailyActivity.length}:${first}:${last}`;
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

	$effect(() => {
		if (!wrapEl) return;
		const update = () => {
			if (!wrapEl) return;
			wrapWidth = wrapEl.clientWidth;
		};
		update();
		const observer = new ResizeObserver(update);
		observer.observe(wrapEl);
		return () => observer.disconnect();
	});

	const showDayNumbers = $derived(!denseCols && colWidth > 12);
</script>

<div class="da-layout">
	<div class="da-chart-area">
		<div class="da-wrap" bind:this={scrollWrap}>
			<div class="da-chart-outer" bind:this={wrapEl}>
				{#if wrapWidth > 0}
					<svg
						class="da-svg"
						width={svgWidth}
						height={CHART_HEIGHT}
						viewBox={`0 0 ${svgWidth} ${CHART_HEIGHT}`}
					>
						<!-- Grid lines -->
						{#each [0.25, 0.5, 0.75, 1] as frac (frac)}
							<line
								x1="0"
								y1={yPos(maxValue * frac)}
								x2={svgWidth}
								y2={yPos(maxValue * frac)}
								class="da-grid-line"
							/>
						{/each}

						<!-- Fill areas -->
						{#if fillArea && conversationsFillPath}
							<path
								d={conversationsFillPath}
								fill={CONVERSATIONS_COLOR}
								opacity="0.5"
								stroke="none"
							/>
						{/if}
						{#if fillArea && promptsFillPath}
							<path d={promptsFillPath} fill={PROMPTS_COLOR} opacity="0.5" stroke="none" />
						{/if}

						<!-- Lines -->
						{#if conversationsPath}
							<path
								d={conversationsPath}
								fill="none"
								stroke={CONVERSATIONS_COLOR}
								stroke-width="2"
							/>
						{/if}
						{#if promptsPath}
							<path d={promptsPath} fill="none" stroke={PROMPTS_COLOR} stroke-width="2" />
						{/if}

						<!-- Dots -->
						{#each dailyActivity as row, i (row.date)}
							{@const x = i * colWidth + colWidth / 2}
							<circle cx={x} cy={yPos(row.conversations)} r="3" fill={CONVERSATIONS_COLOR} />
							<circle cx={x} cy={yPos(row.userPrompts)} r="3" fill={PROMPTS_COLOR} />
						{/each}
					</svg>

					<!-- Popover hit targets overlaid on the chart -->
					<div class="da-hit-targets" style="width:{svgWidth}px;height:{CHART_HEIGHT}px">
						{#each dailyActivity as row, i (row.date)}
							{@const x = i * colWidth}
							<div class="da-hit-col" style="left:{x}px;width:{colWidth}px;height:{CHART_HEIGHT}px">
								<Popover position="below" width="180px" padding="0" fixed={true}>
									<div class="da-hit-inner"></div>
									{#snippet popover()}
										<div class="da-popover">
											<div class="da-popover-date">{formatDateLong(row.date)}</div>
											<div class="da-popover-row">
												<span class="da-swatch" style="background:{CONVERSATIONS_COLOR}"></span>
												<span class="da-popover-name">Conversations</span>
												<span class="da-popover-val">{row.conversations}</span>
											</div>
											<div class="da-popover-row">
												<span class="da-swatch" style="background:{PROMPTS_COLOR}"></span>
												<span class="da-popover-name">Prompts</span>
												<span class="da-popover-val">{row.userPrompts}</span>
											</div>
										</div>
									{/snippet}
								</Popover>
							</div>
						{/each}
					</div>

					<!-- Date labels -->
					<div class="da-date-row">
						{#each dailyActivity as row, i (row.date)}
							{@const x = i * colWidth}
							<div class="da-date" style="left:{x}px;width:{colWidth}px">
								{#if isMonthBoundary(row.date) || showDayNumbers}
									{formatDateLabel(row.date)}
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	</div>
	<div class="da-side">
		<div class="da-key info-box">
			<span class="da-key-item">
				<span class="da-swatch" style="background:{CONVERSATIONS_COLOR}"></span>
				<span class="da-key-label">Conversations</span>
			</span>
			<span class="da-key-item">
				<span class="da-swatch" style="background:{PROMPTS_COLOR}"></span>
				<span class="da-key-label">User prompts</span>
			</span>
		</div>
		<div class="da-totals info-box">
			<div class="title">
				{totalConversations} conversation{totalConversations !== 1 ? 's' : ''}
			</div>
			<div class="title">
				{totalPrompts} prompt{totalPrompts !== 1 ? 's' : ''}
			</div>
			<div class="subtitle">
				last {dailyActivity.length} day{dailyActivity.length === 1 ? '' : 's'}
			</div>
		</div>
	</div>
</div>

<style>
	.da-layout {
		display: flex;
		align-items: flex-start;
		gap: 1rem;
	}

	.da-chart-area {
		flex: 1;
		position: relative;
		min-width: 0;
	}

	.da-wrap {
		margin-top: -1px;
		max-width: 100%;
		overflow-x: hidden;
		overflow-y: hidden;
		padding-bottom: 0.25rem;
		padding-top: 1px;
	}

	.da-chart-outer {
		position: relative;
		width: 100%;
	}

	.da-svg {
		display: block;
		width: 100%;
	}

	.da-grid-line {
		stroke: var(--color-divider);
		stroke-width: 0.5;
	}

	.da-hit-targets {
		position: absolute;
		top: 0;
		left: 0;
	}

	.da-hit-col {
		position: absolute;
		top: 0;
	}

	.da-hit-col :global(.popover-wrap) {
		width: 100%;
		height: 100%;
	}

	.da-hit-inner {
		width: 100%;
		height: 100%;
	}

	.da-date-row {
		position: relative;
		height: 1rem;
		width: 100%;
	}

	.da-date {
		position: absolute;
		font-size: 0.65rem;
		color: var(--color-text-faded);
		text-align: center;
		white-space: nowrap;
	}

	.da-popover {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding: 0.75rem 1rem;
	}

	.da-popover-date {
		font-weight: 600;
		font-size: 0.85rem;
		color: var(--color-text-strong);
	}

	.da-popover-row {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.85rem;
	}

	.da-swatch {
		display: inline-block;
		width: 10px;
		height: 10px;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.da-popover-name {
		color: var(--color-text-strong);
	}

	.da-popover-val {
		color: var(--color-text-tertiary);
		margin-left: auto;
		padding-left: 0.75rem;
		font-variant-numeric: tabular-nums;
	}

	.da-side {
		box-sizing: border-box;
		align-self: stretch;
		display: flex;
		flex-direction: row;
		gap: 1rem;
		align-items: stretch;
		padding-bottom: 0.5rem;
	}

	.da-key {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.da-key-item {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.9rem;
		color: var(--color-text-secondary);
	}

	.da-key-label {
		max-width: 110px;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.da-totals {
		font-size: 0.88rem;
		color: var(--color-text);
		font-variant-numeric: tabular-nums;
	}

	.da-totals .title {
		font-size: 1.15em;
		font-weight: 600;
	}

	.da-totals .subtitle {
		font-size: 1em;
		opacity: 0.8;
	}

	@media (max-width: 780px) {
		.da-layout {
			flex-direction: column;
		}

		.da-side {
			min-width: 0;
		}

		.da-key {
			flex-direction: row;
			flex-wrap: wrap;
			gap: 0.5rem 0.75rem;
		}
	}
</style>
