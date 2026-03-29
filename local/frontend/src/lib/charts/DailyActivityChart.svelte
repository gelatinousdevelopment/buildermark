<script lang="ts">
	import { tick } from 'svelte';
	import Popover from '$lib/components/Popover.svelte';
	import Icon from '$lib/Icon.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import type { DailyActivityRow } from '$lib/types';

	interface Props {
		dailyActivity: DailyActivityRow[];
		fillArea?: boolean;
		height?: number;
	}

	let { dailyActivity, fillArea = true, height = 114 }: Props = $props();
	let countAnswers = $derived(settingsStore.activityChartCountAnswers);
	let countChildConversationsSeparately = $derived(
		settingsStore.activityChartCountChildConversationsSeparately
	);
	const popoverId = `da-menu-${Math.random().toString(36).slice(2, 8)}`;
	let menuBtn: HTMLButtonElement | undefined;
	let menuBody: HTMLDivElement | undefined;

	function effectivePrompts(row: DailyActivityRow): number {
		return row.userPrompts + (countAnswers ? row.userAnswers : 0);
	}

	function positionMenu() {
		if (!menuBtn || !menuBody) return;
		const rect = menuBtn.getBoundingClientRect();
		menuBody.style.top = `${rect.bottom + 4}px`;
		menuBody.style.left = `${rect.left}px`;
	}

	const CONVERSATIONS_COLOR = 'var(--accent-color-darker)';
	const PROMPTS_COLOR = 'var(--accent-color-divider)';

	let scrollWrap: HTMLDivElement | undefined;
	let wrapEl: HTMLDivElement | undefined;
	let wrapWidth = $state(0);
	let lastAutoScrollKey = '';

	const PADDING_TOP = 3;
	const PADDING_BOTTOM = 6;
	let CHART_HEIGHT = $derived(height);
	let DRAW_HEIGHT = $derived(CHART_HEIGHT - PADDING_TOP - PADDING_BOTTOM);

	let cols = $derived(Math.max(dailyActivity.length, 1));

	let maxValue = $derived.by(() => {
		let max = 1;
		for (const row of dailyActivity) {
			if (row.conversations > max) max = row.conversations;
			const p = effectivePrompts(row);
			if (p > max) max = p;
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
	let promptsPath = $derived(buildLinePath((r) => effectivePrompts(r)));
	let conversationsFillPath = $derived(buildFillPath((r) => r.conversations));
	let promptsFillPath = $derived(buildFillPath((r) => effectivePrompts(r)));

	let totalConversations = $derived(dailyActivity.reduce((s, r) => s + r.conversations, 0));
	let totalPrompts = $derived(dailyActivity.reduce((s, r) => s + effectivePrompts(r), 0));
	let promptsPerConversation = $derived(
		totalConversations > 0 ? (totalPrompts / totalConversations).toFixed(2) : '0'
	);

	let endsToday = $derived.by(() => {
		if (dailyActivity.length === 0) return true;
		const lastDate = dailyActivity[dailyActivity.length - 1].date;
		const now = new Date();
		const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
		return lastDate === today;
	});

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
		<button
			class="da-menu-btn"
			bind:this={menuBtn}
			popovertarget={popoverId}
			aria-label="Chart options"
		>
			<Icon name="chevronRight" width="12px" />
		</button>
		<div
			id={popoverId}
			class="da-menu-body"
			bind:this={menuBody}
			popover="auto"
			onbeforetoggle={(e: ToggleEvent) => {
				if (e.newState === 'open') positionMenu();
			}}
		>
			<label class="da-menu-option">
				<input
					type="checkbox"
					checked={settingsStore.activityChartCountChildConversationsSeparately}
					onchange={(e) =>
						(settingsStore.activityChartCountChildConversationsSeparately = (
							e.currentTarget as HTMLInputElement
						).checked)}
				/>
				Count linked child conversations separately
			</label>
			<label class="da-menu-option">
				<input
					type="checkbox"
					checked={settingsStore.activityChartCountAnswers}
					onchange={(e) =>
						(settingsStore.activityChartCountAnswers = (
							e.currentTarget as HTMLInputElement
						).checked)}
				/>
				Count answers as prompts
			</label>
		</div>
		<div class="da-wrap" bind:this={scrollWrap}>
			<div class="da-chart-outer" bind:this={wrapEl}>
				{#if wrapWidth > 0}
					<svg
						class="da-svg"
						width={svgWidth}
						height={CHART_HEIGHT}
						viewBox={`0 0 ${svgWidth} ${CHART_HEIGHT}`}
					>
						<!-- Grid lines + tick labels -->
						{#each [0.25, 0.5, 0.75, 1] as frac (frac)}
							<line
								x1="0"
								y1={yPos(maxValue * frac)}
								x2={svgWidth}
								y2={yPos(maxValue * frac)}
								class="da-grid-line"
							/>
							<text x="0" y={yPos(maxValue * frac) + 11} class="da-tick-label"
								>{Math.round(maxValue * frac)}</text
							>
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
							<circle cx={x} cy={yPos(effectivePrompts(row))} r="3" fill={PROMPTS_COLOR} />
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
												<span class="da-popover-val">{effectivePrompts(row)}</span>
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
				<span class="da-key-label">Builder Prompts</span>
			</span>
			<div style:flex="1"></div>
			<Popover position="above" width="300px" padding="0.75rem" wrapWidth="18px">
				<div class="da-details-icon"><Icon name="info" width="18px" /></div>
				{#snippet popover()}
					<div class="da-details-popover">
						<div class="da-details-row">
							<strong>Conversations</strong>
							{#if countChildConversationsSeparately}
								assigns each conversation to one day only: the day of its latest user message.
							{:else}
								merges linked child conversations into their root conversation and assigns each root
								family to one day only: the day of its latest user message.
							{/if}
						</div>
						<div class="da-details-row">
							<strong>Builder Prompts</strong> assigns each prompt to one day only: the day it was sent.
						</div>
						{#if countAnswers}
							<div class="da-details-row">
								Answers are also included in prompt totals, once each on the day they were sent.
							</div>
						{/if}
						<div class="da-details-row">
							First prompts in child conversations are excluded from this chart.
						</div>
						<div class="da-details-row">
							The Conversations page date filter is broader and shows any conversation with any
							message on that day, so those counts can differ.
						</div>
					</div>
				{/snippet}
			</Popover>
		</div>
		<div class="da-totals info-box">
			<div class="title big">
				{totalConversations}
			</div>
			<div class="title medium">conversation{totalConversations !== 1 ? 's' : ''}</div>
			<div class="title big">
				{totalPrompts}
			</div>
			<div class="title medium">prompt{totalPrompts !== 1 ? 's' : ''}</div>
			<div class="title small">
				avg {promptsPerConversation} p/c
			</div>
			<div class="title small">
				{endsToday ? 'last ' : ''}{dailyActivity.length} day{dailyActivity.length === 1 ? '' : 's'}
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

	.da-menu-btn {
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
		border: var(--divider-width) solid var(--color-divider);
		border-radius: 4px;
		background: var(--color-background-elevated);
		cursor: pointer;
		opacity: 0;
		transition: opacity 0.15s;
	}

	.da-menu-btn :global(.icon) {
		transform: rotate(90deg);
	}

	.da-chart-area:hover .da-menu-btn,
	.da-chart-area:has(:popover-open) .da-menu-btn {
		opacity: 1;
	}

	.da-menu-body {
		position: fixed;
		inset: unset;
		margin: 0;
		background: var(--color-background-surface);
		border: var(--divider-width) solid var(--color-divider);
		border-radius: 5px;
		box-shadow: 0 2px 8px var(--color-popover-shadow);
		padding: 0.5rem 1rem 0.5rem 0.7rem;
		white-space: nowrap;
	}

	.da-menu-option {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.85rem;
		color: var(--color-text);
		cursor: pointer;
		user-select: none;
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

	.da-tick-label {
		font-size: 9px;
		fill: var(--color-text-faded);
		user-select: none;
		pointer-events: none;
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

	.da-details-icon {
		color: var(--color-text-tertiary);
	}

	.da-details-icon:hover {
		color: var(--accent-color);
	}

	.da-details-popover {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		white-space: normal;
		font-size: 0.85rem;
		line-height: 1.4;
		color: var(--color-text-secondary);
	}

	.da-details-row {
		max-width: 300px;
	}

	.da-details-row strong {
		color: var(--color-text);
	}

	.da-totals {
		font-size: 0.88rem;
		color: var(--color-text);
		font-variant-numeric: tabular-nums;
	}

	.da-totals .title {
		font-size: 1.4em;
		font-weight: 500;
		line-height: 1.1;
	}

	.da-totals .title.big {
		font-size: 1.8em;
		font-weight: 600;
	}

	.da-totals .title.medium {
		margin-top: -4px;
	}

	.da-totals .title.small {
		font-size: 0.9rem;
		font-weight: normal;
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
