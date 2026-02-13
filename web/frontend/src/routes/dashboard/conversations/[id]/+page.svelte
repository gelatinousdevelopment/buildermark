<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getConversation, createRating } from '$lib/api';
	import { stars, fmtTime } from '$lib/utils';
	import { marked } from 'marked';
	import { html as diffToHtml } from 'diff2html';
	import 'diff2html/bundles/css/diff2html.min.css';
	import { SvelteSet } from 'svelte/reactivity';
	import type { ConversationDetail, MessageRead, Rating } from '$lib/types';

	type TimelineItem =
		| { kind: 'message'; message: MessageRead; time: number }
		| { kind: 'rating'; rating: Rating; time: number };

	type DisplayItem =
		| TimelineItem
		| { kind: 'log-group'; id: string; messages: MessageRead[]; time: number };

	let conversation: ConversationDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	let inlineRatingMessageId: string | null = $state(null);
	let inlineRatingValue: number = $state(0);
	let inlineNote: string = $state('');
	let inlineSubmitting: boolean = $state(false);
	let inlineError: string | null = $state(null);
	let bottomRatingValue: number = $state(0);
	let bottomNote: string = $state('');
	let bottomSubmitting: boolean = $state(false);
	let bottomError: string | null = $state(null);
	let expandedMessages = $state(new SvelteSet<string>());
	let expandedLogGroups = $state(new SvelteSet<string>());

	function isUserPromptMessage(message: MessageRead): boolean {
		if (message.role !== 'user') return false;
		const trimmed = message.content.trimStart();
		if (trimmed.startsWith('/zrate') || trimmed.startsWith('$zrate')) return false;
		return true;
	}

	function isDiffMessage(message: MessageRead): boolean {
		const trimmed = message.content.trimStart();
		if (trimmed.startsWith('```diff') || trimmed.startsWith('diff --git ')) return true;
		try {
			const obj = JSON.parse(message.rawJson) as Record<string, unknown>;
			return obj.source === 'derived_diff';
		} catch {
			return false;
		}
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

	function extractDiffText(content: string): string {
		let text = content.trim();
		if (text.startsWith('```diff')) {
			text = text.slice('```diff'.length).trimStart();
			if (text.endsWith('```')) text = text.slice(0, -3).trimEnd();
		}

		const gitIdx = text.indexOf('diff --git ');
		if (gitIdx >= 0) return text.slice(gitIdx).trim();

		const oldIdx = text.indexOf('\n--- ');
		if (oldIdx >= 0) return text.slice(oldIdx + 1).trim();

		if (text.startsWith('--- ')) return text;
		return '';
	}

	function renderDiff(content: string): string {
		const diffText = extractDiffText(content);
		if (!diffText) return renderMarkdown(content);
		try {
			return diffToHtml(diffText, {
				drawFileList: true,
				matching: 'lines',
				outputFormat: 'line-by-line'
			});
		} catch {
			return renderMarkdown(content);
		}
	}

	function diffStats(message: MessageRead): { files: number; added: number; removed: number } {
		const diffText = extractDiffText(message.content);
		if (!diffText) return { files: 0, added: 0, removed: 0 };

		const lines = diffText.split('\n');
		let files = 0;
		let added = 0;
		let removed = 0;

		for (const line of lines) {
			if (line.startsWith('diff --git ')) {
				files++;
				continue;
			}
			if (line.startsWith('+++ ') || line.startsWith('--- ')) {
				continue;
			}
			if (line.startsWith('+')) {
				added++;
				continue;
			}
			if (line.startsWith('-')) {
				removed++;
			}
		}

		if (files === 0 && (added > 0 || removed > 0)) files = 1;
		return { files, added, removed };
	}

	function diffStatsLabel(message: MessageRead): string {
		const stats = diffStats(message);
		const fileLabel = stats.files === 1 ? 'file' : 'files';
		return `${stats.files} ${fileLabel}, +${stats.added} -${stats.removed}`;
	}

	function firstLine(s: string): string {
		return s.replace(/\s+/g, ' ').trim();
	}

	function messageTypeLabel(message: MessageRead): string {
		if (isDiffMessage(message)) return 'diff';
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
		if (isDiffMessage(message)) {
			const diffText = extractDiffText(message.content);
			const firstDiffLine = diffText.split('\n').find((line) => line.startsWith('diff --git '));
			if (firstDiffLine) return firstDiffLine;
		}
		const line = firstLine(message.content);
		if (!line) return `[${messageTypeLabel(message)}]`;
		return line.length > 120 ? `${line.slice(0, 117)}...` : line;
	}

	function messageModel(message: MessageRead): string {
		return typeof message.model === 'string' ? message.model.trim() : '';
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

	function groupModelLabel(messages: MessageRead[]): string {
		const models = new SvelteSet<string>();
		for (const message of messages) {
			const model = messageModel(message);
			if (model) models.add(model);
		}
		if (models.size === 1) return Array.from(models)[0] ?? 'model';
		return 'model';
	}

	let displayItems: DisplayItem[] = $derived.by(() => {
		const items: DisplayItem[] = [];
		let logRun: MessageRead[] = [];

		function flushLogRun() {
			if (logRun.length > 1) {
				const first = logRun[0];
				items.push({
					kind: 'log-group',
					id: `log-group-${first.id}`,
					messages: [...logRun],
					time: first.timestamp
				});
			} else if (logRun.length === 1) {
				const only = logRun[0];
				items.push({ kind: 'message', message: only, time: only.timestamp });
			}
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

	let hasBottomRating = $derived.by(() => {
		if (timeline.length === 0) return false;
		return timeline[timeline.length - 1]?.kind === 'rating';
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
			{#if item.kind === 'message'}
				{#if isUserPromptMessage(item.message)}
					<div class="message">
						<div class="message-header">
							<strong>{item.message.role}</strong> &middot; {fmtTime(item.message.timestamp)}
							{#if messageModel(item.message)}
								<span class="message-model">{messageModel(item.message)}</span>
							{/if}
							{#if isDiffMessage(item.message)}
								<span class="message-diff-stats">{diffStatsLabel(item.message)}</span>
							{/if}
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
								{#if messageModel(item.message)}
									<span class="message-model">{messageModel(item.message)}</span>
								{/if}
								{#if isDiffMessage(item.message)}
									<span class="message-diff-stats">{diffStatsLabel(item.message)}</span>
								{/if}
								<span class="expansion-indicator">
									<span class="chevron">{expandedMessages.has(item.message.id) ? '▾' : '▸'}</span>
								</span>
							</div>
							<div class="message-summary">{messageSummary(item.message)}</div>
						</button>
						{#if expandedMessages.has(item.message.id)}
							{#if isDiffMessage(item.message)}
								<div class="message-content diff-body">{@html renderDiff(item.message.content)}</div>
							{:else}
								<div class="message-content markdown-body">{@html renderMarkdown(item.message.content)}</div>
							{/if}
						{/if}
					</div>
				{/if}
			{:else if item.kind === 'log-group'}
				<div class="message message-collapsed log-group">
					<button class="message-summary-btn" onclick={() => toggleLogGroup(item.id)}>
						<div class="message-header">
							<strong>{item.messages.length} logs from {groupModelLabel(item.messages)}</strong>
							<span class="expansion-indicator">
								<span class="chevron">{expandedLogGroups.has(item.id) ? '▾' : '▸'}</span>
							</span>
						</div>
					</button>
					{#if expandedLogGroups.has(item.id)}
						<div class="log-group-items">
							{#each item.messages as logMessage (logMessage.id)}
								<div class="message message-collapsed">
									<button class="message-summary-btn" onclick={() => toggleExpanded(logMessage.id)}>
										<div class="message-header">
											<strong>{messageTypeLabel(logMessage)}</strong> &middot;
											{fmtTime(logMessage.timestamp)}
											{#if messageModel(logMessage)}
												<span class="message-model">{messageModel(logMessage)}</span>
											{/if}
											{#if isDiffMessage(logMessage)}
												<span class="message-diff-stats">{diffStatsLabel(logMessage)}</span>
											{/if}
											<span class="expansion-indicator">
												<span class="chevron">{expandedMessages.has(logMessage.id) ? '▾' : '▸'}</span>
											</span>
										</div>
										<div class="message-summary">{messageSummary(logMessage)}</div>
									</button>
									{#if expandedMessages.has(logMessage.id)}
										{#if isDiffMessage(logMessage)}
											<div class="message-content diff-body">{@html renderDiff(logMessage.content)}</div>
										{:else}
											<div class="message-content markdown-body">{@html renderMarkdown(logMessage.content)}</div>
										{/if}
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</div>
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
	{#if !hasBottomRating}
		<div class="rating-card rating-input">
			<div class="message-header">
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

	.message-model {
		color: #9a9a9a;
	}

	.message-diff-stats {
		color: #5f6b7a;
		font-variant-numeric: tabular-nums;
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

	.diff-body :global(.d2h-wrapper) {
		overflow-x: auto;
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

	.log-group {
		border-color: #e5e5e5;
	}

	.log-group-items {
		margin-top: 0.5rem;
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
