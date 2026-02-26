<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import Conversations from '$lib/components/project/Conversations.svelte';
	import Commits from '$lib/components/project/Commits.svelte';
	import { relationshipCache } from '$lib/stores/relationshipCache.svelte';
	import { projectDateFilterStore } from '$lib/stores/projectDateFilter.svelte';
	import { dateStringToUnixMsRange } from '$lib/utils';

	const projectId = $derived(page.params.project_id ?? '');
	const selectedDate = $derived(projectDateFilterStore.selectedDate);
	const dateRange = $derived.by(() => {
		if (!selectedDate) return null;
		return dateStringToUnixMsRange(selectedDate);
	});

	let loadedCommitHashes: string[] = $state([]);
	let loadedConversationIds: string[] = $state([]);

	function handleCommitsLoaded(hashes: string[]) {
		loadedCommitHashes = hashes;
		triggerRelationshipLoad(hashes, loadedConversationIds);
	}

	function handleConversationsLoaded(ids: string[]) {
		loadedConversationIds = ids;
		triggerRelationshipLoad(loadedCommitHashes, ids);
	}

	function triggerRelationshipLoad(commitHashes: string[], conversationIds: string[]) {
		if (commitHashes.length === 0 || !projectId) return;
		void relationshipCache.loadRelationships(projectId, commitHashes, conversationIds);
	}
</script>

<div class="project-content">
	<div class="column conversations">
		<div class="heading">
			<a href={resolve('/projects/[project_id]/conversations', { project_id: projectId })}
				>Agent Conversations</a
			>
		</div>
		<Conversations
			{projectId}
			page={1}
			pageSize={30}
			limit={30}
			compact={true}
			showAgentColumn={true}
			showRatingsColumn={true}
			enableRelationshipHover={true}
			onConversationsLoaded={handleConversationsLoaded}
			start={dateRange?.from}
			end={dateRange?.to}
		/>
		<div class="more">
			<a
				class="bordered small"
				href={resolve('/projects/[project_id]/conversations', { project_id: projectId })}>More...</a
			>
		</div>
	</div>

	<div class="column commits">
		<Commits
			{projectId}
			page={1}
			pageSize={30}
			limit={30}
			compact={true}
			showHeader={true}
			headerLink={resolve('/projects/[project_id]/commits', { project_id: projectId })}
			showBranch={false}
			showDate={true}
			enableRelationshipHover={true}
			onCommitsLoaded={handleCommitsLoaded}
			start={dateRange?.from}
			end={dateRange?.to}
		/>
		<div class="more">
			<a
				class="bordered small"
				href={resolve('/projects/[project_id]/commits', { project_id: projectId })}>More...</a
			>
		</div>
	</div>
</div>

<style>
	.project-content {
		align-items: stretch;
		/*background: var(--color-background-content);*/
		/*border-radius: 10px;*/
		/*border: 0.5px solid var(--color-divider);*/
		/*box-sizing: border-box;*/
		display: flex;
		flex-direction: row;
		justify-content: space-between;
		/*margin: 2rem auto;*/
		/*transition: all 200ms;*/
	}

	/*@media (max-width: 1600px) {
		.project-content {
			border-width: 0 0 0.5px 0;
			margin: 0 auto;
			border-radius: 0;
		}
	}*/

	.column {
		flex: 1;
		min-height: 18rem;
		padding: 1rem 0 1rem 0;
	}

	.heading {
		font-weight: 600;
		text-transform: uppercase;
		font-size: 0.9rem;
		opacity: 0.5;
		margin-bottom: 0.75rem;
		padding: 0 1rem;
	}

	.heading a {
		color: inherit;
		text-decoration: none;
	}

	.heading a:hover {
		text-decoration: underline;
	}

	.commits {
		border-left: 0.5px solid var(--color-divider);
	}

	.more {
		margin-top: 0.75rem;
		margin-left: 1rem;
	}

	@media (max-width: 1023px) {
		.project-content {
			flex-direction: column;
		}

		.commits {
			border-left: 0;
			border-top: 0.5px solid var(--color-divider);
		}
	}
</style>
