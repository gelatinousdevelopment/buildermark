<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Conversations from '$lib/components/project/Conversations.svelte';
	import Commits from '$lib/components/project/Commits.svelte';
	import { navStore } from '$lib/stores/nav.svelte';
	import { relationshipCache } from '$lib/stores/relationshipCache.svelte';
	import { SvelteMap } from 'svelte/reactivity';

	let { data } = $props();

	let rows = $derived(data.rows);
	let error = $derived(data.error);

	$effect(() => {
		if (data.shouldRedirectToImport) {
			goto(resolve('/projects/import'));
		}
	});

	$effect(() => {
		for (const row of rows) {
			navStore.setCachedLabel(row.project.id, row.project.label || row.project.path);
		}
	});

	// Per-project relationship tracking for hover highlights.
	const projectCommitHashes = new SvelteMap<string, string[]>();
	const projectConversationIds = new SvelteMap<string, string[]>();

	function triggerRelationshipLoad(projectId: string) {
		const commitHashes = projectCommitHashes.get(projectId) ?? [];
		const conversationIds = projectConversationIds.get(projectId) ?? [];
		if (commitHashes.length === 0) return;
		void relationshipCache.loadRelationships(projectId, commitHashes, conversationIds);
	}

	function handleCommitsLoaded(projectId: string, hashes: string[]) {
		projectCommitHashes.set(projectId, hashes);
		triggerRelationshipLoad(projectId);
	}

	function handleConversationsLoaded(projectId: string, ids: string[]) {
		projectConversationIds.set(projectId, ids);
		triggerRelationshipLoad(projectId);
	}

	function projectName(project: { label: string; path: string }): string {
		return project.label || project.path;
	}
</script>

<div class="limited-content-width">
	{#if error}
		<p class="error">{error}</p>
	{:else}
		<div class="projects">
			{#each rows as row, index (row.project.id)}
				<div class="project">
					<div class="meta">
						<div class="label">
							<a href={resolve('/projects/[project_id]', { project_id: row.project.id })}
								>{projectName(row.project)}</a
							>
						</div>
						<div class="right">
							<div class="path">{row.project.path}</div>
						</div>
					</div>
					<div class="content">
						<div class="column conversations">
							<div class="heading">
								<a
									href={resolve('/projects/[project_id]/conversations', {
										project_id: row.project.id
									})}>Agent Conversations</a
								>
							</div>
							<Conversations
								projectId={row.project.id}
								limit={10}
								compact={true}
								autoload={false}
								initialData={row.conversationData}
								initialError={row.conversationError}
								showAgentColumn={true}
								showRatingsColumn={true}
								enableRelationshipHover={true}
								onConversationsLoaded={(ids) => handleConversationsLoaded(row.project.id, ids)}
							/>
						</div>
						<div class="column commits">
							<Commits
								projectId={row.project.id}
								limit={10}
								compact={true}
								showHeader={true}
								headerLink={resolve(`/projects/${encodeURIComponent(row.project.id)}/commits`)}
								showBranch={false}
								useLoadQueue={true}
								loadPriority={index}
								enableRelationshipHover={true}
								onCommitsLoaded={(hashes) => handleCommitsLoaded(row.project.id, hashes)}
							/>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.projects {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 1rem;
	}

	.project .content {
		align-items: stretch;
		background: var(--color-background-content);
		border-radius: 12px;
		border: var(--divider-width) solid var(--color-divider);
		display: flex;
		flex-direction: row;
		justify-content: space-between;
		padding: 0;
	}

	@media (max-width: 1023px) {
		.project .content {
			flex-direction: column;
		}
	}

	.project .meta .label a {
		color: var(--color-text);
		text-decoration: none;
	}

	.project .meta .label a:hover {
		color: var(--accent-color);
		text-decoration: underline;
	}

	.project .meta .label:has(a:hover) {
		opacity: 1;
	}

	.project .meta .right {
		align-items: center;
		display: flex;
		gap: 0.5rem;
	}

	.project:has(.content:hover) {
		.meta .label {
			opacity: 1;
		}

		.meta .label a {
			color: var(--accent-color);
		}

		.content .column .heading {
			opacity: 0.75;
		}

		.commits {
			border-left-color: var(--accent-color-divider);
		}
	}

	.project:has(.content) .meta .label::before {
		background: none;
		border-radius: 4px;
		content: '';
		inset: 0px auto 0px 0px;
		position: absolute;
		transition: width 150ms ease-in-out;
		width: 0%;
		z-index: -1;
	}

	.project:has(.content:hover) .meta .label::before {
		width: 100%;
	}

	.project .content:hover {
		border-color: var(--accent-color-divider);
		box-shadow: 1px 1px 7px rgb(0, 0, 0, 0.1);
	}

	.project .column {
		min-height: 16rem;
		padding: 1rem 0 0.7rem 0;
		flex: 1;
	}

	.project .column .heading {
		font-weight: 600;
		text-transform: uppercase;
		font-size: 0.9rem;
		opacity: 0.5;
		margin-bottom: 0.75rem;
		margin-left: 1rem;
	}

	.project .column .heading a {
		color: inherit;
		text-decoration: none;
	}

	.project .column .heading a:hover {
		text-decoration: underline;
	}

	.meta {
		display: flex;
		gap: 1rem;
		align-items: flex-end;
		justify-content: space-between;
		padding: 0rem 1rem 0.7rem 1rem;
	}

	.meta .label {
		border-radius: 5px;
		border: 1px solid transparent;
		box-sizing: border-box;
		font-size: 1.8rem;
		font-weight: 300;
		letter-spacing: 0.03rem;
		margin: -0.2rem -0.8rem;
		opacity: 0.7;
		padding: 0.2rem 0.8rem;
		position: relative;
	}

	.meta .path {
		font-size: 0.9rem;
		font-weight: 400;
		opacity: 0.5;
	}

	.commits {
		border-left: var(--divider-width) solid var(--color-divider);
	}

	@media (max-width: 1023px) {
		.commits {
			border-left: 0;
			border-top: var(--divider-width) solid var(--color-divider);
		}
	}
</style>
