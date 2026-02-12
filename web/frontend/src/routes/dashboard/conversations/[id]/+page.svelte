<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getConversation, createRating } from '$lib/api';
	import { stars, fmtTime } from '$lib/utils';
	import { marked } from 'marked';
	import { SvelteSet } from 'svelte/reactivity';
	import type { ConversationDetail, MessageRead, Rating } from '$lib/types';

	type TimelineItem =
		| { kind: 'message'; message: MessageRead; time: number }
		| { kind: 'rating'; rating: Rating; time: number };

	let conversation: ConversationDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	let inlineRatingMessageId: string | null = $state(null);
	let inlineRatingValue: number = $state(0);
	let inlineNote: string = $state('');
	let inlineSubmitting: boolean = $state(false);
	let inlineError: string | null = $state(null);
	let expandedMessages = $state(new SvelteSet<string>());

	function isUserPromptMessage(message: MessageRead): boolean {
		if (message.role !== 'user') return false;
		const trimmed = message.content.trimStart();
		if (trimmed.startsWith('/zrate') || trimmed.startsWith('$zrate')) return false;
		return true;
	}

	function escapeHtml(s: string): string {
		return s
			.replaceAll('&', '&amp;')
			.replaceAll('<', '&lt;')
			.replaceAll('>', '&gt;')
			.replaceAll('"', '&quot;')
			.replaceAll("'", '&#39;');
	}

	function renderMarkdown(content: string): string {
		return marked.parse(escapeHtml(content), { gfm: true, breaks: true }) as string;
	}

	function firstLine(s: string): string {
		return s.replace(/\s+/g, ' ').trim();
	}

	function messageTypeLabel(message: MessageRead): string {
		try {
			const obj = JSON.parse(message.rawJson) as Record<string, unknown>;
			const t = typeof obj.type === 'string' ? obj.type : '';
			if (t) return t;
		} catch {
			// ignore parse failures
		}
		return message.role;
	}

	function messageSummary(message: MessageRead): string {
		const line = firstLine(message.content);
		if (!line) return `[${messageTypeLabel(message)}]`;
		return line.length > 120 ? `${line.slice(0, 117)}...` : line;
	}

	function toggleExpanded(messageId: string) {
		if (expandedMessages.has(messageId)) {
			expandedMessages.delete(messageId);
			return;
		}
		expandedMessages.add(messageId);
	}

	function openInlineRating(messageId: string, starValue: number) {
		inlineRatingMessageId = messageId;
		inlineRatingValue = starValue;
		inlineNote = '';
		inlineError = null;
	}

	function cancelInlineRating() {
		inlineRatingMessageId = null;
		inlineRatingValue = 0;
		inlineNote = '';
		inlineError = null;
	}

	async function submitInlineRating() {
		if (!conversation || !inlineRatingMessageId || inlineRatingValue < 1) return;
		inlineSubmitting = true;
		inlineError = null;
		try {
			const newRating = await createRating(conversation.id, inlineRatingValue, inlineNote);
			conversation.ratings = [...conversation.ratings, newRating];
			cancelInlineRating();
		} catch (e) {
			inlineError = e instanceof Error ? e.message : 'Failed to submit rating';
		} finally {
			inlineSubmitting = false;
		}
	}

	let timeline: TimelineItem[] = $derived.by(() => {
		if (!conversation) return [];

		const matchedMessageIds = new SvelteSet<string>();
		const matchedRatingIds = new SvelteSet<string>();
		const items: TimelineItem[] = [];

		// Match each rating to the closest /zrate user message within 120s
		for (const rating of conversation.ratings) {
			const ratingTime = new Date(rating.createdAt).getTime();
			let bestMessage: MessageRead | null = null;
			let bestDelta = Infinity;

			for (const message of conversation.messages) {
				const trimmed = message.content.trimStart();
				if (message.role !== 'user' || (!trimmed.startsWith('/zrate') && !trimmed.startsWith('$zrate'))) {
					continue;
				}
				if (matchedMessageIds.has(message.id)) continue;
				const delta = Math.abs(message.timestamp - ratingTime);
				if (delta < bestDelta && delta <= 120_000) {
					bestDelta = delta;
					bestMessage = message;
				}
			}

			if (bestMessage) {
				matchedMessageIds.add(bestMessage.id);
				matchedRatingIds.add(rating.id);
				items.push({ kind: 'rating', rating, time: bestMessage.timestamp });
			}
		}

		// Add unmatched messages
		for (const message of conversation.messages) {
			if (!matchedMessageIds.has(message.id)) {
				items.push({ kind: 'message', message, time: message.timestamp });
			}
		}

		// Add unmatched ratings
		for (const rating of conversation.ratings) {
			if (!matchedRatingIds.has(rating.id)) {
				items.push({ kind: 'rating', rating, time: new Date(rating.createdAt).getTime() });
			}
		}

		items.sort((a, b) => a.time - b.time);
		return items;
	});

	onMount(async () => {
		try {
			const id = page.params.id;
			if (!id) throw new Error('Missing conversation ID');
			conversation = await getConversation(id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load conversation';
		} finally {
			loading = false;
		}
	});
</script>

<div class="breadcrumb">
	<a href={resolve('/dashboard')}>Dashboard</a> &rsaquo; Conversation
</div>

{#if loading}
	<p class="loading">Loading conversation...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if conversation}
	<h2>{conversation.title || conversation.id}</h2>
	<p>Agent: {conversation.agent} | Project: {conversation.projectId}</p>

	{#if timeline.length === 0}
		<p>No messages or ratings.</p>
	{:else}
		{#each timeline as item (item.kind === 'message' ? item.message.id : item.rating.id)}
			{#if item.kind === 'message'}
				{#if isUserPromptMessage(item.message)}
					<div class="message">
						<div class="message-header">
							<strong>{item.message.role}</strong> &middot; {fmtTime(item.message.timestamp)}
							<span class="expansion-indicator"><span class="chevron">▾</span></span>
						</div>
						<div class="message-content markdown-body">{@html renderMarkdown(item.message.content)}</div>
					</div>
					<div class="inline-rating">
						{#if inlineRatingMessageId === item.message.id}
							<div class="inline-rating-expanded">
								<div class="inline-stars">
									{#each [1, 2, 3, 4, 5] as star (star)}
										<button
											class="star-btn"
											class:active={star <= inlineRatingValue}
											onclick={() => (inlineRatingValue = star)}
										>
											{star <= inlineRatingValue ? '★' : '☆'}
										</button>
									{/each}
								</div>
								<input
									type="text"
									class="inline-note"
									placeholder="Optional note..."
									bind:value={inlineNote}
								/>
								<div class="inline-actions">
									<button
										class="btn-sm"
										disabled={inlineSubmitting || inlineRatingValue < 1}
										onclick={submitInlineRating}
									>
										{inlineSubmitting ? 'Submitting...' : 'Submit'}
									</button>
									<button class="btn-sm btn-cancel" onclick={cancelInlineRating}> Cancel </button>
								</div>
								{#if inlineError}
									<p class="inline-error">{inlineError}</p>
								{/if}
							</div>
						{:else}
							<div class="inline-stars-collapsed">
								{#each [1, 2, 3, 4, 5] as star (star)}
									<button
										class="star-btn faded"
										onclick={() => openInlineRating(item.message.id, star)}
									>
										☆
									</button>
								{/each}
							</div>
						{/if}
					</div>
				{:else}
					<div class="message message-collapsed">
						<button class="message-summary-btn" onclick={() => toggleExpanded(item.message.id)}>
							<div class="message-header">
								<strong>{messageTypeLabel(item.message)}</strong> &middot;
								{fmtTime(item.message.timestamp)}
								<span class="expansion-indicator">
									<span class="chevron">{expandedMessages.has(item.message.id) ? '▾' : '▸'}</span>
								</span>
							</div>
							<div class="message-summary">{messageSummary(item.message)}</div>
						</button>
						{#if expandedMessages.has(item.message.id)}
							<div class="message-content markdown-body">{@html renderMarkdown(item.message.content)}</div>
						{/if}
					</div>
				{/if}
			{:else}
				<div class="rating-card">
					<div class="message-header">
						<strong>Rating</strong> &middot; {fmtTime(item.rating.createdAt)}
					</div>
					<div class="rating-stars">{stars(item.rating.rating)}</div>
					{#if item.rating.note}
						<div class="rating-field"><strong>Note:</strong> {item.rating.note}</div>
					{/if}
					{#if item.rating.analysis}
						<div class="rating-field"><strong>Analysis:</strong> {item.rating.analysis}</div>
					{/if}
				</div>
			{/if}
		{/each}
	{/if}
{/if}

<style>
	.message {
		margin-bottom: 1rem;
		padding: 0.75rem;
		border: 1px solid #eee;
		border-radius: 4px;
	}

	.message-collapsed {
		padding: 0.5rem 0.75rem;
		background: #fafafa;
	}

	.message-header {
		font-size: 0.85rem;
		color: #666;
		margin-bottom: 0.25rem;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.expansion-indicator {
		margin-left: auto;
		color: #888;
	}

	.chevron {
		display: inline-block;
		width: 0.8rem;
	}

	.message-summary-btn {
		display: block;
		width: 100%;
		text-align: left;
		background: transparent;
		border: 0;
		padding: 0;
		cursor: pointer;
	}

	.message-summary {
		font-size: 0.9rem;
		color: #333;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.message-content {
		font-size: 0.9rem;
		margin-top: 0.35rem;
	}

	.markdown-body :global(p) {
		margin: 0.25rem 0;
	}

	.markdown-body :global(pre) {
		overflow-x: auto;
		padding: 0.5rem;
		background: #f7f7f7;
		border-radius: 4px;
	}

	.markdown-body :global(code) {
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
	}

	.rating-card {
		margin-bottom: 1rem;
		padding: 0.75rem;
		border: 2px solid #f0c040;
		border-radius: 4px;
		background: #fffbea;
	}

	.rating-stars {
		font-size: 1.1rem;
		margin-bottom: 0.25rem;
	}

	.rating-field {
		font-size: 0.9rem;
		margin-top: 0.25rem;
	}

	.inline-rating {
		margin-bottom: 1rem;
		padding-left: 0.75rem;
	}

	.inline-stars-collapsed {
		display: flex;
		gap: 2px;
	}

	.star-btn {
		background: none;
		border: none;
		cursor: pointer;
		font-size: 1.1rem;
		padding: 0;
		line-height: 1;
		color: #ccc;
	}

	.star-btn:hover,
	.star-btn.active {
		color: #f0c040;
	}

	.star-btn.faded {
		color: #ccc;
	}

	.star-btn.faded:hover,
	.inline-stars-collapsed:hover .star-btn.faded {
		color: #f0c040;
	}

	.inline-rating-expanded {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		max-width: 400px;
	}

	.inline-stars {
		display: flex;
		gap: 2px;
	}

	.inline-note {
		padding: 0.25rem 0.5rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		font-size: 0.85rem;
	}

	.inline-actions {
		display: flex;
		gap: 0.5rem;
	}

	.btn-sm {
		padding: 0.25rem 0.75rem;
		font-size: 0.85rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		background: #f0c040;
		cursor: pointer;
	}

	.btn-sm:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-cancel {
		background: #fff;
	}

	.inline-error {
		color: #c00;
		font-size: 0.85rem;
		margin: 0;
	}
</style>
