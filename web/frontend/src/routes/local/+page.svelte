<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { listProjects, getProject } from '$lib/api';
	import AgentTag from '$lib/components/AgentTag.svelte';
	import { fmtTimeWithSeconds, stars, shortId, singleLineTitle } from '$lib/utils';
	import type { ProjectDetail } from '$lib/types';

	let projects: ProjectDetail[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	function latestRatingTimestamp(project: ProjectDetail): number {
		let latest = 0;
		for (const conv of project.conversations) {
			for (const r of conv.ratings) {
				if (r.createdAt > latest) latest = r.createdAt;
			}
		}
		return latest;
	}

	function sortByRecentRating(list: ProjectDetail[]): ProjectDetail[] {
		return list.slice().sort((a, b) => {
			const ta = latestRatingTimestamp(a);
			const tb = latestRatingTimestamp(b);
			if (ta && !tb) return -1;
			if (!ta && tb) return 1;
			if (ta !== tb) return tb - ta;
			return a.path.localeCompare(b.path);
		});
	}

	async function load() {
		const active = await listProjects(false);
		const activeDetails = await Promise.all(active.map((p) => getProject(p.id, 1, 10)));
		projects = sortByRecentRating(activeDetails);
	}

	onMount(async () => {
		try {
			await load();
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
{:else if projects.length === 0}
	<p>No projects found.</p>
{:else}
	{#each projects as project (project.id)}
		<div class="project-section">
			<div class="project-heading">
				<h2>{project.label || project.path}</h2>
				<a
					class="btn-sm settings-link"
					href={resolve(
						project.defaultBranch
							? `/local/projects/${encodeURIComponent(project.id)}/commits/${encodeURIComponent(project.defaultBranch)}`
							: `/local/projects/${encodeURIComponent(project.id)}/commits`
					)}
				>
					Commits
				</a>
				<a
					class="btn-sm settings-link"
					href={resolve('/local/projects/[project_id]/conversations', { project_id: project.id })}
				>
					Conversations
				</a>
				<details class="project-menu">
					<summary class="btn-sm menu-trigger">...</summary>
					<div class="menu-list">
						<a
							class="menu-item"
							href={resolve('/local/projects/[project_id]/settings', { project_id: project.id })}
						>
							Settings
						</a>
					</div>
				</details>
			</div>
			{#if project.label}
				<p class="project-path">{project.path}</p>
			{/if}
			{#if project.conversations.length === 0}
				<p>No conversations.</p>
			{:else}
				{#if project.conversationPagination.total > project.conversations.length}
					<p class="conversation-count">
						Showing {project.conversations.length} of {project.conversationPagination.total} conversations
					</p>
				{/if}
				<table class="data">
					<thead>
						<tr>
							<th>Conversation</th>
							<th>Agent</th>
							<th>Last Activity</th>
							<th>Ratings</th>
						</tr>
					</thead>
					<tbody>
						{#each project.conversations as conv (conv.id)}
							<tr>
								<td>
									<a
										href={resolve('/local/projects/[project_id]/conversations/[id]', {
											project_id: project.id,
											id: conv.id
										})}
									>
										{(conv.title && singleLineTitle(conv.title)) || shortId(conv.id)}
									</a>
								</td>
								<td><AgentTag agent={conv.agent} /></td>
								<td
									>{conv.lastMessageTimestamp
										? fmtTimeWithSeconds(conv.lastMessageTimestamp)
										: '—'}</td
								>
								<td class="ratings">
									{#if conv.ratings.length > 0}
										{#each conv.ratings as r (r.id)}
											<span title={r.analysis ? `${r.note}\n\nAnalysis: ${r.analysis}` : r.note}
												>{stars(r.rating)}</span
											>
										{/each}
									{:else}
										—
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	{/each}
{/if}

<style>
	.project-section {
		margin-bottom: 2rem;
	}

	.project-heading {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.5rem;
	}

	.project-heading h2 {
		font-size: 1.1rem;
		margin: 0;
		color: #333;
	}

	.project-path {
		font-size: 0.8rem;
		color: #999;
		margin: 0 0 0.5rem 0;
	}

	.conversation-count {
		margin: 0 0 0.5rem 0;
		font-size: 0.8rem;
		color: #888;
	}

	.ratings {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.settings-link {
		text-decoration: none;
	}

	.project-menu {
		position: relative;
	}

	.menu-trigger {
		list-style: none;
		user-select: none;
	}

	.menu-trigger::-webkit-details-marker {
		display: none;
	}

	.menu-list {
		position: absolute;
		top: calc(100% + 0.3rem);
		right: 0;
		min-width: 8rem;
		background: #fff;
		border: 1px solid #ddd;
		border-radius: 4px;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
		padding: 0.3rem;
		z-index: 2;
	}

	.menu-item {
		display: block;
		width: 100%;
		background: transparent;
		border: 0;
		text-align: left;
		padding: 0.35rem 0.45rem;
		font-size: 0.82rem;
		color: #444;
		cursor: pointer;
		text-decoration: none;
	}

	.menu-item:hover {
		background: #f3f3f3;
	}
</style>
