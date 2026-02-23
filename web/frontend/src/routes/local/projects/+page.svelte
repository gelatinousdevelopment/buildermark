<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { discoverImportableProjects, getProject, importProjects, listProjects } from '$lib/api';
	import Conversations from '$lib/components/project/Conversations.svelte';
	import Commits from '$lib/components/project/Commits.svelte';
	import ProjectOnboarding from '$lib/components/project/ProjectOnboarding.svelte';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import { relationshipCache } from '$lib/stores/relationshipCache.svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import type { ImportableProject, Project, ProjectDetail } from '$lib/types';

	type ProjectRow = {
		project: Project;
		conversationData: ProjectDetail | null;
		conversationError: string | null;
		lastMessageTimestamp: number;
	};

	let rows: ProjectRow[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let detectedProjects: ImportableProject[] = $state([]);
	let detectedLoading = $state(false);
	let detectedError: string | null = $state(null);
	let selectedProjectPaths: string[] = $state([]);
	let historyImportDays = $state('90');
	let savingSelection = $state(false);
	let saveSelectionError: string | null = $state(null);
	const historyDayOptions = ['7', '14', '30', '60', '90', '180', '365', 'all'];

	let importStatusMessage = $derived(
		websocketStore.importStatus?.state === 'running'
			? websocketStore.importStatus.message
			: null
	);
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

	function sortRows(a: ProjectRow, b: ProjectRow): number {
		if (a.lastMessageTimestamp !== b.lastMessageTimestamp) {
			return b.lastMessageTimestamp - a.lastMessageTimestamp;
		}
		return projectName(a.project).localeCompare(projectName(b.project));
	}

	function toggleSelection(projectPath: string, checked: boolean) {
		if (checked) {
			selectedProjectPaths = selectedProjectPaths.includes(projectPath)
				? selectedProjectPaths
				: [...selectedProjectPaths, projectPath];
		} else {
			selectedProjectPaths = selectedProjectPaths.filter((path) => path !== projectPath);
		}
	}

	function setHistoryImportDays(days: string) {
		historyImportDays = days;
	}

	async function loadDetectedProjects() {
		detectedLoading = true;
		detectedError = null;
		try {
			const response = await discoverImportableProjects(30);
			detectedProjects = response.projects.sort((a, b) =>
				projectName(a).localeCompare(projectName(b))
			);
		} catch (e) {
			detectedError = e instanceof Error ? e.message : 'Failed to load detected projects';
		} finally {
			detectedLoading = false;
		}
	}

	async function startTrackingSelected() {
		if (selectedProjectPaths.length === 0) return;
		savingSelection = true;
		saveSelectionError = null;
		websocketStore.clearImportStatus();
		try {
			await importProjects(selectedProjectPaths, historyImportDays);
			// Import runs async on the server; wait for completion via WebSocket.
			const result = await websocketStore.waitForImportComplete();
			if (result.state === 'error') {
				saveSelectionError = result.message;
			} else {
				selectedProjectPaths = [];
				await Promise.all([loadTrackedRows(), loadDetectedProjects()]);
			}
		} catch (e) {
			saveSelectionError = e instanceof Error ? e.message : 'Failed to import selected projects';
		} finally {
			savingSelection = false;
			websocketStore.clearImportStatus();
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
		<ProjectOnboarding
			{detectedProjects}
			{detectedLoading}
			{detectedError}
			{selectedProjectPaths}
			selectedHistoryDays={historyImportDays}
			{historyDayOptions}
			{savingSelection}
			{saveSelectionError}
			{importStatusMessage}
			onToggleSelection={toggleSelection}
			onHistoryDaysChange={setHistoryImportDays}
			onStartTrackingSelected={startTrackingSelected}
		/>
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
								headerLink={resolve(
									`/local/projects/${encodeURIComponent(row.project.id)}/commits`
								)}
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
