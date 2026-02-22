<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { getConversation, createRating } from '$lib/api';
	import { layoutStore } from '$lib/stores/layout.svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import type { ConversationDetail, MessageRead, Rating } from '$lib/types';
	import { isUserPromptMessage, isDiffMessage, messageModel } from '$lib/messageUtils';
	import DiffMessageCard from '$lib/components/DiffMessageCard.svelte';
	import LogMessageCard from '$lib/components/LogMessageCard.svelte';
	import UserPromptMessageCard from '$lib/components/UserPromptMessageCard.svelte';
	import RatingMessageCard from '$lib/components/RatingMessageCard.svelte';
	import LogGroupCard from '$lib/components/LogGroupCard.svelte';
	import AgentTag from '$lib/components/AgentTag.svelte';

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
	let selectedMessage: MessageRead | null = $state(null);
	let isWideMode = $state(false);
	let wideModeQuery: MediaQueryList | null = null;

	function selectMessage(message: MessageRead) {
		selectedMessage = selectedMessage?.id === message.id ? null : message;
	}

	function clearSelectionOnLeftBackground(e: MouseEvent) {
		if (e.target !== e.currentTarget) return;
		selectedMessage = null;
	}

	function updateWideMode(query: MediaQueryList | MediaQueryListEvent) {
		isWideMode = query.matches;
	}

	function activateMessage(message: MessageRead, expanded: boolean) {
		if (!isWideMode && !expanded) {
			toggleExpanded(message.id);
		}
		selectMessage(message);
	}

	function handleMessageActivateKeydown(e: KeyboardEvent, message: MessageRead, expanded: boolean) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			activateMessage(message, expanded);
		}
	}

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
		layoutStore.fixedHeight = true;
		wideModeQuery = window.matchMedia('(min-width: 1024px)');
		updateWideMode(wideModeQuery);
		wideModeQuery.addEventListener('change', updateWideMode);
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

	onDestroy(() => {
		wideModeQuery?.removeEventListener('change', updateWideMode);
		layoutStore.fixedHeight = false;
	});
</script>

<div class="content">
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="column left" onclick={clearSelectionOnLeftBackground}>
		{#if loading}
			<p class="loading">Loading conversation...</p>
		{:else if error}
			<p class="error">{error}</p>
		{:else if conversation}
			<h2>{conversation.title || conversation.id}</h2>
			<p class="agent-header">Agent: <AgentTag agent={conversation.agent} /></p>
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
						<div class="message user-message" data-message-id={item.message.id}>
							<UserPromptMessageCard message={item.message} />
						</div>
					{:else if item.kind === 'message' && isDiffMessage(item.message)}
						{@const messageSelected = selectedMessage?.id === item.message.id}
						{@const diffExpanded = isWideMode
							? messageSelected
							: expandedMessages.has(item.message.id)}
						{@const messageInteractive = isWideMode || !diffExpanded}
						<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
						<div
							class="message inline-diff-message"
							class:message-selected={messageSelected}
							class:message-collapsed={!diffExpanded}
							role={messageInteractive ? 'button' : undefined}
							tabindex={messageInteractive ? 0 : undefined}
							onclick={() => activateMessage(item.message, diffExpanded)}
							onkeydown={messageInteractive
								? (e: KeyboardEvent) => handleMessageActivateKeydown(e, item.message, diffExpanded)
								: undefined}
						>
							<DiffMessageCard
								timestamp={item.message.timestamp}
								role={item.message.role === 'agent' ? conversation.agent : item.message.role}
								model={messageModel(item.message)}
								content={item.message.content}
								expanded={diffExpanded}
								subtleAgentTag={true}
								onToggle={!isWideMode && diffExpanded
									? () => toggleExpanded(item.message.id)
									: undefined}
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
								agent={conversation.agent}
								expanded={groupExpanded}
								subtleAgentTag={true}
								wideMode={isWideMode}
								selectedMessageId={selectedMessage?.id ?? null}
								{expandedMessages}
								onToggleMessage={toggleExpanded}
								onSelectMessage={selectMessage}
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
	</div>
	<hr class="divider" />
	<div class="column right">
		{#if selectedMessage}
			{#if isDiffMessage(selectedMessage)}
				<!-- <div class="message right-message"> -->
				<DiffMessageCard
					timestamp={selectedMessage.timestamp}
					role={selectedMessage.role === 'agent' ? conversation?.agent : selectedMessage.role}
					model={messageModel(selectedMessage)}
					content={selectedMessage.content}
					expanded={true}
					contentOnly={true}
				/>
				<!-- </div> -->
			{:else}
				<!-- <div class="message right-message"> -->
				<LogMessageCard message={selectedMessage} expanded={true} contentOnly={true} />
				<!-- </div> -->
			{/if}
		{:else}
			<div class="empty">No message selected</div>
		{/if}
	</div>
</div>

<style>
	.content {
		display: flex;
		flex-direction: row;
		align-items: stretch;
		flex: 1;
		min-height: 0;
	}

	.column {
		box-sizing: border-box;
		flex: 1;
		max-width: 50%;
		overflow-y: scroll;
		position: relative;
	}

	.column.left {
		padding: 0 1rem;
	}

	.content .divider {
		display: block;
		background: var(--color-divider);
		width: 0.5px;
		margin: 0;
		padding: 0;
		border: 0;
	}

	.column.right {
		padding: 0.5rem;
	}

	.column.right .empty {
		/*align-self: center;*/
		justify-self: center;
		margin-top: 40vh;
		opacity: 0.4;
		font-size: 1.3rem;
	}

	.column.left .inline-diff-message :global(.message-content),
	.column.left :global(.log-item .message-content) {
		display: none;
	}

	@media (max-width: 1023px) {
		.column {
			max-width: 100%;
		}
		.column.right {
			display: none;
		}
		.column.left .inline-diff-message :global(.message-content),
		.column.left :global(.log-item .message-content) {
			display: block;
		}
	}

	.message {
		background: #ffffff;
		margin-bottom: 0.5rem;
		padding: 0.6rem 1rem;
		border: 1px solid #dddddd;
		border-radius: 8px;
		line-height: 1.4em;
	}

	.message-collapsed {
		background: #fafafa;
		cursor: pointer;
	}

	.message-selected {
		border-color: #84b8ff;
		background: #eff6ff;
	}

	.message-selected:hover {
		border-color: #5a9cff;
		background: #e4f0ff;
	}

	.message-collapsed:hover {
		border-color: var(--accent-color);
		background: var(--accent-color-ultralight);
	}

	.message.user-message {
		background: var(--color-prompt-background);
		border: 1px solid var(--color-prompt-border);
		color: #444;
	}

	.agent-header {
		display: flex;
		align-items: center;
		gap: 0.4rem;
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
		background: var(--color-rating-background);
		border-radius: 8px;
		border: 1px solid var(--color-rating-border);
		margin-bottom: 1rem;
		padding: 0.75rem;
		color: #444;
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
		border: 1px solid transparent;
		margin-left: 1rem;
		padding: 0;
		width: fit-content;
		border-radius: 5px;
	}

	.log-group.message-collapsed:hover {
		background: var(--accent-color-ultralight);
		border: 1px solid var(--accent-color);
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
