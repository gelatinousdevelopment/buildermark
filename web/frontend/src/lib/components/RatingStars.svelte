<script lang="ts">
	import type { Rating } from '$lib/types';
	import { stars } from '$lib/utils';
	import { renderMarkdown, normalizeEscapedNewlines } from '$lib/messageUtils';
	import Popover from './Popover.svelte';

	interface Props {
		ratings: Rating[];
	}

	let { ratings }: Props = $props();

	let avg = $derived(
		ratings.length > 0
			? Math.round(ratings.reduce((sum, r) => sum + r.rating, 0) / ratings.length)
			: 0
	);
</script>

{#if ratings.length > 0}
	<Popover position="leading" width="360px">
		<span class="avg-stars">{stars(avg)}</span>
		{#snippet popover()}
			<div class="rating-popover">
				{#each ratings as r (r.id)}
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
{/if}

<style>
	.avg-stars {
		cursor: default;
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
		color: #333;
	}

	.rating-analysis {
		color: #666;
		font-size: 0.82rem;
	}
</style>
