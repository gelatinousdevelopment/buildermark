<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { listProjects, getProject, setProjectIgnored } from '$lib/api';
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
	let detectedProjects: Project[] = $state([]);
	let detectedLoading = $state(false);
	let detectedError: string | null = $state(null);
	let selectedProjectIds: string[] = $state([]);
	let savingSelection = $state(false);
	let saveSelectionError: string | null = $state(null);

	const selectedCount = $derived(selectedProjectIds.length);

	function projectName(project: Project): string {
		return project.label || project.path;
	}

	function sortRows(a: ProjectRow, b: ProjectRow): number {
		if (a.lastMessageTimestamp !== b.lastMessageTimestamp) {
			return b.lastMessageTimestamp - a.lastMessageTimestamp;
		}
		return projectName(a.project).localeCompare(projectName(b.project));
	}

	function pathTail(path: string): string {
		const normalized = path.replace(/[\\/]+$/, '');
		const parts = normalized.split(/[\\/]/);
		return parts[parts.length - 1] || path;
	}

	function previousLocationSuggestions(currentProject: Project): Project[] {
		const currentTail = pathTail(currentProject.path).toLowerCase();
		return detectedProjects.filter(
			(project) =>
				project.id !== currentProject.id && pathTail(project.path).toLowerCase() === currentTail
		);
	}

	function toggleSelection(projectId: string, checked: boolean) {
		if (checked) {
			selectedProjectIds = selectedProjectIds.includes(projectId)
				? selectedProjectIds
				: [...selectedProjectIds, projectId];
		} else {
			selectedProjectIds = selectedProjectIds.filter((id) => id !== projectId);
		}
	}

	async function loadDetectedProjects() {
		detectedLoading = true;
		detectedError = null;
		try {
			detectedProjects = (await listProjects(true)).sort((a, b) =>
				projectName(a).localeCompare(projectName(b))
			);
		} catch (e) {
			detectedError = e instanceof Error ? e.message : 'Failed to load detected projects';
		} finally {
			detectedLoading = false;
		}
	}

	async function startTrackingSelected() {
		if (selectedProjectIds.length === 0) return;
		savingSelection = true;
		saveSelectionError = null;
		try {
			await Promise.all(selectedProjectIds.map((projectId) => setProjectIgnored(projectId, false)));
			selectedProjectIds = [];
			await Promise.all([loadTrackedRows(), loadDetectedProjects()]);
		} catch (e) {
			saveSelectionError = e instanceof Error ? e.message : 'Failed to update project tracking';
		} finally {
			savingSelection = false;
		}
	}

	async function loadTrackedRows() {
		loading = true;
		error = null;
		try {
			const projects = (await listProjects(false)).filter((project) => project.gitId);
			const loadedRows = await Promise.all(
				projects.map(async (project): Promise<ProjectRow> => {
					try {
						const conversationData = await getProject(project.id, 1, 10);
						const latestConversationTs =
							conversationData.conversations[0]?.lastMessageTimestamp ?? 0;
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
	}

	onMount(async () => {
		await Promise.all([loadTrackedRows(), loadDetectedProjects()]);
	});
</script>

<div class="limited-content-width">
	{#if loading}
		<p class="loading">Loading projects...</p>
	{:else if error}
		<p class="error">{error}</p>
	{:else if rows.length === 0}
		<div class="onboarding inset-when-limited-content-width">
			<div class="column left">
				<h2>Welcome to BuilderBit Local</h2>
				<p>
					Track projects to see agent conversations and commit attribution side-by-side in one
					dashboard.
				</p>
				<p class="muted">
					We found projects from your agent conversation folders. Choose what to track now—you can
					always change this later in settings.
				</p>
			</div>
			<div class="column right">
				<h3>Select projects to track</h3>
				{#if detectedLoading}
					<p class="loading">Finding projects from agent conversations…</p>
				{:else if detectedError}
					<p class="error">{detectedError}</p>
				{:else if detectedProjects.length === 0}
					<p class="muted">No detected projects found yet.</p>
				{:else}
					<ul class="project-options">
						{#each detectedProjects as project (project.id)}
							<li>
								<label>
									<input
										type="checkbox"
										checked={selectedProjectIds.includes(project.id)}
										onchange={(event) =>
											toggleSelection(
												project.id,
												(event.currentTarget as HTMLInputElement).checked
											)}
									/>
									<span class="text">
										<span class="title">{projectName(project)}</span>
										<span class="subtitle">{project.path}</span>
									</span>
								</label>
								{#if previousLocationSuggestions(project).length > 0}
									<ul class="suggestions">
										{#each previousLocationSuggestions(project) as suggestion (suggestion.id)}
											<li>
												<label>
													<input
														type="checkbox"
														checked={selectedProjectIds.includes(suggestion.id)}
														onchange={(event) =>
															toggleSelection(
																suggestion.id,
																(event.currentTarget as HTMLInputElement).checked
															)}
													/>
													<span>{suggestion.path}</span>
												</label>
											</li>
										{/each}
									</ul>
								{/if}
							</li>
						{/each}
					</ul>
					{#if saveSelectionError}
						<p class="error">{saveSelectionError}</p>
					{/if}
					<button
						class="btn-sm"
						disabled={selectedCount === 0 || savingSelection}
						onclick={startTrackingSelected}
					>
						{savingSelection
							? 'Saving…'
							: `Track ${selectedCount || ''} selected project${selectedCount === 1 ? '' : 's'}`}
					</button>
				{/if}
			</div>
		</div>
	{:else}
		<div class="projects">
			{#each rows as row, index (row.project.id)}
				<div class="project">
					<div class="meta">
						<div class="label">
							<a href={resolve('/local/projects/[project_id]', { project_id: row.project.id })}
								>{projectName(row.project)}</a
							>
						</div>
						<div class="right">
							<div class="path">{row.project.path}</div>
							<!-- <a
								href={resolve('/local/projects/[project_id]/settings', {
									project_id: row.project.id
								})}><Icon name="gear" width="14px" /></a
							> -->
						</div>
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
								compact={true}
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
									href={resolve(
										row.project.defaultBranch
											? `/local/projects/${encodeURIComponent(row.project.id)}/commits/${encodeURIComponent(row.project.defaultBranch)}`
											: `/local/projects/${encodeURIComponent(row.project.id)}/commits`
									)}>Git Commits</a
								>
							</div>
							<Commits
								projectId={row.project.id}
								branch={row.project.defaultBranch}
								limit={10}
								compact={true}
								showBranch={false}
								useLoadQueue={true}
								loadPriority={index}
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

	.onboarding {
		display: grid;
		grid-template-columns: 40% 60%;
		padding: 1.2rem;
		gap: 1.2rem;
	}

	.onboarding h2,
	.onboarding h3 {
		margin: 0;
	}

	.onboarding .left {
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}

	.onboarding .left p {
		margin: 0;
		font-size: 1rem;
		line-height: 1.45;
	}

	.onboarding .muted {
		opacity: 0.75;
	}

	.onboarding .right {
		display: flex;
		flex-direction: column;
		gap: 0.8rem;
	}

	.project-options {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		max-height: 30rem;
		overflow: auto;
	}

	.project-options > li {
		border: 0.5px solid var(--color-divider);
		border-radius: 8px;
		padding: 0.45rem 0.55rem;
	}

	.project-options label {
		display: flex;
		align-items: flex-start;
		gap: 0.45rem;
	}

	.project-options .text {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}

	.project-options .title {
		font-weight: 600;
	}

	.project-options .subtitle {
		opacity: 0.7;
		font-size: 0.85rem;
		font-family: var(--font-family-monospace);
	}

	.suggestions {
		list-style: none;
		margin: 0.45rem 0 0 1.6rem;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.suggestions span {
		font-size: 0.8rem;
		opacity: 0.75;
		font-family: var(--font-family-monospace);
	}

	@media (max-width: 900px) {
		.onboarding {
			grid-template-columns: 1fr;
		}
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

		background: #fff;
		border-color: var(--color-divider);
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

	/*.project .meta .right a:not(:hover) {
		color: var(--color-text);
		text-decoration: none;
	}*/

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
		/*border: 0.5px solid var(--accent-color);*/
		width: 100%;
	}

	.project .content:hover {
		/*background: #f8f8f8;*/
		border-color: var(--accent-color-divider);
		/*box-shadow: 1px 2px 0px var(--accent-color-divider);*/
		box-shadow: 1px 1px 7px rgb(0, 0, 0, 0.1);
	}

	.project .column {
		min-height: 16rem;
		padding: 1rem 0 0.7rem 0;
		flex: 1;
		max-width: 50%;
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
		border-left: 0.5px solid var(--color-divider);
	}

	@media (max-width: 1023px) {
		.commits {
			border-left: 0;
			border-top: 0.5px solid var(--color-divider);
		}
	}
</style>
