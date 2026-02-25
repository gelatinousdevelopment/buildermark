<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { createRating, setConversationHidden } from '$lib/api';
	import { layoutStore } from '$lib/stores/layout.svelte';
	import { relationshipCache } from '$lib/stores/relationshipCache.svelte';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import type { ConversationDetail, MessageRead, Rating } from '$lib/types';
	import { isUserPromptMessage, isDiffMessage, messageModel } from '$lib/messageUtils';
	import DiffMessageCard from '$lib/components/DiffMessageCard.svelte';
	import LogMessageCard from '$lib/components/LogMessageCard.svelte';
	import UserPromptMessageCard from '$lib/components/UserPromptMessageCard.svelte';
	import RatingMessageCard from '$lib/components/RatingMessageCard.svelte';
	import LogGroupCard from '$lib/components/LogGroupCard.svelte';
	import AgentTag from '$lib/components/AgentTag.svelte';
	import { singleLineTitle, shortId } from '$lib/utils';
	import { resolve } from '$app/paths';

	type TimelineItem =
		| { kind: 'message'; message: MessageRead; time: number }
		| { kind: 'rating'; rating: Rating; time: number };

	type DisplayItem =
		| TimelineItem
		| { kind: 'log-group'; id: string; messages: MessageRead[]; time: number };

	let { data } = $props();

	let conversation: ConversationDetail = $derived(data.conversation);

	let bottomRatingValue: number = $state(0);
	let bottomNote: string = $state('');
	let bottomSubmitting: boolean = $state(false);
	let bottomError: string | null = $state(null);
	let hiddenSubmitting: boolean = $state(false);
	let hiddenError: string | null = $state(null);
	let localHidden: boolean = $state(false);
	let recalculatingDiffMatching: boolean = $state(false);
	let expandedMessages = new SvelteSet<string>();
	let expandedLogGroups = new SvelteSet<string>();
	let selectedMessage: MessageRead | null = $state(null);
	let isWideMode = $state(false);
	let wideModeQuery: MediaQueryList | null = null;
	let leftColumn: HTMLDivElement | undefined = $state();

	// Reset UI state when conversation changes (e.g. navigating parent/child links).
	let lastConversationId = '';
	$effect(() => {
		if (conversation.id !== lastConversationId) {
			lastConversationId = conversation.id;
			bottomRatingValue = 0;
			bottomNote = '';
			bottomError = null;
			hiddenError = null;
			hiddenSubmitting = false;
			localHidden = conversation.hidden;
			recalculatingDiffMatching = false;
			expandedMessages.clear();
			expandedLogGroups.clear();
			selectedMessage = null;
			leftColumn?.scrollTo(0, 0);
		}
	});

	// Local copy of ratings so we can append without mutating the load data.
	let localRatings: Rating[] = $derived(conversation.ratings);

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
		if (bottomRatingValue < 1) return;
		bottomSubmitting = true;
		bottomError = null;
		try {
			const newRating = await createRating(conversation.id, bottomRatingValue, bottomNote);
			localRatings = [...localRatings, newRating];
			bottomRatingValue = 0;
			bottomNote = '';
		} catch (e) {
			bottomError = e instanceof Error ? e.message : 'Failed to submit rating';
		} finally {
			bottomSubmitting = false;
		}
	}

	async function toggleConversationHidden() {
		hiddenSubmitting = true;
		hiddenError = null;
		try {
			const updated = await setConversationHidden(conversation.id, !localHidden);
			localHidden = updated.hidden;
			if (updated.hidden) {
				recalculatingDiffMatching = true;
			}
			relationshipCache.clearProject(conversation.projectId);
		} catch (e) {
			hiddenError = e instanceof Error ? e.message : 'Failed to update hidden state';
		} finally {
			hiddenSubmitting = false;
		}
	}

	$effect(() => {
		if (!recalculatingDiffMatching) return;
		const status = websocketStore.importStatus;
		if (!status) return;
		if (!status.message.includes(conversation.id)) return;
		if (status.state === 'complete' || status.state === 'error') {
			recalculatingDiffMatching = false;
		}
	});

	let timeline: TimelineItem[] = $derived.by(() => {
		const items: TimelineItem[] = [];

		// Messages are pre-filtered server-side.
		// Subtract 1s from user prompt timestamps so they sort before
		// the model messages that share the same second-level timestamp.
		for (const message of conversation.messages) {
			const adjust = isUserPromptMessage(message) ? 1000 : 0;
			items.push({ kind: 'message', message, time: message.timestamp - adjust });
		}

		// Ratings have matchedTimestamp from server-side rating matching.
		for (const rating of localRatings) {
			const time = rating.matchedTimestamp ?? rating.createdAt;
			items.push({ kind: 'rating', rating, time });
		}

		items.sort((a, b) => a.time - b.time);
		return items;
	});

	let conversationModels: string[] = $derived.by(() => {
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

	onMount(() => {
		layoutStore.fixedHeight = true;
		wideModeQuery = window.matchMedia('(min-width: 1024px)');
		updateWideMode(wideModeQuery);
		wideModeQuery.addEventListener('change', updateWideMode);
	});

	onDestroy(() => {
		wideModeQuery?.removeEventListener('change', updateWideMode);
		layoutStore.fixedHeight = false;
	});
</script>

<div class="content">
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="column left" bind:this={leftColumn} onclick={clearSelectionOnLeftBackground}>
		<h2>{(conversation.title && singleLineTitle(conversation.title)) || conversation.id}</h2>
		<p class="agent-header">Agent: <AgentTag agent={conversation.agent} /></p>
		{#if localHidden}
			<div class="hidden-banner">
				<span>This conversation is hidden.</span>
				<div class="hidden-banner-actions">
					<button
						class="bordered small hidden-banner-btn"
						disabled={hiddenSubmitting}
						onclick={toggleConversationHidden}
						>{hiddenSubmitting ? 'Saving...' : 'Mark as not hidden'}</button
					>
					{#if recalculatingDiffMatching}
						<span class="recalculate-message">Reclaculating diff matching...</span>
					{/if}
				</div>
			</div>
		{/if}
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

		{#if conversation.parentConversationId}
			<a
				class="conversation-link parent-link"
				href={resolve('/local/projects/[project_id]/conversations/[id]', {
					project_id: conversation.projectId,
					id: conversation.parentConversationId
				})}
			>
				<svg class="link-icon" viewBox="0 0 16 16" fill="currentColor"
					><path d="M8 3l4 4H9v5H7V7H4l4-4z" /></svg
				>
				Parent conversation
			</a>
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
		{#if conversation.childConversations && conversation.childConversations.length > 0}
			{#each conversation.childConversations as child (child.id)}
				<a
					class="conversation-link child-link"
					href={resolve('/local/projects/[project_id]/conversations/[id]', {
						project_id: conversation.projectId,
						id: child.id
					})}
				>
					<svg class="link-icon" viewBox="0 0 16 16" fill="currentColor"
						><path d="M8 13l4-4H9V4H7v5H4l4 4z" /></svg
					>
					Child conversation: {(child.title && singleLineTitle(child.title)) || shortId(child.id)}
				</a>
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
						class="bordered small"
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
		{#if !localHidden}
			<div class="conversation-visibility">
				<button
					class="bordered small visibility-btn"
					disabled={hiddenSubmitting}
					onclick={toggleConversationHidden}
				>
					{#if hiddenSubmitting}
						Saving...
					{:else}
						Hide conversation
					{/if}
				</button>
				{#if recalculatingDiffMatching}
					<span class="recalculate-message">Reclaculating diff matching...</span>
				{/if}
				{#if hiddenError}
					<p class="inline-error">{hiddenError}</p>
				{/if}
			</div>
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

	h2 {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
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
		max-width: calc(100% - 1rem);
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

	.inline-error {
		color: #c00;
		font-size: 0.85rem;
		margin: 0;
	}

	.conversation-visibility {
		align-items: center;
		display: flex;
		gap: 0.6rem;
		margin: 1rem 0 1rem 0;
	}

	.visibility-btn {
		background: #e8e8e8;
	}

	.recalculate-message {
		color: #666;
		font-size: 0.85rem;
	}

	.hidden-banner {
		align-items: center;
		background: #ececec;
		border: 1px solid #d4d4d4;
		border-radius: 6px;
		display: flex;
		justify-content: space-between;
		margin: 0.25rem 0 0.75rem 0;
		padding: 0.5rem 0.7rem;
	}

	.hidden-banner-actions {
		align-items: center;
		display: flex;
		gap: 0.55rem;
	}

	.hidden-banner-btn {
		background: #ddd;
	}

	.conversation-link {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.85rem;
		text-decoration: none;
		margin-bottom: 0.5rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.parent-link {
		background: var(--color-prompt-background, #f0f4ff);
		border: 1px solid var(--color-prompt-border, #d0d8e8);
		color: #555;
	}

	.child-link {
		background: var(--color-rating-background, #fffbf0);
		border: 1px solid var(--color-rating-border, #e8e0c8);
		color: #555;
	}

	.conversation-link:hover {
		border-color: var(--accent-color);
		color: var(--accent-color);
	}

	.link-icon {
		width: 14px;
		height: 14px;
		flex-shrink: 0;
		opacity: 0.5;
	}
</style>
