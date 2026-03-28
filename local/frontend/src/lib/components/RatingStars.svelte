<script lang="ts">
	import type { Rating } from '$lib/types';
	import { stars } from '$lib/utils';
	import { renderMarkdown, normalizeEscapedNewlines } from '$lib/messageUtils';
	import Popover from './Popover.svelte';
	import AddRatingForm from './AddRatingForm.svelte';

	interface Props {
		ratings: Rating[];
		conversationId?: string;
		agent?: string;
		projectPath?: string;
	}

	let { ratings, conversationId, agent, projectPath }: Props = $props();

	let localRatings: Rating[] = $state([]);
	let allRatings = $derived([...ratings, ...localRatings]);

	let avg = $derived(
		allRatings.length > 0
			? Math.round(allRatings.reduce((sum, r) => sum + r.rating, 0) / allRatings.length)
			: 0
	);
</script>

{#if allRatings.length > 0}
	<Popover position="leading" width="360px">
		<span class="avg-stars">{stars(avg)}</span>
		{#snippet popover()}
			<div class="rating-popover">
				{#each allRatings as r (r.id)}
					<div class="rating-entry">
						<div class="rating-stars">{stars(r.rating)}</div>
						{#if r.note}
							<div class="rating-note markdown-body">
								<!-- eslint-disable-next-line svelte/no-at-html-tags -->
								{@html renderMarkdown(normalizeEscapedNewlines(r.note))}
							</div>
						{/if}
						{#if r.analysis}
							<div class="rating-analysis markdown-body">
								<!-- eslint-disable-next-line svelte/no-at-html-tags -->
								{@html renderMarkdown(normalizeEscapedNewlines(r.analysis))}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/snippet}
	</Popover>
{:else if conversationId}
	<Popover position="leading" width="360px">
		<span class="empty-stars">{stars(0)}</span>
		{#snippet popover()}
			<AddRatingForm
				{conversationId}
				{agent}
				{projectPath}
				onrating={(r) => (localRatings = [...localRatings, r])}
			/>
		{/snippet}
	</Popover>
{/if}

<style>
	.avg-stars {
		color: var(--color-rating-stars);
		cursor: default;
	}

	.empty-stars {
		color: var(--color-rating-stars);
		cursor: default;
		opacity: 0;
		transition: opacity 0.15s;
	}

	:global(td:hover) .empty-stars {
		opacity: 0.5;
	}

	.rating-popover {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		font-size: 0.9rem;
		white-space: normal;
	}

	.rating-entry {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.rating-stars {
		flex-shrink: 0;
	}

	.rating-note {
		color: var(--color-text-strong);
	}

	.rating-analysis {
		color: var(--color-text-secondary);
		font-size: 0.82rem;
	}
</style>
