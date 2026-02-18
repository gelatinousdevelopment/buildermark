<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import Conversations from '$lib/components/project/Conversations.svelte';
	import Commits from '$lib/components/project/Commits.svelte';

	const projectId = $derived(page.params.project_id ?? '');
</script>

<div class="project-content">
	<div class="column conversations">
		<div class="heading">
			<a href={resolve('/local/projects/[project_id]/conversations', { project_id: projectId })}
				>Agent Conversations</a
			>
		</div>
		<Conversations
			{projectId}
			page={1}
			pageSize={20}
			limit={20}
			showAgentColumn={true}
			showRatingsColumn={true}
		/>
		<div class="more">
			<a
				class="btn-sm"
				href={resolve('/local/projects/[project_id]/conversations', { project_id: projectId })}
				>More...</a
			>
		</div>
	</div>

	<div class="column commits">
		<div class="heading">
			<a href={resolve('/local/projects/[project_id]/commits', { project_id: projectId })}
				>Git Commits</a
			>
		</div>
		<Commits {projectId} page={1} limit={20} compact={true} />
		<div class="more">
			<a
				class="btn-sm"
				href={resolve('/local/projects/[project_id]/commits', { project_id: projectId })}>More...</a
			>
		</div>
	</div>
</div>

<style>
	.project-content {
		display: flex;
		flex-direction: row;
		justify-content: space-between;
		align-items: stretch;
		margin: -1rem 0;
	}

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
	}

	.heading a {
		color: inherit;
		text-decoration: none;
	}

	.heading a:hover {
		text-decoration: underline;
	}

	.conversations {
		padding-right: 1rem;
	}

	.commits {
		border-left: 0.5px solid var(--color-divider);
		padding-left: 1rem;
	}

	.more {
		margin-top: 0.75rem;
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
