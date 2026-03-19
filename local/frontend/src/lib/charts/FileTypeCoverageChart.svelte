<script lang="ts">
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import Popover from '$lib/components/Popover.svelte';
	import type { FileTypeCoverage } from '$lib/types';
	import { agentColor, MANUAL_COLOR } from './chartColors';
	import { settingsStore } from '$lib/stores/settings.svelte';

	interface Props {
		data: FileTypeCoverage[];
	}

	let { data }: Props = $props();

	const showAll = $derived(settingsStore.fileTypeCoverageShowAll);
	const shortList = $derived.by(() => {
		const multiFile = data.filter((row) => row.totalFiles > 1);
		if (multiFile.length > 0 && multiFile.length < data.length) return multiFile.slice(0, 10);
		return data.slice(0, 10);
	});
	const filteredData = $derived(showAll ? data : shortList);
	const hasHiddenItems = $derived(filteredData.length < data.length);

	const agents = $derived.by(() => {
		const names = new SvelteSet<string>();
		for (const row of data) {
			for (const seg of row?.agentSegments || []) {
				names.add(seg.agent);
			}
		}
		const sorted = [...names].sort();
		const map = new SvelteMap<string, number>();
		for (let i = 0; i < sorted.length; i++) {
			map.set(sorted[i], i);
		}
		return map;
	});
</script>

{#if data.length === 0}
	<p class="empty">No file data in the selected range.</p>
{:else}
	<div class="coverage-table">
		{#each filteredData as row (row.extension)}
			<div class="ext-row">
				<div class="col-ext" title={row.extension}>{row.extension}</div>
				<div class="col-bar">
					<div class="bar">
						{#each row?.agentSegments as seg (seg.agent)}
							{#if seg.linePercent > 0}
								<Popover
									position="above"
									padding="0.6rem 0.8rem"
									flex="0 0 {seg.linePercent}%"
									fixed={true}
								>
									<div
										class="segment"
										style:min-width="3px"
										style:background={agentColor(seg.agent, agents.get(seg.agent) ?? 0)}
									></div>
									{#snippet popover()}
										<div class="popover-content">
											<strong>{seg.agent}</strong>
											<div>
												{seg.linesFromAgent.toLocaleString()} lines ({seg.linePercent.toFixed(1)}%)
											</div>
										</div>
									{/snippet}
								</Popover>
							{/if}
						{/each}
						{#if row.manualPercent > 0}
							<Popover
								position="above"
								padding="0.6rem 0.8rem"
								flex="0 0 {row.manualPercent}%"
								fixed={true}
							>
								<div class="segment" style:min-width="3px" style:background={MANUAL_COLOR}></div>
								{#snippet popover()}
									<div class="popover-content">
										<strong>Manual</strong>
										<div>
											{Math.round((row.manualPercent / 100) * row.totalLines).toLocaleString()} lines
											({row.manualPercent.toFixed(1)}%)
										</div>
									</div>
								{/snippet}
							</Popover>
						{/if}
					</div>
				</div>
				<div class="col-count">
					<div class="count">{row.totalFiles}</div>
					<div class="label">Files</div>
				</div>
			</div>
		{/each}
	</div>

	<div class="legend">
		{#each [...agents.entries()] as [agent, idx] (agent)}
			<span class="legend-item">
				<span class="legend-swatch" style:background={agentColor(agent, idx)}></span>
				{agent}
			</span>
		{/each}
		<span class="legend-item">
			<span class="legend-swatch" style:background={MANUAL_COLOR}></span>
			Manual
		</span>
		{#if hasHiddenItems || showAll}
			<button
				class="bordered tiny toggle-btn"
				onclick={() =>
					(settingsStore.fileTypeCoverageShowAll = !settingsStore.fileTypeCoverageShowAll)}
			>
				{showAll ? 'Show Less' : 'Show All'}
			</button>
		{/if}
		<div style:width="104px"></div>
	</div>
{/if}

<style>
	.empty {
		color: var(--color-text-secondary);
		margin: 1rem 0;
	}

	.coverage-table {
		display: flex;
		flex-direction: column;
		gap: 0;
	}

	.ext-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0 0 1px 0;
		font-size: 0.85rem;
	}

	.col-ext {
		width: 90px;
		flex-shrink: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.col-bar {
		flex: 1;
		min-width: 0;
	}

	.col-count {
		display: flex;
		gap: 0.5rem;
		width: 114px;
		flex-shrink: 0;
		text-align: right;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
		font-size: 0.8rem;
	}

	.col-count .count {
		text-align: right;
		width: 30px;
	}

	.col-count .label {
		text-align: left;
	}

	.bar {
		display: flex;
		height: 14px;
		border-radius: 2px;
		overflow: hidden;
		background: var(--color-background-surface);
		gap: 1px;
	}

	.segment {
		height: 100%;
		min-width: 3px;
		cursor: default;
		transition: opacity 0.15s;
	}

	.segment:hover {
		opacity: 0.8;
	}

	.popover-content {
		font-size: 0.8rem;
		line-height: 1.4;
	}

	.popover-content strong {
		display: block;
		margin-bottom: 0.15rem;
	}

	.legend {
		display: flex;
		gap: 1.5rem;
		margin-top: 0.6rem;
		margin-left: calc(90px + 0.75rem);
		flex-wrap: wrap;
	}

	.legend-item {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.75rem;
		color: var(--color-text-secondary);
	}

	.legend-swatch {
		display: inline-block;
		width: 10px;
		height: 10px;
		border-radius: 2px;
	}

	.toggle-btn {
		margin-left: auto;
	}
</style>
