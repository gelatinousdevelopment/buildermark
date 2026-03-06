<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import Conversations from '$lib/components/project/Conversations.svelte';
	import Commits from '$lib/components/project/Commits.svelte';
	import { relationshipCache } from '$lib/stores/relationshipCache.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { projectDateFilterStore } from '$lib/stores/projectDateFilter.svelte';
	import { projectLayoutData } from '$lib/stores/projectLayoutData.svelte';
	import { dateStringToUnixMsRange } from '$lib/utils';
	import type { ProjectDetail, DailyCommitSummary } from '$lib/types';

	const projectId = $derived(page.params.project_id ?? '');
	const order = $derived(page.url.searchParams.get('order') ?? settingsStore.commitSortOrder);
	const selectedDate = $derived(projectDateFilterStore.selectedDate);
	const dateRange = $derived.by(() => {
		if (!selectedDate) return null;
		return dateStringToUnixMsRange(selectedDate);
	});

	let loadedCommitHashes: string[] = $state([]);
	let loadedConversationIds: string[] = $state([]);

	function handleCommitsLoaded(hashes: string[]) {
		loadedCommitHashes = hashes;
	}

	function handleConversationsLoaded(ids: string[]) {
		loadedConversationIds = ids;
	}

	$effect(() => {
		if (loadedCommitHashes.length > 0 && loadedConversationIds.length > 0 && projectId) {
			void relationshipCache.loadRelationships(
				projectId,
				loadedCommitHashes,
				loadedConversationIds
			);
		}
	});

	function handleProjectLoaded(project: ProjectDetail) {
		projectLayoutData.setProject(projectId, project);
	}

	function handleCommitsDataLoaded(data: { dailySummary: DailyCommitSummary[]; branch: string }) {
		projectLayoutData.setCommitsData(projectId, data.dailySummary, data.branch);
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
			pageSize={100}
			limit={100}
			compact={true}
			showAgentColumn={true}
			showRatingsColumn={true}
			enableRelationshipHover={true}
			onConversationsLoaded={handleConversationsLoaded}
			onProjectLoaded={handleProjectLoaded}
			{order}
			start={dateRange?.from}
			end={dateRange?.to}
		/>
		<div class="more">
			<a
				class="bordered small"
				href={resolve('/projects/[project_id]/conversations', { project_id: projectId })}
				>More Conversations...</a
			>
		</div>
	</div>

	<div class="column commits">
		<Commits
			{projectId}
			page={1}
			pageSize={100}
			limit={100}
			compact={true}
			showHeader={true}
			headerLink={resolve('/projects/[project_id]/commits', { project_id: projectId })}
			showBranch={false}
			enableRelationshipHover={true}
			onCommitsLoaded={handleCommitsLoaded}
			onCommitsDataLoaded={handleCommitsDataLoaded}
			defaultToCurrentUser={false}
			{order}
			start={dateRange?.from}
			end={dateRange?.to}
		/>
		<div class="more">
			<a
				class="bordered small"
				href={resolve('/projects/[project_id]/commits', { project_id: projectId })}
				>More Commits...</a
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
		width: fit-content;
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
