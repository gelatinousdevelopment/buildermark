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

	interface Segment {
		name: string;
		percent: number;
	}

	interface Props {
		/** Single aggregate agent percentage (used when segments is empty). */
		agentPercent?: number;
		/** Per-agent breakdown. When provided and non-empty, each agent gets its own color. */
		segments?: Segment[];
		/** Show the key/legend below the bar. */
		showKey?: boolean;
		/** Include the Manual label in the key. */
		showManual?: boolean;
	}

	let { agentPercent = 0, segments = [], showKey = true, showManual = false }: Props = $props();

	// 20 default fallback colors for agents without a CSS variable override.
	const DEFAULT_COLORS = [
		'#e07b39',
		'#10b981',
		'#3b82f6',
		'#8b5cf6',
		'#ec4899',
		'#f59e0b',
		'#06b6d4',
		'#ef4444',
		'#84cc16',
		'#6366f1',
		'#14b8a6',
		'#f97316',
		'#a855f7',
		'#22d3ee',
		'#fb7185',
		'#fbbf24',
		'#34d399',
		'#818cf8',
		'#f472b6',
		'#2dd4bf'
	];

	const MANUAL_COLOR = 'var(--agent-color-manual, #656565)';

	/** Convert an agent name to its CSS variable reference with a fallback. */
	function agentColor(name: string, index: number): string {
		const varName = `--agent-color-${name.replace(/[^a-zA-Z0-9]/g, '-').toLowerCase()}`;
		return `var(${varName}, ${DEFAULT_COLORS[index % DEFAULT_COLORS.length]})`;
	}

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

	/** All bar segments including Manual. */
	let barSegments: ResolvedSegment[] = $derived.by(() => {
		const result = [...resolvedSegments];
		if (manualPercent > 0) {
			result.push({ name: 'Manual', percent: manualPercent, color: MANUAL_COLOR });
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

<div class="apb">
	{#if showKey}
		<div class="apb-bar" role="img" aria-label="Agent percentage bar">
			{#each barSegments as seg (seg.name)}
				<span class="apb-segment" style="width:{seg.percent}%;background:{seg.color}"></span>
			{/each}
		</div>
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
		font-size: 0.78rem;
	}

	.apb-popover-name {
		color: #333;
	}

	.apb-popover-pct {
		color: #888;
		margin-left: auto;
		padding-left: 0.75rem;
		font-variant-numeric: tabular-nums;
	}

	.apb-bar {
		background: var(--agent-color-manual, #656565);
		border-radius: 2px;
		display: flex;
		height: 8px;
		overflow: hidden;
		width: 100%;
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
		color: #555;
	}

	.apb-swatch {
		display: inline-block;
		width: 10px;
		height: 10px;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.apb-pct {
		color: #888;
		font-variant-numeric: tabular-nums;
	}
</style>
