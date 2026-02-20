<script lang="ts">
	import { resolve } from '$app/paths';
	import { SvelteSet, SvelteMap } from 'svelte/reactivity';
	import { agentColor, MANUAL_COLOR } from './chartColors';
	import Popover from '$lib/components/Popover.svelte';
	import type { DailyCommitSummary } from '$lib/types';

	interface Props {
		dailySummary: DailyCommitSummary[];
		branch: string;
	}

	let { dailySummary, branch }: Props = $props();

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
</script>

<div class="dc-wrap">
	<div class="dc-chart">
		{#each columns as col (col.date)}
			<div class="dc-col">
				<div class="dc-bar-area">
					{#if col.total > 0}
						<Popover position="below" width="250px" padding="0">
							<div class="dc-bar">
								{#each col.segments as seg (seg.key)}
									<div class="dc-seg" style="height:{seg.percent}%;background:{seg.color}"></div>
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
														`/local/projects/${encodeURIComponent(c.projectId)}/commits/${encodeURIComponent(branch)}/${encodeURIComponent(c.commitHash)}`
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

<style>
	.dc-wrap {
		max-width: 700px;
	}

	.dc-chart {
		display: flex;
		gap: 2px;
		align-items: stretch;
	}

	.dc-col {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}

	.dc-bar-area {
		height: 120px;
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
		gap: 0.5px;
	}

	.dc-bar-empty {
		background: #f0f0f0;
	}

	.dc-seg {
		width: 100%;
		min-height: 0;
		flex-shrink: 0;
	}

	.dc-date {
		font-size: 0.65rem;
		color: #999;
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
		color: #333;
	}

	.dc-popover-lines {
		font-size: 0.8rem;
		color: #888;
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
		color: #333;
	}

	.dc-popover-pct {
		color: #888;
		margin-left: auto;
		padding-left: 0.75rem;
		font-variant-numeric: tabular-nums;
	}

	.dc-popover-commits {
		border-top: 1px solid #eee;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		max-height: 150px;
		overflow-y: auto;
		padding: 1rem;
	}

	.dc-commit-link {
		font-size: 0.8rem;
		color: var(--link-color, #1f4cd1);
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
</style>
