<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getProjectCommitDetail, setCommitOverrideLinePercent } from '$lib/api';
	import { fmtTime, singleLineTitle } from '$lib/utils';
	import type { ProjectCommitDetailResponse, AgentCoverageSegment } from '$lib/types';
	import DiffMessageCard from '$lib/components/DiffMessageCard.svelte';
	import DiffCount from '$lib/components/DiffCount.svelte';
	import AgentPercentageBar from '$lib/components/AgentPercentageBar.svelte';

	function toBarSegments(segs?: AgentCoverageSegment[]): { name: string; percent: number }[] {
		if (!segs || segs.length === 0) return [];
		return segs.map((s) => ({ name: s.agent, percent: s.linePercent }));
	}

	let detail: ProjectCommitDetailResponse | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let expandedMessageIds: string[] = $state([]);
	let totalAdded = $derived.by(() => detail?.files.reduce((sum, f) => sum + f.added, 0) ?? 0);
	let totalRemoved = $derived.by(() => detail?.files.reduce((sum, f) => sum + f.removed, 0) ?? 0);
	type DiffSection = {
		path: string;
		diffText: string;
	};
	let diffSections = $derived.by(() => parseDiffSections(detail?.diff ?? ''));
	let diffSectionByPath = $derived.by(
		() => new Map(diffSections.map((section) => [section.path, section.diffText]))
	);
	let visibleFiles = $derived.by(
		() => detail?.files.filter((file) => file.added > 0 || file.removed > 0) ?? []
	);
	let renderableDiffFiles = $derived.by(() =>
		visibleFiles.filter((file) => !file.ignored && diffSectionByPath.has(file.path))
	);
	let agentLinesTotal = $derived.by(() => detail?.commit.linesTotal ?? 0);
	let agentLinesFromAgent = $derived.by(() => detail?.commit.linesFromAgent ?? 0);
	let collapsedDiffPaths: string[] = $state([]);

	let editingOverride = $state(false);
	let overrideInput = $state('');
	let savingOverride = $state(false);

	let hasOverride = $derived.by(() => detail?.commit?.overrideLinePercent != null);
	let isWorkingCopyUnknown = $derived.by(
		() =>
			!!detail?.commit.workingCopy &&
			!hasOverride &&
			(detail?.messages.length ?? 0) === 0 &&
			agentLinesFromAgent === 0
	);
	let effectivePercent = $derived.by(() => {
		if (detail?.commit.overrideLinePercent != null) {
			return detail.commit.overrideLinePercent;
		}
		return percent(agentLinesFromAgent, agentLinesTotal);
	});

	async function saveOverride() {
		if (!detail || savingOverride) return;
		const val = parseFloat(overrideInput);
		if (isNaN(val) || val < 0 || val > 100) return;
		savingOverride = true;
		try {
			await setCommitOverrideLinePercent(detail.commit.projectId, detail.commit.commitHash, val);
			detail.commit.overrideLinePercent = val;
			editingOverride = false;
		} finally {
			savingOverride = false;
		}
	}

	async function clearOverride() {
		if (!detail || savingOverride) return;
		savingOverride = true;
		try {
			await setCommitOverrideLinePercent(detail.commit.projectId, detail.commit.commitHash, null);
			detail.commit.overrideLinePercent = undefined;
			editingOverride = false;
		} finally {
			savingOverride = false;
		}
	}

	function startEditOverride() {
		overrideInput = hasOverride
			? String(detail!.commit.overrideLinePercent)
			: String(Math.round(percent(agentLinesFromAgent, agentLinesTotal)));
		editingOverride = true;
	}

	let allMessagesExpanded = $derived.by(() => {
		if (!detail) return false;
		return (
			detail.messages.length > 0 && detail.messages.every((m) => expandedMessageIds.includes(m.id))
		);
	});

	function toggleExpandAllMessages() {
		if (!detail) return;
		if (allMessagesExpanded) {
			expandedMessageIds = [];
		} else {
			expandedMessageIds = detail.messages.map((m) => m.id);
		}
	}

	function isExpanded(id: string): boolean {
		return expandedMessageIds.includes(id);
	}

	function toggleExpanded(id: string) {
		if (expandedMessageIds.includes(id)) {
			expandedMessageIds = expandedMessageIds.filter((v) => v !== id);
			return;
		}
		expandedMessageIds = [...expandedMessageIds, id];
	}

	function isDiffExpanded(path: string): boolean {
		return !collapsedDiffPaths.includes(path);
	}

	function toggleDiffPath(path: string) {
		if (collapsedDiffPaths.includes(path)) {
			collapsedDiffPaths = collapsedDiffPaths.filter((p) => p !== path);
		} else {
			collapsedDiffPaths = [...collapsedDiffPaths, path];
		}
	}

	function normalizeGitPath(pathText: string): string {
		let path = pathText.trim();
		if (path.startsWith('"') && path.endsWith('"')) {
			path = path.slice(1, -1).replaceAll('\\"', '"').replaceAll('\\\\', '\\');
		}
		if (path.startsWith('a/') || path.startsWith('b/')) {
			return path.slice(2);
		}
		return path;
	}

	function parseDiffHeader(line: string): { oldPath: string; newPath: string } | null {
		const match = line.match(/^diff --git (.+) (.+)$/);
		if (!match) return null;
		return {
			oldPath: normalizeGitPath(match[1]),
			newPath: normalizeGitPath(match[2])
		};
	}

	function parseDiffSections(diffText: string): DiffSection[] {
		if (!diffText.trim()) return [];
		const lines = diffText.split('\n');
		const sections: DiffSection[] = [];
		let currentPath = '';
		let currentLines: string[] = [];

		const pushCurrent = () => {
			if (!currentPath || currentLines.length === 0) return;
			sections.push({
				path: currentPath,
				diffText: currentLines.join('\n')
			});
		};

		for (const line of lines) {
			if (line.startsWith('diff --git ')) {
				pushCurrent();
				currentLines = [line];
				const parsed = parseDiffHeader(line);
				if (!parsed) {
					currentPath = '';
					continue;
				}
				currentPath = parsed.newPath === '/dev/null' ? parsed.oldPath : parsed.newPath;
				continue;
			}
			if (currentLines.length > 0) currentLines.push(line);
		}
		pushCurrent();
		return sections;
	}

	function diffAnchor(path: string): string {
		return `diff-${encodeURIComponent(path)}`;
	}

	function percent(part: number, total: number): number {
		if (total <= 0) return 0;
		return (part * 100) / total;
	}

	onMount(async () => {
		try {
			const projectId = page.params.project_id;
			const branch = page.params.branch;
			const commitHash = page.params.commit_hash;
			if (!projectId || !branch || !commitHash) {
				throw new Error('Missing project, branch, or commit ID');
			}
			detail = await getProjectCommitDetail(projectId, commitHash, branch);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load commit detail';
		} finally {
			loading = false;
		}
	});
</script>

<div class="content">
	{#if loading}
		<p class="loading">Loading commit...</p>
	{:else if error}
		<p class="error">{error}</p>
	{:else if detail}
		<div class="right">
			{#if !detail?.commit.workingCopy}
				<div class="override-controls">
					{#if editingOverride}
						<input
							type="number"
							min="0"
							max="100"
							step="any"
							bind:value={overrideInput}
							class="override-input"
							disabled={savingOverride}
							onkeydown={(e) => {
								if (e.key === 'Enter') saveOverride();
								if (e.key === 'Escape') editingOverride = false;
							}}
						/>
						<button class="btn-override-action" onclick={saveOverride} disabled={savingOverride}
							>Save</button
						>
						<button
							class="btn-override-action"
							onclick={() => (editingOverride = false)}
							disabled={savingOverride}>Cancel</button
						>
					{:else}
						<button class="btn-override-action" onclick={startEditOverride}>
							{hasOverride ? 'Edit Override' : 'Override Agent Attribution'}
						</button>
					{/if}
				</div>
			{/if}
		</div>
		<h2>{detail.commit.subject || detail.commit.commitHash.slice(0, 8)}</h2>
		<p>
			{fmtTime(detail.commit.authoredAtUnixMs)} | {detail.commit.commitHash.slice(0, 12)}
			{#if detail.commitUrl}
				|
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL -->
				<a href={detail.commitUrl} target="_blank" rel="noopener noreferrer"
					>{new URL(detail.commitUrl).hostname}</a
				>
			{/if}
		</p>
		{#if hasOverride}
			<p class="override-display">
				Agent Attribution Override: {detail.commit.overrideLinePercent}%
				<button class="btn-override-action" onclick={clearOverride} disabled={savingOverride}
					>Remove</button
				>
			</p>
		{/if}
		{#if isWorkingCopyUnknown}
			<p class="unknown-attribution">Agent attribution: Unknown</p>
		{:else}
			<div class="detail-bar">
				<AgentPercentageBar
					agentPercent={effectivePercent}
					segments={hasOverride ? [] : toBarSegments(detail.commit.agentSegments)}
					totalLines={agentLinesTotal}
					showManual={true}
					height="18px"
				/>
			</div>
		{/if}
		<p>Changes: <DiffCount added={totalAdded} removed={totalRemoved} /></p>

		<h3>{detail.commit.workingCopy ? 'Working Copy Diff' : 'Commit Diff'}</h3>
		{#if visibleFiles.length === 0}
			<p>No changed files in this diff.</p>
		{:else}
			<div class="file-table-wrap">
				<table class="file-table">
					<!-- <thead>
						<tr>
							<th>File</th>
							<th>Changes</th>
							<th class="pct-col">Agents</th>
						</tr>
					</thead> -->
					<tbody>
						{#each visibleFiles as file (file.path)}
							<tr class:ignored-row={file.ignored || file.moved}>
								<td>
									{#if !file.ignored && diffSectionByPath.has(file.path)}
										<a href={`#${diffAnchor(file.path)}`}>{file.path}</a>
									{:else}
										{file.path}
									{/if}
									{#if file.moved}
										<span class="file-tag">[moved]</span>
									{:else if file.copiedFromAgent}
										<span class="file-tag">[copied-from-agent]</span>
									{/if}
								</td>
								<td class="changes-col">
									{#if !file.ignored}
										<DiffCount added={file.added} removed={file.removed} />
									{/if}
								</td>
								<td class="pct-col">
									{#if !file.ignored && !file.moved}
										<div class="file-bar-wrap">
											<AgentPercentageBar
												agentPercent={file.linePercent}
												segments={toBarSegments(file.agentSegments)}
												totalLines={file.added + file.removed}
												showKey={true}
											/>
										</div>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
		{#if renderableDiffFiles.length === 0}
			<p>No non-ignored file diffs to display.</p>
		{:else}
			{#each renderableDiffFiles as file (file.path)}
				{@const fileExpanded = isDiffExpanded(file.path)}
				<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
				<div
					class="commit-diff-section diff-card"
					class:diff-card-collapsed={!fileExpanded}
					id={diffAnchor(file.path)}
					role={!fileExpanded ? 'button' : undefined}
					tabindex={!fileExpanded ? 0 : undefined}
					onclick={!fileExpanded ? () => toggleDiffPath(file.path) : undefined}
					onkeydown={!fileExpanded
						? (e: KeyboardEvent) => {
								if (e.key === 'Enter' || e.key === ' ') {
									e.preventDefault();
									toggleDiffPath(file.path);
								}
							}
						: undefined}
				>
					<DiffMessageCard
						label={file.path}
						content={diffSectionByPath.get(file.path) ?? ''}
						expanded={fileExpanded}
						agentPercent={file.linePercent}
						onToggle={fileExpanded ? () => toggleDiffPath(file.path) : undefined}
						contentOnly={true}
						toggleWithHeaderClick={true}
					/>
				</div>
			{/each}
		{/if}

		<div class="section-header">
			<h3>Matched Messages</h3>
			<button class="btn-expand-all" onclick={toggleExpandAllMessages}>
				{allMessagesExpanded ? 'Collapse All' : 'Expand All'}
			</button>
		</div>
		{#if detail.attribution?.hasFallbackAttribution}
			<p class="fallback-note">
				Attribution includes fallback copied-line matching ({detail.attribution
					.fallbackMatchedLines}
				lines). Exact matched-message lines: {detail.attribution.exactMatchedLines}.
			</p>
		{/if}
		{#if detail.messages.length === 0}
			<p>No tracked diff messages matched this commit.</p>
		{:else}
			{#each detail.messages as message (message.id)}
				{@const msgExpanded = isExpanded(message.id)}
				<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
				<div
					class="matched-message-diff-card"
					class:diff-card-collapsed={!msgExpanded}
					role={!msgExpanded ? 'button' : undefined}
					tabindex={!msgExpanded ? 0 : undefined}
					onclick={!msgExpanded ? () => toggleExpanded(message.id) : undefined}
					onkeydown={!msgExpanded
						? (e: KeyboardEvent) => {
								if (e.key === 'Enter' || e.key === ' ') {
									e.preventDefault();
									toggleExpanded(message.id);
								}
							}
						: undefined}
				>
					<DiffMessageCard
						timestamp={message.timestamp}
						role={message.agent || 'agent'}
						model={message.model ?? ''}
						content={message.content}
						expanded={msgExpanded}
						statsLabel={`matched ${message.linesMatched} lines, ${message.charsMatched} chars`}
						linkHref={resolve('/projects/[project_id]/conversations/[id]', {
							project_id: detail.commit.projectId,
							id: message.conversationId
						})}
						linkLabel={`Conversation: ${(message.conversationTitle && singleLineTitle(message.conversationTitle)) || message.conversationId}`}
						onToggle={msgExpanded ? () => toggleExpanded(message.id) : undefined}
						toggleWithHeaderClick={true}
					/>
				</div>
			{/each}
		{/if}
	{/if}
</div>

<style>
	.content {
		padding: 0 1rem;
	}

	h2 {
		font-size: 1.3rem;
		margin: 1rem 0;
	}

	.right {
		float: right;
	}

	.fallback-note {
		color: var(--color-fallback-note);
		font-size: 0.9rem;
		margin: 0.35rem 0 0.6rem 0;
	}

	h3 {
		line-height: 1;
		margin: 1.5rem 0 0.5rem 0;
		padding: 0;
	}

	.file-table-wrap {
		overflow-x: auto;
		margin-bottom: 0.75rem;
	}

	.file-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.9rem;
	}

	/*.file-table th,*/
	.file-table td {
		padding: 0.4rem 0.5rem;
		border-bottom: 1px solid var(--color-border-light);
		text-align: left;
	}

	/*.file-table th {
		color: #666;
		font-size: 0.82rem;
		font-weight: 600;
	}*/

	.file-table .pct-col {
		text-align: right;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.file-table .changes-col {
		font-variant-numeric: tabular-nums;
		padding-right: 1.5rem;
		text-align: right;
		white-space: nowrap;
	}

	.ignored-row {
		color: var(--color-text-faded);
		background: var(--color-background-subtle);
	}

	.file-tag {
		margin-left: 0.4rem;
		font-size: 0.8rem;
		color: var(--color-text-tertiary);
	}

	.section-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.section-header h3 {
		margin: 0;
	}

	.btn-expand-all {
		padding: 0.15rem 0.5rem;
		font-size: 0.8rem;
		border: 1px solid var(--color-border-input);
		border-radius: 4px;
		background: var(--color-button-bg);
		cursor: pointer;
		color: var(--color-text-secondary);
	}

	.btn-expand-all:hover {
		border-color: var(--accent-color);
		color: var(--accent-color);
	}

	.commit-diff-section {
		margin-top: 0.5rem;
	}

	.matched-message-diff-card {
		margin-bottom: 1rem;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--color-border-light);
		border-radius: 4px;
		background: var(--color-background-subtle);
	}

	.diff-card-collapsed {
		cursor: pointer;
	}

	.diff-card-collapsed:hover {
		border-color: var(--accent-color);
		background: var(--accent-color-ultralight);
	}

	.override-display {
		color: var(--color-status-red);
		font-size: 0.9rem;
		margin-bottom: 0.75rem;
	}

	.override-controls {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.5rem;
	}

	.override-input {
		width: 3rem;
		padding: 0.2rem 0.4rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		font-size: 0.85rem;
	}

	.btn-override-action {
		padding: 0.15rem 0.5rem;
		font-size: 0.8rem;
		border: 1px solid var(--color-border-input);
		border-radius: 4px;
		background: var(--color-button-bg);
		cursor: pointer;
		color: var(--color-text-secondary);
	}

	.btn-override-action:hover {
		border-color: var(--accent-color);
		color: var(--accent-color);
	}

	.btn-override-action:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.detail-bar {
		max-width: 500px;
		margin-bottom: 0.75rem;
	}

	.unknown-attribution {
		color: var(--color-text-faded);
		font-size: 0.9rem;
		margin-bottom: 0.75rem;
	}

	.file-bar-wrap {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 100px;
	}
</style>
