<script lang="ts">
	/**
	 * AgentPercentageBar – horizontal color bar showing agent vs manual contribution.
	 *
	 * Modes:
	 *  - Total: pass `agentPercent` to show a single "Agent" segment vs Manual.
	 *  - Per-agent: pass `segments` array to show each agent's contribution separately.
	 *
	 * Options:
	 *  - showKey (default true): render a GitHub-style legend below the bar.
	 *  - showManual (default false): include the Manual entry in the legend.
	 */

	import Popover from './Popover.svelte';
	import { agentColor, MANUAL_COLOR } from '$lib/charts/chartColors';

	interface Segment {
		name: string;
		percent: number;
	}

	interface Props {
		/** Single aggregate agent percentage (used when segments is empty). */
		agentPercent?: number;
		/** Per-agent breakdown. When provided and non-empty, each agent gets its own color. */
		segments?: Segment[];
		/** Total lines in the denominator. When 0, bar shows empty (gray). */
		totalLines?: number;
		/** Show the key/legend below the bar. */
		showKey?: boolean;
		/** Include the Manual label in the key. */
		showManual?: boolean;
		height?: string;
		/** When true, the commit's parent is missing (shallow clone boundary). */
		needsParent?: boolean;
		/** Link to the commit detail page (used when needsParent is true). */
		commitHref?: string;
	}

	let {
		agentPercent = 0,
		segments = [],
		totalLines = -1,
		showKey = true,
		showManual = false,
		height = undefined,
		needsParent = false,
		commitHref = undefined
	}: Props = $props();

	interface ResolvedSegment {
		name: string;
		percent: number;
		color: string;
	}

	let resolvedSegments: ResolvedSegment[] = $derived.by(() => {
		if (segments.length > 0) {
			return segments
				.filter((s) => s.percent > 0)
				.map((s, i) => ({
					name: s.name,
					percent: s.percent,
					color: agentColor(s.name, i)
				}));
		}
		if (agentPercent > 0) {
			return [
				{
					name: 'Agent',
					percent: agentPercent,
					color: agentColor('generic', 0)
				}
			];
		}
		return [];
	});

	let manualPercent = $derived.by(() => {
		const total = resolvedSegments.reduce((sum, s) => sum + s.percent, 0);
		return Math.max(0, 100 - total);
	});

	/** All bar segments including Manual. Empty when totalLines is 0 (gray bar). */
	let barSegments: ResolvedSegment[] = $derived.by(() => {
		if (totalLines === 0) return [];
		const result = [...resolvedSegments];
		if (manualPercent > 0) {
			result.push({ name: 'manual', percent: manualPercent, color: MANUAL_COLOR });
		}
		return result;
	});

	/** Segments shown in the key/legend. */
	let keySegments: ResolvedSegment[] = $derived.by(() => {
		return showManual ? barSegments : resolvedSegments;
	});

	/** Key formatting: show decimals only for extreme values (<1% or >99%). */
	function formatKeyPercent(percent: number): string {
		if (percent < 1 || (percent > 99 && percent < 100)) {
			return `${percent.toFixed(1)}%`;
		}
		return `${Math.round(percent)}%`;
	}
</script>

<div class="apb" style:--bar-height={height}>
	{#if needsParent}
		<Popover position="leading">
			{#if commitHref}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- href is pre-resolved by caller -->
				<a
					href={commitHref}
					class="apb-bar apb-bar-warning"
					aria-label="Shallow clone — parent missing"><span class="apb-warning-icon">⚠</span></a
				>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{:else}
				<div class="apb-bar apb-bar-warning" role="img" aria-label="Shallow clone — parent missing">
					<span class="apb-warning-icon">⚠</span>
				</div>
			{/if}
			{#snippet popover()}
				<div class="apb-popover-content">
					This commit's parent isn't available locally. Click to resolve.
				</div>
			{/snippet}
		</Popover>
	{:else if showKey}
		<div class="apb-bar" role="img" aria-label="Agent percentage bar">
			{#each barSegments as seg (seg.name)}
				<span class="apb-segment" style="width:{seg.percent}%;background:{seg.color}"></span>
			{/each}
		</div>
	{:else if barSegments.length === 0}
		<div class="apb-bar" role="img" aria-label="Agent percentage bar"></div>
	{:else}
		<Popover position="leading">
			<div class="apb-bar" role="img" aria-label="Agent percentage bar">
				{#each barSegments as seg (seg.name)}
					<span class="apb-segment" style="width:{seg.percent}%;background:{seg.color}"></span>
				{/each}
			</div>
			{#snippet popover()}
				<div class="apb-popover-content">
					{#each barSegments as seg (seg.name)}
						<div class="apb-popover-row">
							<span class="apb-swatch" style="background:{seg.color}"></span>
							<span class="apb-popover-name">{seg.name}</span>
							<span class="apb-popover-pct">{seg.percent.toFixed(1)}%</span>
						</div>
					{/each}
				</div>
			{/snippet}
		</Popover>
	{/if}
	{#if showKey && keySegments.length > 0}
		<div class="apb-key">
			{#each keySegments as seg (seg.name)}
				<span class="apb-key-item">
					<span class="apb-swatch" style="background:{seg.color}"></span>
					<span class="apb-label">{seg.name}</span>
					<span class="apb-pct">{formatKeyPercent(seg.percent)}</span>
				</span>
			{/each}
		</div>
	{/if}
</div>

<style>
	.apb {
		--bar-height: 10px;
		width: 100%;
	}

	.apb-popover-content {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.apb-popover-row {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 1rem;
	}

	.apb-popover-name {
		color: var(--color-text-strong);
		font-size: 15px;
	}

	.apb-popover-pct {
		color: var(--color-text-tertiary);
		margin-left: auto;
		padding-left: 0.75rem;
		font-variant-numeric: tabular-nums;
	}

	.apb-bar {
		background: var(--color-background-empty, #f0f0f0);
		border-radius: 2px;
		display: flex;
		gap: var(--divider-width);
		height: var(--bar-height, 10px);
		overflow: hidden;
		width: 100%;
	}

	.apb-bar-warning {
		justify-content: center;
		align-items: center;
		background: var(--color-background-empty, #f0f0f0);
		text-decoration: none;
		cursor: pointer;
	}

	.apb-warning-icon {
		font-size: 0.65rem;
		line-height: 1;
		color: var(--color-status-yellow, #b08800);
	}

	.apb-segment {
		display: block;
		height: 100%;
		min-width: 0;
		transition: width 0.2s ease;
	}

	.apb-key {
		display: flex;
		flex-wrap: wrap;
		gap: 0.6rem;
		margin-top: 0.4rem;
	}

	.apb-key-item {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.78rem;
		color: var(--color-text-secondary);
	}

	.apb-swatch {
		display: inline-block;
		width: 10px;
		height: 10px;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.apb-pct {
		color: var(--color-text-tertiary);
		font-variant-numeric: tabular-nums;
	}
</style>
