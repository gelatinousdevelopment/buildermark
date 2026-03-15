<script lang="ts">
	import { stars, fmtTime } from '$lib/utils';
	import { renderMarkdown, normalizeEscapedNewlines } from '$lib/messageUtils';
	import type { Rating } from '$lib/types';

	interface Props {
		rating: Rating;
		ondelete?: (id: string) => void;
	}

	let { rating, ondelete }: Props = $props();
	let deleting = $state(false);

	function handleDelete(e: MouseEvent) {
		e.stopPropagation();
		if (deleting) return;
		deleting = true;
		ondelete?.(rating.id);
	}
</script>

<div class="message-header">
	<strong>Rating</strong> &middot; {fmtTime(rating.createdAt)}
	{#if ondelete}
		<button class="delete-btn" title="Delete rating" disabled={deleting} onclick={handleDelete}>
			{deleting ? '...' : '×'}
		</button>
	{/if}
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

	.delete-btn {
		margin-left: auto;
		background: none;
		border: none;
		color: var(--color-text-tertiary);
		cursor: pointer;
		font-size: 1.1rem;
		line-height: 1;
		padding: 0.1rem 0.3rem;
		border-radius: 4px;
	}

	.delete-btn:hover {
		color: var(--color-danger, #e53e3e);
		background: var(--color-bg-hover, rgba(0, 0, 0, 0.05));
	}

	.delete-btn:disabled {
		opacity: 0.5;
		cursor: default;
	}
</style>
