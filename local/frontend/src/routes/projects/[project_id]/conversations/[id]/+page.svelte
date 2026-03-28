<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { deleteRating, setConversationHidden } from '$lib/api';
	import { layoutStore } from '$lib/stores/layout.svelte';
	import { relationshipCache } from '$lib/stores/relationshipCache.svelte';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import type { ConversationDetail, MessageRead, Rating } from '$lib/types';
	import {
		isPlanPromptMessage,
		isUserPromptMessage,
		isQuestionMessage,
		isAnswerMessage,
		isFinalAnswerMessage,
		isStandaloneTimelineMessage,
		isDiffMessage,
		messageModel
	} from '$lib/messageUtils';
	import { combineDiffs } from '$lib/diffCombiner';
	import DiffMessageCard from '$lib/components/DiffMessageCard.svelte';
	import LogMessageCard from '$lib/components/LogMessageCard.svelte';
	import UserPromptMessageCard from '$lib/components/UserPromptMessageCard.svelte';
	import AgentMessageCard from '$lib/components/AgentMessageCard.svelte';
	import RatingMessageCard from '$lib/components/RatingMessageCard.svelte';
	import LogGroupCard from '$lib/components/LogGroupCard.svelte';
	import AgentTag from '$lib/components/AgentTag.svelte';
	import AddRatingForm from '$lib/components/AddRatingForm.svelte';
	import { singleLineTitle, shortId } from '$lib/utils';
	import { resolve } from '$app/paths';
	import Icon from '$lib/Icon.svelte';
	import type { Project } from '$lib/types';

	type TimelineItem =
		| { kind: 'message'; message: MessageRead; time: number }
		| { kind: 'rating'; rating: Rating; time: number };

	type DisplayItem =
		| TimelineItem
		| { kind: 'log-group'; id: string; messages: MessageRead[]; time: number }
		| {
				kind: 'combined-diff';
				id: string;
				content: string;
				diffMessages: MessageRead[];
				time: number;
		  };

	let { data } = $props();

	let conversation: ConversationDetail = $derived(data.conversation);
	let matchedCommitHashes: string[] = $derived(data.matchedCommitHashes ?? []);
	let matchedCommitBranches: Record<string, string> = $derived(data.commitBranches ?? {});
	let matchedCommitSubjects: Record<string, string> = $derived(data.commitSubjects ?? {});

	let hiddenSubmitting: boolean = $state(false);
	let hiddenError: string | null = $state(null);
	let localHidden: boolean = $state(false);
	let recalculatingDiffMatching: boolean = $state(false);
	let expandedMessages = new SvelteSet<string>();
	let expandedLogGroups = new SvelteSet<string>();
	let isMobileDevice = $state(false);
	let selectedMessage: MessageRead | null = $state(null);
	let selectedCombinedDiffId: string | null = $state(null);
	let selectedCombinedDiffContent: string | null = $state(null);
	let isWideMode = $state(false);
	let leftColumn: HTMLDivElement | undefined = $state();
	let mergeDiffsEnabled: boolean = $state(true);

	// Reset UI state when conversation changes (e.g. navigating parent/child links).
	let lastConversationId = '';
	$effect(() => {
		if (conversation.id !== lastConversationId) {
			lastConversationId = conversation.id;
			hiddenError = null;
			hiddenSubmitting = false;
			localHidden = conversation.hidden;
			recalculatingDiffMatching = false;
			expandedMessages.clear();
			expandedLogGroups.clear();
			selectedMessage = null;
			selectedCombinedDiffId = null;
			selectedCombinedDiffContent = null;
			leftColumn?.scrollTo(0, 0);
		}
	});

	// Local copy of ratings so we can append without mutating the load data.
	let localRatings: Rating[] = $derived(conversation.ratings);
	let projects: Project[] = $derived(data.projects ?? []);
	let projectPath = $derived(
		projects.find((project) => project.id === conversation.projectId)?.path ?? ''
	);
	function selectMessage(message: MessageRead) {
		if (selectedMessage?.id === message.id) {
			selectedMessage = null;
		} else {
			selectedMessage = message;
			selectedCombinedDiffId = null;
			selectedCombinedDiffContent = null;
		}
	}

	function selectCombinedDiff(id: string, content: string) {
		if (selectedCombinedDiffId === id) {
			selectedCombinedDiffId = null;
			selectedCombinedDiffContent = null;
		} else {
			selectedCombinedDiffId = id;
			selectedCombinedDiffContent = content;
			selectedMessage = null;
		}
	}

	function clearSelectionOnLeftBackground(e: MouseEvent) {
		if (e.target !== e.currentTarget) return;
		selectedMessage = null;
		selectedCombinedDiffId = null;
		selectedCombinedDiffContent = null;
	}

	function activateCombinedDiff(item: { id: string; content: string }, expanded: boolean) {
		if (!isWideMode && !expanded) {
			toggleExpanded(item.id);
		}
		selectCombinedDiff(item.id, item.content);
	}

	function handleCombinedDiffKeydown(
		e: KeyboardEvent,
		item: { id: string; content: string },
		expanded: boolean
	) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			activateCombinedDiff(item, expanded);
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

	function matchedCommitHref(commitHash: string): string {
		const branch = matchedCommitBranches[commitHash] || 'main';
		return resolve(
			`/projects/${encodeURIComponent(conversation.projectId)}/commits/${encodeURIComponent(branch)}/${encodeURIComponent(commitHash)}`
		);
	}

	function matchedCommitSubject(commitHash: string): string {
		return matchedCommitSubjects[commitHash] || commitHash;
	}

	async function handleDeleteRating(id: string) {
		try {
			await deleteRating(id);
			localRatings = localRatings.filter((r) => r.id !== id);
		} catch (e) {
			console.error('Failed to delete rating', e);
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
		const status = websocketStore.getJob('diff_recompute');
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
			// Questions are represented again in the answer message with full detail,
			// so rendering both is redundant in the timeline UI.
			if (isQuestionMessage(message)) continue;
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
		let run: MessageRead[] = [];

		function flushRun() {
			if (run.length === 0) return;

			const first = run[0];
			items.push({
				kind: 'log-group',
				id: `log-group-${first.id}`,
				messages: [...run],
				time: first.timestamp
			});

			const diffs = run.filter((m) => isDiffMessage(m));
			if (diffs.length > 0) {
				items.push({
					kind: 'combined-diff',
					id: `combined-diff-${diffs[0].id}`,
					content: combineDiffs(diffs, mergeDiffsEnabled),
					diffMessages: diffs,
					time: diffs[0].timestamp
				});
			}

			run = [];
		}

		for (const item of timeline) {
			if (item.kind === 'rating') {
				flushRun();
				items.push(item);
				continue;
			}
			if (isStandaloneTimelineMessage(item.message)) {
				flushRun();
				items.push(item);
				continue;
			}
			// All non-user messages (including diffs) go into the run.
			// Diffs are combined into a single combined-diff display item;
			// individual messages (including diffs) remain in the log group.
			run.push(item.message);
		}

		flushRun();
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
		isMobileDevice = window.screen.width < 900;
		if (!isMobileDevice) {
			layoutStore.fixedHeight = true;
		}
		layoutStore.hideContainer = true;
	});

	onDestroy(() => {
		layoutStore.fixedHeight = false;
		layoutStore.hideContainer = false;
	});
</script>

<div class="content" class:mobile={isMobileDevice}>
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
				href={resolve('/projects/[project_id]/conversations/[id]', {
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
				{#if item.kind === 'message' && isFinalAnswerMessage(item.message)}
					<div class="message agent-message" data-message-id={item.message.id}>
						<AgentMessageCard message={item.message} agent={conversation.agent} />
					</div>
				{:else if item.kind === 'message' && (isUserPromptMessage(item.message) || isAnswerMessage(item.message))}
					<div
						class="message user-message"
						class:plan-message={isPlanPromptMessage(item.message)}
						data-message-id={item.message.id}
					>
						<UserPromptMessageCard message={item.message} />
					</div>
				{:else if item.kind === 'combined-diff'}
					{@const combinedSelected = selectedCombinedDiffId === item.id}
					{@const combinedExpanded = isWideMode ? combinedSelected : expandedMessages.has(item.id)}
					{@const combinedInteractive = isWideMode || !combinedExpanded}
					<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
					<div
						class="message inline-diff-message"
						class:message-selected={combinedSelected}
						class:message-collapsed={!combinedExpanded}
						role={combinedInteractive ? 'button' : undefined}
						tabindex={combinedInteractive ? 0 : undefined}
						onclick={() => activateCombinedDiff(item, combinedExpanded)}
						onkeydown={combinedInteractive
							? (e: KeyboardEvent) => handleCombinedDiffKeydown(e, item, combinedExpanded)
							: undefined}
					>
						<DiffMessageCard
							label={item.diffMessages.length > 1 ? 'combined diff' : 'diff'}
							content={item.content}
							expanded={combinedExpanded}
							onToggle={!isWideMode && combinedExpanded ? () => toggleExpanded(item.id) : undefined}
						/>
					</div>
				{:else if item.kind === 'rating'}
					<div class="rating-card">
						<RatingMessageCard rating={item.rating} ondelete={handleDeleteRating} />
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
					href={resolve('/projects/[project_id]/conversations/[id]', {
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
				<AddRatingForm
					conversationId={conversation.id}
					agent={conversation.agent}
					{projectPath}
					onrating={(r) => (localRatings = [...localRatings, r])}
				/>
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
		{#if selectedCombinedDiffContent}
			<DiffMessageCard content={selectedCombinedDiffContent} expanded={true} contentOnly={true} />
		{:else if selectedMessage}
			{#if isDiffMessage(selectedMessage)}
				<DiffMessageCard
					timestamp={selectedMessage.timestamp}
					role={selectedMessage.role === 'agent' ? conversation?.agent : selectedMessage.role}
					model={messageModel(selectedMessage)}
					content={selectedMessage.content}
					expanded={true}
					contentOnly={true}
				/>
			{:else}
				<LogMessageCard
					message={selectedMessage}
					expanded={true}
					contentOnly={true}
					showRawJson={true}
				/>
			{/if}
		{:else}
			<div class="empty-state">
				<div class="empty">No agent message selected</div>
				{#if matchedCommitHashes.length > 0}
					<br />
					<br />
					<hr class="divider" style:max-width="60%" />
					<br />
					<br />
					<section class="matched-commits matched-commits-side">
						<h3>Matched commits</h3>
						<div class="matched-commits-container">
							<div class="matched-commit-list">
								{#each matchedCommitHashes as commitHash (commitHash)}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
									<a class="matched-commit-link" href={matchedCommitHref(commitHash)}>
										<div class="matched-commit-icon"><Icon name="commit" width="18px" /></div>
										<span class="matched-commit-text">
											<span class="matched-commit-subject">{matchedCommitSubject(commitHash)}</span>
											<span class="matched-commit-meta">
												<span class="matched-commit-hash">{shortId(commitHash, 8)}</span>
												<span class="matched-commit-branch"
													>{matchedCommitBranches[commitHash] || 'main'}</span
												>
											</span>
										</span>
									</a>
								{/each}
							</div>
						</div>
					</section>
				{/if}
			</div>
		{/if}
	</div>
</div>

<style>
	.content {
		display: flex;
		flex-direction: row;
		align-items: stretch;
		flex: 1;
		min-height: 100%;
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
		font-size: 1.3rem;
		justify-self: center;
		opacity: 0.4;
		text-align: center;
	}

	.empty-state {
		align-items: center;
		display: flex;
		flex-direction: column;
		justify-content: center;
		min-height: 100%;
		width: 100%;
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

	.content.mobile .column {
		overflow-y: visible;
	}

	.column .divider {
		display: block;
		background: var(--color-divider);
		height: 0.5px;
		width: 100%;
		margin: 0;
		padding: 0;
		border: 0;
	}

	.message {
		background: var(--color-background-content);
		margin-bottom: 0.5rem;
		padding: 0.6rem 1rem;
		border: 1px solid var(--color-border-medium);
		border-radius: 8px;
		line-height: 1.4em;
		margin-left: 0;
		margin-right: 10%;
	}

	.message-collapsed {
		background: var(--color-background-subtle);
		cursor: pointer;
	}

	.message-selected {
		border-color: var(--color-selected-border);
		background: var(--color-selected-bg);
	}

	.message-selected:hover {
		border-color: var(--color-selected-border-hover);
		background: var(--color-selected-bg-hover);
	}

	.message-collapsed:hover {
		border-color: var(--accent-color);
		background: var(--accent-color-ultralight);
	}

	.message.user-message {
		background: var(--color-prompt-background);
		border: 1px solid var(--color-prompt-border);
		color: var(--color-text);
		margin-left: 10%;
		margin-right: 0rem;
	}

	.message.user-message.plan-message {
		background: var(--color-relationship-highlight);
		border-color: var(--color-relationship-icon);
		color: var(--color-relationship-foreground);
	}

	.message.agent-message {
		color: var(--color-text);
		margin-left: 0;
		margin-right: 10%;
		padding: 0;
		border: none;
		background: none;
	}

	h2 {
		font-size: 1.3rem;
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
		color: var(--color-text-secondary);
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.models-label {
		font-weight: 600;
		color: var(--color-text-secondary);
	}

	.models-list {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem 1rem;
		list-style: none;
		margin: 0;
		padding: 0;
		color: var(--color-text-tertiary);
	}

	.rating-card {
		background: var(--color-rating-background);
		border-radius: 8px;
		border: 1px solid var(--color-rating-border);
		margin-bottom: 1rem;
		padding: 0.75rem;
		color: var(--color-text);
	}

	.rating-input {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.log-group {
		background: none;
		border: 1px solid transparent;
		padding: 0;
		width: fit-content;
		border-radius: 5px;
		max-width: calc(100% - 10%);
	}

	.log-group.message-collapsed:hover {
		background: var(--accent-color-ultralight);
		border: 1px solid var(--accent-color);
	}

	.log-group.message-collapsed:hover :global(.log-group-header strong) {
		color: var(--accent-color);
	}

	.conversation-visibility {
		align-items: center;
		display: flex;
		gap: 0.6rem;
		margin: 1rem 0 1rem 0;
	}

	.visibility-btn {
		background: var(--color-visibility-btn-bg);
	}

	.recalculate-message {
		color: var(--color-text-secondary);
		font-size: 0.85rem;
	}

	.hidden-banner {
		align-items: center;
		background: var(--color-hidden-bg);
		border: 1px solid var(--color-hidden-border);
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
		background: var(--color-hidden-btn-bg);
	}

	.conversation-link {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.75rem 0.75rem;
		border-radius: 6px;
		font-size: 1rem;
		text-decoration: none;
		margin-bottom: 0.5rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		background: var(--color-relationship-highlight);
		border: 1px solid var(--color-relationship-icon);
		color: var(--color-relationship-foreground);
	}

	.conversation-link:hover {
		filter: brightness(0.97);
	}

	.link-icon {
		width: 16px;
		height: 16px;
		flex-shrink: 0;
		color: var(--color-relationship-foreground);
	}

	.matched-commits {
		margin: 0;
	}

	.matched-commits-side {
		display: flex;
		flex-direction: column;
		align-items: center;
		width: 100%;
	}

	.matched-commits h3 {
		font-size: 0.9rem;
		font-weight: normal;
		margin: 0 0 0.75rem 0;
		text-transform: uppercase;
		color: var(--color-text-tertiary);
	}

	.matched-commits-container {
		width: min(60%, 100%);
	}

	.matched-commit-list {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
	}

	.matched-commit-link {
		align-items: center;
		background: var(--color-background-subtle);
		border: 1px solid var(--color-border-medium);
		border-radius: 6px;
		color: var(--color-link-body);
		display: flex;
		gap: 0.75rem;
		padding: 0.55rem 0.75rem;
		text-decoration: none;
	}

	.matched-commit-icon {
		color: var(--color-text);
	}

	.matched-commit-link:hover {
		border-color: var(--accent-color);
		background: var(--accent-color-ultralight);
	}

	.matched-commit-text {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 0.2rem;
		min-width: 0;
	}

	.matched-commit-subject {
		color: var(--color-text);
		display: block;
		font-size: 0.9rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.matched-commit-meta {
		align-items: center;
		color: var(--color-text-secondary);
		display: flex;
		gap: 0.5rem;
		min-width: 0;
	}

	.matched-commit-hash {
		font-family:
			ui-monospace,
			SFMono-Regular,
			SF Mono,
			Menlo,
			monospace;
		font-size: 0.8rem;
	}

	.matched-commit-branch {
		font-size: 0.8rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
