<script lang="ts">
	import Popover from '$lib/components/Popover.svelte';
	import Icon from '$lib/Icon.svelte';
	import type { AgentRatingDistribution } from '$lib/types';

	interface Props {
		agents: AgentRatingDistribution[];
		showUnrated?: boolean;
		showHeader?: boolean;
	}

	let { agents, showUnrated = false, showHeader = false }: Props = $props();

	const BUCKETS = ['unrated', '1', '2', '3', '4', '5'] as const;
	const BUCKET_COLORS: Record<string, string> = {
		unrated: '#9ca3af',
		'1': 'var(--color-stars-1)',
		'2': 'var(--color-stars-2)',
		'3': 'var(--color-stars-3)',
		'4': 'var(--color-stars-4)',
		'5': 'var(--color-stars-5)'
	};
	const BUCKET_LABELS: Record<string, string> = {
		unrated: 'Unrated',
		'1': '1 star',
		'2': '2 stars',
		'3': '3 stars',
		'4': '4 stars',
		'5': '5 stars'
	};

	const visibleBuckets = $derived(showUnrated ? BUCKETS : BUCKETS.filter((b) => b !== 'unrated'));

	const sorted = $derived(
		[...agents]
			.map((a) => {
				if (showUnrated) return a;
				const total = a.ratedConversations;
				return { ...a, totalConversations: total };
			})
			.filter((a) => a.totalConversations > 0)
			.sort((a, b) => b.totalConversations - a.totalConversations)
	);
</script>

{#if sorted.length === 0}
	<p class="empty">No conversations in the selected range.</p>
{:else}
	<div class="ratings-table">
		{#if showHeader}
			<div class="header-row">
				<div class="col-agent">Agent</div>
				<div class="col-bar">Distribution</div>
				<div class="col-stat">Avg</div>
				<div class="col-stat">Reviews</div>
				{#if showUnrated}
					<div class="col-stat">Convos</div>
				{/if}
			</div>
		{/if}
		{#each sorted as agent (agent.agent)}
			{@const total = agent.totalConversations}
			<div class="agent-row">
				<div class="col-agent" title={agent.agent}>{agent.agent}</div>
				<div class="col-bar">
					<div class="bar">
						{#each visibleBuckets as bucket (bucket)}
							{@const count = agent.distribution[bucket] ?? 0}
							{#if count > 0}
								{@const pct = (count / total) * 100}
								<Popover position="above" padding="0.6rem 0.8rem" flex="0 0 {pct}%" fixed={true}>
									<div
										class="segment"
										style:min-width="3px"
										style:background={BUCKET_COLORS[bucket]}
									></div>
									{#snippet popover()}
										<div class="popover-content">
											<strong>{agent.agent}</strong>
											<div>{BUCKET_LABELS[bucket]}: {count} ({pct.toFixed(1)}%)</div>
										</div>
									{/snippet}
								</Popover>
							{/if}
						{/each}
					</div>
				</div>
				<div class="col-stat col-avg">
					{#if agent.ratedConversations > 0}
						{agent.averageRating.toFixed(1)}<span class="star-icon"
							><Icon name="star" width="13px" height="13px" /></span
						>
					{:else}
						–
					{/if}
				</div>
				<div class="col-stat">
					{agent.ratedConversations}{#if !showHeader}&nbsp;Reviews{/if}
				</div>
				{#if showUnrated}
					<div class="col-stat">{agent.totalConversations}</div>
				{/if}
			</div>
		{/each}
	</div>

	<div class="legend">
		{#each visibleBuckets as bucket (bucket)}
			<span class="legend-item">
				<span class="legend-swatch" style:background={BUCKET_COLORS[bucket]}></span>
				{#if bucket === 'unrated'}
					Unrated
				{:else}
					{#each Array.from({ length: Number(bucket) }, (_, i) => i) as i (i)}<span
							class="legend-star"
							><Icon
								name="star"
								width="12px"
								height="12px"
								fillColor={BUCKET_COLORS[bucket]}
							/></span
						>{/each}
				{/if}
			</span>
		{/each}
	</div>
{/if}

<style>
	.empty {
		color: var(--color-text-secondary);
		margin: 1rem 0;
	}

	.ratings-table {
		display: flex;
		flex-direction: column;
		gap: 0;
	}

	.header-row,
	.agent-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.3rem 0;
	}

	.header-row {
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--color-text-secondary);
		border-bottom: var(--divider-width) solid var(--color-divider);
		padding-bottom: 0.4rem;
		margin-bottom: 0.2rem;
	}

	.agent-row {
		font-size: 0.85rem;
	}

	.col-agent {
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

	.col-stat {
		width: 70px;
		flex-shrink: 0;
		text-align: right;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.col-avg {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 1px;
		width: 34px;
	}

	.star-icon {
		display: inline-flex;
		align-items: center;
	}

	.bar {
		display: flex;
		height: 20px;
		border-radius: 3px;
		overflow: hidden;
		background: var(--color-background-surface);
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
		margin: 0.3rem 0;
		margin-left: calc(90px + 0.75rem);
		flex-wrap: wrap;
	}

	.legend-item {
		display: flex;
		align-items: center;
		gap: 0rem;
		font-size: 0.75rem;
		color: var(--color-text-secondary);
	}

	.legend-swatch {
		display: inline-block;
		width: 10px;
		height: 10px;
		border-radius: 2px;
		margin-right: 0.2rem;
	}

	.legend-star {
		display: inline-flex;
		align-items: center;
	}
</style>
