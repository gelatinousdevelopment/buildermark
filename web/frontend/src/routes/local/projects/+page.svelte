<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { listProjects, getProject } from '$lib/api';
	import Conversations from '$lib/components/project/Conversations.svelte';
	import Commits from '$lib/components/project/Commits.svelte';
	import type { Project, ProjectDetail } from '$lib/types';

	type ProjectRow = {
		project: Project;
		conversationData: ProjectDetail | null;
		conversationError: string | null;
		lastMessageTimestamp: number;
	};

	let rows: ProjectRow[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	function projectName(project: Project): string {
		return project.label || project.path;
	}

	function sortRows(a: ProjectRow, b: ProjectRow): number {
		if (a.lastMessageTimestamp !== b.lastMessageTimestamp) {
			return b.lastMessageTimestamp - a.lastMessageTimestamp;
		}
		return projectName(a.project).localeCompare(projectName(b.project));
	}

	onMount(async () => {
		try {
			const projects = (await listProjects(false)).filter((project) => project.gitId);
			const loadedRows = await Promise.all(
				projects.map(async (project): Promise<ProjectRow> => {
					try {
						const conversationData = await getProject(project.id, 1, 10);
						const latestConversationTs = conversationData.conversations[0]?.lastMessageTimestamp ?? 0;
						return {
							project,
							conversationData,
							conversationError: null,
							lastMessageTimestamp: latestConversationTs
						};
					} catch (e) {
						return {
							project,
							conversationData: null,
							conversationError:
								e instanceof Error ? e.message : 'Failed to load project conversations',
							lastMessageTimestamp: 0
						};
					}
				})
			);
			rows = loadedRows.sort(sortRows);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load projects';
		} finally {
			loading = false;
		}
	});
</script>

{#if loading}
	<p class="loading">Loading projects...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if rows.length === 0}
	<p>No tracked projects with git IDs found.</p>
{:else}
	<div class="projects">
		{#each rows as row, index (row.project.id)}
			<div class="project">
				<div class="meta">
					<div class="label">{projectName(row.project)}</div>
					<div class="path">{row.project.path}</div>
				</div>
				<div class="content">
					<div class="column conversations">
						<div class="heading">
							<a
								href={resolve('/local/projects/[project_id]/conversations', {
									project_id: row.project.id
								})}>Agent Conversations</a
							>
						</div>
						<Conversations
							projectId={row.project.id}
							limit={10}
							autoload={false}
							initialData={row.conversationData}
							initialError={row.conversationError}
							showAgentColumn={true}
							showRatingsColumn={true}
						/>
					</div>
					<div class="column commits">
						<div class="heading">
							<a
								href={resolve('/local/projects/[project_id]/commits', {
									project_id: row.project.id
								})}>Git Commits</a
							>
						</div>
						<Commits
							projectId={row.project.id}
							limit={10}
							compact={true}
							useLoadQueue={true}
							loadPriority={index}
						/>
					</div>
				</div>
			</div>
		{/each}
	</div>
{/if}

<style>
	.projects {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.project .content {
		align-items: stretch;
		background: #fbfbfb;
		border-radius: 12px;
		border: 0.5px solid #ccc;
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

	.project:has(.content:hover) {
		.meta .label {
			opacity: 1;
			color: var(--accent-color);
		}

		.content .column .heading {
			opacity: 0.75;
		}
	}

	.project:has(.content) .meta .label::before {
		background: var(--accent-color-ultralight);
		border-radius: 5px;
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
		background: #f8f8f8;
		border-color: #bbb;
	}

	.project .column {
		min-height: 18rem;
		padding: 1rem;
	}

	.project .column .heading {
		font-weight: 600;
		text-transform: uppercase;
		font-size: 0.9rem;
		opacity: 0.5;
		margin-bottom: 0.75rem;
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
		padding: 0.5rem 1rem 1rem 1rem;
	}

	.meta .label {
		box-sizing: border-box;
		font-size: 1.8rem;
		font-weight: 300;
		letter-spacing: 0.03rem;
		opacity: 0.7;
		padding: 0.2rem 0.8rem;
		margin: -0.2rem -0.8rem;
		border: 1px solid transparent;
		border-radius: 5px;
		position: relative;
	}

	.meta .path {
		font-size: 0.9rem;
		font-weight: 400;
		opacity: 0.5;
	}

	.conversations {
		flex: 2;
	}

	.commits {
		border-left: 0.5px solid var(--color-divider);
		flex: 2;
	}

	@media (max-width: 1023px) {
		.commits {
			border-left: 0;
			border-top: 0.5px solid var(--color-divider);
		}
	}
</style>
