<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getConversation, createRating } from '$lib/api';
	import { SvelteSet } from 'svelte/reactivity';
	import type { ConversationDetail, MessageRead, Rating } from '$lib/types';
	import { isUserPromptMessage, isDiffMessage, messageModel } from '$lib/messageUtils';
	import DiffMessageCard from '$lib/components/DiffMessageCard.svelte';
	import UserPromptMessageCard from '$lib/components/UserPromptMessageCard.svelte';
	import RatingMessageCard from '$lib/components/RatingMessageCard.svelte';
	import LogGroupCard from '$lib/components/LogGroupCard.svelte';

	type TimelineItem =
		| { kind: 'message'; message: MessageRead; time: number }
		| { kind: 'rating'; rating: Rating; time: number };

	type DisplayItem =
		| TimelineItem
		| { kind: 'log-group'; id: string; messages: MessageRead[]; time: number };

	let conversation: ConversationDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	let bottomRatingValue: number = $state(0);
	let bottomNote: string = $state('');
	let bottomSubmitting: boolean = $state(false);
	let bottomError: string | null = $state(null);
	let expandedMessages = new SvelteSet<string>();
	let expandedLogGroups = new SvelteSet<string>();

	function toggleExpanded(messageId: string) {
		if (expandedMessages.has(messageId)) {
			expandedMessages.delete(messageId);
			return;
		}
		expandedMessages.add(messageId);
	}

	function toggleLogGroup(groupId: string) {
		if (expandedLogGroups.has(groupId)) {
			expandedLogGroups.delete(groupId);
			return;
		}
		expandedLogGroups.add(groupId);
	}

	async function submitBottomRating() {
		if (!conversation || bottomRatingValue < 1) return;
		bottomSubmitting = true;
		bottomError = null;
		try {
			const newRating = await createRating(conversation.id, bottomRatingValue, bottomNote);
			conversation.ratings = [...conversation.ratings, newRating];
			bottomRatingValue = 0;
			bottomNote = '';
		} catch (e) {
			bottomError = e instanceof Error ? e.message : 'Failed to submit rating';
		} finally {
			bottomSubmitting = false;
		}
	}

	let timeline: TimelineItem[] = $derived.by(() => {
		if (!conversation) return [];

		const items: TimelineItem[] = [];

		// Messages are pre-filtered server-side.
		// Subtract 1s from user prompt timestamps so they sort before
		// the model messages that share the same second-level timestamp.
		for (const message of conversation.messages) {
			const adjust = isUserPromptMessage(message) ? 1000 : 0;
			items.push({ kind: 'message', message, time: message.timestamp - adjust });
		}

		// Ratings have matchedTimestamp from server-side rating matching.
		for (const rating of conversation.ratings) {
			const time = rating.matchedTimestamp ?? new Date(rating.createdAt).getTime();
			items.push({ kind: 'rating', rating, time });
		}

		items.sort((a, b) => a.time - b.time);
		return items;
	});

	let conversationModels: string[] = $derived.by(() => {
		if (!conversation) return [];
		const seen = new SvelteSet<string>();
		const models: string[] = [];
		for (const message of conversation.messages) {
			const model = messageModel(message);
			if (!model || seen.has(model)) continue;
			seen.add(model);
			models.push(model);
		}
		return models;
	});

	let displayItems: DisplayItem[] = $derived.by(() => {
		const items: DisplayItem[] = [];
		let logRun: MessageRead[] = [];

		function flushLogRun() {
			if (logRun.length === 0) return;
			const first = logRun[0];
			items.push({
				kind: 'log-group',
				id: `log-group-${first.id}`,
				messages: [...logRun],
				time: first.timestamp
			});
			logRun = [];
		}

		for (const item of timeline) {
			if (item.kind === 'rating') {
				flushLogRun();
				items.push(item);
				continue;
			}
			if (isUserPromptMessage(item.message) || isDiffMessage(item.message)) {
				flushLogRun();
				items.push(item);
				continue;
			}
			logRun.push(item.message);
		}

		flushLogRun();
		return items;
	});

	let hasRatingAfterLastUser = $derived.by(() => {
		let lastUserIdx = -1;
		for (let i = timeline.length - 1; i >= 0; i--) {
			const item = timeline[i];
			if (item.kind === 'message' && isUserPromptMessage(item.message)) {
				lastUserIdx = i;
				break;
			}
		}
		if (lastUserIdx === -1) return false;
		for (let i = lastUserIdx + 1; i < timeline.length; i++) {
			if (timeline[i].kind === 'rating') return true;
		}
		return false;
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

{#if loading}
	<p class="loading">Loading conversation...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if conversation}
	<h2>{conversation.title || conversation.id}</h2>
	<p>Agent: {conversation.agent}</p>
	{#if conversationModels.length > 0}
		<div class="models-summary">
			<span class="models-label">Models:</span>
			<ul class="models-list">
				{#each conversationModels as model (model)}
					<li>{model}</li>
				{/each}
			</ul>
		</div>
	{/if}

	{#if timeline.length === 0}
		<p>No messages or ratings.</p>
	{:else}
		{#each displayItems as item (item.kind === 'message' ? item.message.id : item.kind === 'rating' ? item.rating.id : item.id)}
			{#if item.kind === 'message' && isUserPromptMessage(item.message)}
				<div class="message">
					<UserPromptMessageCard message={item.message} />
				</div>
			{:else if item.kind === 'message' && isDiffMessage(item.message)}
				{@const diffExpanded = expandedMessages.has(item.message.id)}
				<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
				<div
					class="message"
					class:message-collapsed={!diffExpanded}
					role={!diffExpanded ? 'button' : undefined}
					tabindex={!diffExpanded ? 0 : undefined}
					onclick={!diffExpanded ? () => toggleExpanded(item.message.id) : undefined}
					onkeydown={!diffExpanded
						? (e: KeyboardEvent) => {
								if (e.key === 'Enter' || e.key === ' ') {
									e.preventDefault();
									toggleExpanded(item.message.id);
								}
							}
						: undefined}
				>
					<DiffMessageCard
						timestamp={item.message.timestamp}
						model={messageModel(item.message)}
						content={item.message.content}
						expanded={diffExpanded}
						onToggle={diffExpanded ? () => toggleExpanded(item.message.id) : undefined}
					/>
				</div>
			{:else if item.kind === 'rating'}
				<div class="rating-card">
					<RatingMessageCard rating={item.rating} />
				</div>
			{:else if item.kind === 'log-group'}
				{@const groupExpanded = expandedLogGroups.has(item.id)}
				<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
				<div
					class="message log-group"
					class:message-collapsed={!groupExpanded}
					role={!groupExpanded ? 'button' : undefined}
					tabindex={!groupExpanded ? 0 : undefined}
					onclick={!groupExpanded ? () => toggleLogGroup(item.id) : undefined}
					onkeydown={!groupExpanded
						? (e: KeyboardEvent) => {
								if (e.key === 'Enter' || e.key === ' ') {
									e.preventDefault();
									toggleLogGroup(item.id);
								}
							}
						: undefined}
				>
					<LogGroupCard
						messages={item.messages}
						expanded={groupExpanded}
						{expandedMessages}
						onToggleMessage={toggleExpanded}
						onToggle={groupExpanded ? () => toggleLogGroup(item.id) : undefined}
					/>
				</div>
			{/if}
		{/each}
	{/if}
	{#if !hasRatingAfterLastUser}
		<div class="rating-card rating-input">
			<div class="rating-input-header">
				<strong>Add rating</strong>
			</div>
			<div class="inline-stars">
				{#each [1, 2, 3, 4, 5] as star (star)}
					<button
						class="star-btn"
						class:active={star <= bottomRatingValue}
						onclick={() => (bottomRatingValue = star)}
					>
						{star <= bottomRatingValue ? '★' : '☆'}
					</button>
				{/each}
			</div>
			<input
				type="text"
				class="inline-note"
				placeholder="Optional note..."
				bind:value={bottomNote}
			/>
			<div class="inline-actions">
				<button
					class="btn-sm"
					disabled={bottomSubmitting || bottomRatingValue < 1}
					onclick={submitBottomRating}
				>
					{bottomSubmitting ? 'Submitting...' : 'Submit'}
				</button>
			</div>
			{#if bottomError}
				<p class="inline-error">{bottomError}</p>
			{/if}
		</div>
	{/if}
{/if}

<style>
	.message {
		background: #fffff4;
		margin-bottom: 0.5rem;
		padding: 0.75rem;
		border: 1px solid #ddddaa;
		border-radius: 4px;
	}

	.message-collapsed {
		padding: 0.5rem 0.75rem;
		background: #fafafa;
		cursor: pointer;
	}

	.message-collapsed:hover {
		border-color: var(--accent-color);
		background: var(--accent-color-ultralight);
	}

	.models-summary {
		margin-bottom: 0.85rem;
		font-size: 0.85rem;
		color: #666;
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.models-label {
		font-weight: 600;
		color: #555;
	}

	.models-list {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem 1rem;
		list-style: none;
		margin: 0;
		padding: 0;
		color: #888;
	}

	.rating-card {
		margin-bottom: 1rem;
		padding: 0.75rem;
		border: 2px solid #f0c040;
		border-radius: 4px;
		background: #fffbea;
	}

	.rating-input {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.rating-input-header {
		font-size: 0.85rem;
		color: #666;
	}

	.log-group {
		background: none;
		border: none;
		margin-left: 1rem;
		padding: 0.3rem;
		width: fit-content;
	}

	.log-group :global(.log-group-header strong) {
		color: #828282;
		font-size: 0.9rem;
		font-weight: normal;
	}

	.log-group.message-collapsed:hover {
		background: var(--accent-color-ultralight);
	}

	.log-group.message-collapsed:hover :global(.log-group-header strong) {
		color: var(--accent-color);
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

	.inline-error {
		color: #c00;
		font-size: 0.85rem;
		margin: 0;
	}
</style>
