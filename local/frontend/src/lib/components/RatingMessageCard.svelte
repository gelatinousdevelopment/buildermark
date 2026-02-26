<script lang="ts">
	import { stars, fmtTime } from '$lib/utils';
	import { renderMarkdown, normalizeEscapedNewlines } from '$lib/messageUtils';
	import type { Rating } from '$lib/types';

	interface Props {
		rating: Rating;
	}

	let { rating }: Props = $props();
</script>

<div class="message-header">
	<strong>Rating</strong> &middot; {fmtTime(rating.createdAt)}
</div>
<div class="rating-stars">{stars(rating.rating)}</div>
{#if rating.note}
	<div class="rating-field">
		<strong>Note:</strong>
		<div class="markdown-body">
			<!-- eslint-disable-next-line svelte/no-at-html-tags -->
			{@html renderMarkdown(normalizeEscapedNewlines(rating.note))}
		</div>
	</div>
{/if}
{#if rating.analysis}
	<div class="rating-field">
		<strong>Analysis:</strong>
		<div class="markdown-body">
			<!-- eslint-disable-next-line svelte/no-at-html-tags -->
			{@html renderMarkdown(normalizeEscapedNewlines(rating.analysis))}
		</div>
	</div>
{/if}

<style>
	.message-header {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.rating-stars {
		font-size: 1.1rem;
		margin-bottom: 0.25rem;
	}

	.rating-field {
		font-size: 0.9rem;
		margin-top: 0.25rem;
	}

	.markdown-body {
		font-size: 1rem;
	}
</style>
