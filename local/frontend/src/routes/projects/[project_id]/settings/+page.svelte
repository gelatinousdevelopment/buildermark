<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import {
		deleteProject,
		refreshProjectCommits,
		setProjectLabel,
		setProjectPath,
		setProjectOldPaths,
		setProjectAltRemotes,
		setProjectIgnoreDiffPaths,
		setProjectIgnoreDefaultDiffPaths,
		setProjectTeamServer
	} from '$lib/api';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import Icon from '$lib/Icon.svelte';
	import Dialog from '$lib/Dialog.svelte';
	import { layoutStore } from '$lib/stores/layout.svelte';

	onMount(() => {
		layoutStore.hideContainer = true;
	});
	onDestroy(() => {
		layoutStore.hideContainer = false;
	});

	let { data } = $props();

	const defaultPaths = [
		'**/.git/**',
		'**/.next/**',
		'**/.nuxt/**',
		'**/__pycache__/**',
		'**/node_modules/**',
		'*.map',
		'*.min.css',
		'*.min.js',
		'bun.lockb',
		'Cargo.lock',
		'composer.lock',
		'Gemfile.lock',
		'go.sum',
		'npm-shrinkwrap.json',
		'package-lock.json',
		'packages.lock.json',
		'paket.lock',
		'pdm.lock',
		'Pipfile.lock',
		'pnpm-lock.yaml',
		'poetry.lock',
		'yarn.lock'
	];

	let project = $derived(data.project);
	let teamServers = $derived(data.teamServers);

	// svelte-ignore state_referenced_locally
	let label = $state(project.label ?? '');
	// svelte-ignore state_referenced_locally
	let path = $state(project.path ?? '');
	// svelte-ignore state_referenced_locally
	let oldPaths = $state(project.oldPaths ?? '');
	// svelte-ignore state_referenced_locally
	let altRemotes = $state(project.altRemotes ?? '');
	// svelte-ignore state_referenced_locally
	let ignoreDiffPaths = $state(project.ignoreDiffPaths ?? '');
	// svelte-ignore state_referenced_locally
	let ignoreDefaultDiffPaths = $state(project.ignoreDefaultDiffPaths ?? true);
	// svelte-ignore state_referenced_locally
	let teamServerId = $state(project.teamServerId ?? '');

	let showDefaultPaths = $state(false);
	let saving = $state(false);
	let error: string | null = $state(null);
	let notice: string | null = $state(null);

	let refreshing = $state(false);
	let refreshDays = $state('0');

	const refreshDayOptions = [
		{ value: '0', label: 'Recent only' },
		{ value: '1', label: '1 day' },
		{ value: '7', label: '7 days' },
		{ value: '14', label: '14 days' },
		{ value: '30', label: '30 days' },
		{ value: '60', label: '60 days' },
		{ value: '90', label: '90 days' },
		{ value: '180', label: '180 days' },
		{ value: '365', label: '365 days' },
		{ value: '36500', label: 'All' }
	];

	let refreshStatus = $derived.by(() => {
		const job = websocketStore.getJob('commit_refresh');
		if (!job || !project.id) return null;
		if (job.projectId && job.projectId !== project.id) return null;
		return job;
	});

	let refreshBusy = $derived(refreshing || refreshStatus?.state === 'running');

	let showDeleteModal = $state(false);
	let deleteConfirmName = $state('');
	let deleting = $state(false);
	let deleteError: string | null = $state(null);

	let projectDisplayName = $derived(label || path || project.path);
	let recomputeStatus = $derived.by(() => {
		const job = websocketStore.getJob('diff_recompute');
		if (!job) return null;
		return job;
	});

	async function save() {
		saving = true;
		error = null;
		notice = null;
		websocketStore.clearJob('diff_recompute');
		try {
			if (label) await setProjectLabel(project.id, label);
			await setProjectPath(project.id, path);
			await setProjectTeamServer(project.id, teamServerId);
			await setProjectOldPaths(project.id, oldPaths);
			await setProjectAltRemotes(project.id, altRemotes);
			await setProjectIgnoreDiffPaths(project.id, ignoreDiffPaths);
			await setProjectIgnoreDefaultDiffPaths(project.id, ignoreDefaultDiffPaths);
			notice = 'Saved.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	async function refreshCommits() {
		refreshing = true;
		websocketStore.clearJob('commit_refresh');
		try {
			const days = Number(refreshDays);
			await refreshProjectCommits(project.id, '', days > 0 ? days : undefined);
		} catch {
			refreshing = false;
			return;
		}
		refreshing = false;
	}

	async function confirmDeleteProject() {
		deleting = true;
		deleteError = null;
		try {
			await deleteProject(project.id);
			goto(resolve('/projects'));
		} catch (e) {
			deleteError = e instanceof Error ? e.message : 'Failed to delete project';
		} finally {
			deleting = false;
		}
	}
</script>

<div class="outer">
	<div class="page">
		<h1>Project Settings</h1>

		<div class="columns">
			<div class="column">
				<div class="section">
					<div class="row">
						<label class="field-label" for="project-label">Project Name</label>
						<input
							id="project-label"
							class="project-label"
							type="text"
							bind:value={label}
							placeholder="Project label"
						/>
					</div>
					<div class="row">
						<label class="field-label" for="project-path">Path to Git Repository</label>
						<input
							id="project-path"
							type="text"
							bind:value={path}
							placeholder="/path/to/project"
							class="mono-input"
						/>
					</div>
					<div class="row">
						<label class="field-label" for="team-server-select">Team Server</label>
						<div class="field">
							<select id="team-server-select" bind:value={teamServerId} class="team-server-select">
								<option value="">None</option>
								{#each teamServers as server (server.id)}
									<option value={server.id}>{server.label}</option>
								{/each}
							</select>
						</div>
					</div>
				</div>

				<div class="section">
					<h2>Ignore Paths for Agent Attribution</h2>
					<div class="defaults-row">
						<label class="checkbox-label">
							<input type="checkbox" bind:checked={ignoreDefaultDiffPaths} />
							Ignore default paths
						</label>
						<button
							class="info-btn"
							title="Show default paths"
							onclick={() => (showDefaultPaths = !showDefaultPaths)}
						>
							<Icon name="info" width="15px" />
						</button>
					</div>
					{#if showDefaultPaths}
						<ul class="paths-list">
							{#each defaultPaths as p (p)}
								<li><code>{p}</code></li>
							{/each}
						</ul>
					{/if}
					<p class="hint">One glob path per line.</p>
					<textarea
						id="ignore-diff-paths"
						bind:value={ignoreDiffPaths}
						rows="4"
						spellcheck="false"
						placeholder="Glob patterns, one per line"
					></textarea>
				</div>

				<div class="section">
					<h2>Old Filesystem Paths</h2>
					<p class="hint">
						Match conversations created from previous project locations. One absolute path per line.
					</p>
					<textarea
						id="old-paths"
						bind:value={oldPaths}
						rows="4"
						spellcheck="false"
						placeholder="/old/path/to/repo"
					></textarea>
				</div>

				{#if project.gitWorktreePaths}
					{@const paths = project.gitWorktreePaths.split('\n')}
					<div class="section">
						<h2>Git Worktree Paths</h2>
						<p class="hint">
							Auto-detected git worktrees. Conversations from these paths are included in this
							project.
						</p>
						<ul class="paths-list">
							{#each paths as p (p)}
								<li><code>{p}</code></li>
							{/each}
						</ul>
					</div>
				{/if}

				<div class="section">
					<h2>Alternate Git Remote URLs</h2>
					<p class="hint">
						Current remote: <code>{project.remote || 'none'}</code>
					</p>
					<p class="hint">
						Match conversations from other remote URLs for this repository. One URL per line.
					</p>
					<textarea
						id="alt-remotes"
						bind:value={altRemotes}
						rows="4"
						spellcheck="false"
						placeholder="git@github.com:other-org/repo.git"
					></textarea>
				</div>

				<div class="section">
					<h2>Alternate Coding Agent Paths</h2>
					<p class="hint">
						Need to track conversations from other user folders or mounted filesystems? Configure
						that in <a href={resolve('/settings')}>Buildermark Local Settings</a>.
					</p>
				</div>

				<div class="actions">
					<button class="bordered prominent" disabled={saving} onclick={save}
						>{saving ? 'Saving...' : 'Save Settings'}</button
					>
					{#if notice}
						<span class="notice">{notice}</span>
					{/if}
				</div>
				{#if error}
					<p class="error">{error}</p>
				{/if}
			</div>

			<hr class="divider" />

			<div class="column">
				<div class="advanced-zone">
					<h2>Advanced Actions</h2>
					<p class="advanced-description">
						Re-scan and recompute commit attribution for this project. Select a time window to check
						for missed commits and resolve any with missing parents (shallow clones).
					</p>
					<div class="actions">
						<select bind:value={refreshDays} disabled={refreshBusy} class="refresh-days-select">
							{#each refreshDayOptions as opt (opt.value)}
								<option value={opt.value}>{opt.label}</option>
							{/each}
						</select>
						<button class="bordered small" disabled={refreshBusy} onclick={refreshCommits}
							>{refreshBusy ? 'Refreshing...' : 'Refresh Commits'}</button
						>
					</div>
					{#if refreshStatus}
						<p class="refresh-status" class:refresh-error={refreshStatus.state === 'error'}>
							{refreshStatus.message}
						</p>
					{:else if recomputeStatus}
						<p class="refresh-status" class:refresh-error={recomputeStatus.state === 'error'}>
							{recomputeStatus.message}
						</p>
					{/if}
				</div>

				<div class="danger-zone">
					<h2>Danger Zone</h2>
					<p class="danger-description">
						Permanently delete this project and all its data, including conversations, messages,
						ratings, and commits.
					</p>
					<button class="bordered prominent btn-danger" onclick={() => (showDeleteModal = true)}
						>Delete Project</button
					>
				</div>
			</div>
		</div>
	</div>
</div>

<Dialog open={showDeleteModal} title="Delete Project" onclose={() => (showDeleteModal = false)}>
	<p>
		This will permanently delete <strong>{projectDisplayName}</strong> and all associated data. This action
		cannot be undone.
	</p>
	<label for="delete-confirm">
		Type <strong>{projectDisplayName}</strong> to confirm:
	</label>
	<input
		id="delete-confirm"
		type="text"
		bind:value={deleteConfirmName}
		placeholder={projectDisplayName}
		autocomplete="off"
	/>
	{#if deleteError}
		<p class="error">{deleteError}</p>
	{/if}
	{#snippet actions()}
		<button class="bordered small" onclick={() => (showDeleteModal = false)}>Cancel</button>
		<button
			class="bordered prominent small btn-danger"
			disabled={deleteConfirmName !== projectDisplayName || deleting}
			onclick={confirmDeleteProject}
		>
			{deleting ? 'Deleting...' : 'Delete Project'}
		</button>
	{/snippet}
</Dialog>

<style>
	.outer {
		flex: 1;
	}

	.page {
		display: flex;
		flex-direction: column;
		gap: 0;
		margin: 0;

		max-width: 1100px;
		margin: 0 auto;

		background: var(--color-background-content);
		border-radius: var(--content-section-border-radius);
		border: var(--divider-width) solid var(--color-divider);
		box-sizing: border-box;
		margin: 1.5rem auto;
		width: 100%;
	}

	h1 {
		margin: 0;
		font-size: 1.2rem;
		padding: 1.5rem;
	}

	h2 {
		margin: 0;
		font-size: 1rem;
	}

	.columns {
		display: flex;
		flex-direction: row;
		gap: 0rem;
		flex: 1;
		border-top: var(--divider-width) solid var(--color-divider);
	}

	.column {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
		padding: 1rem 1.5rem 1.5rem 1.5rem;
	}

	.column:first-of-type {
		flex: 1.67;
	}

	hr.divider {
		background: var(--color-divider);
		border: 0;
		margin: 0;
		min-width: var(--divider-width);
		width: var(--divider-width);
	}

	.section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.section .row {
		display: flex;
		gap: 1rem;
	}

	.section .row .field-label {
		max-width: 230px;
		width: 230px;
	}

	.section .row .field {
		width: 100%;
	}

	@media (max-width: 1200px) {
		.columns {
			flex-direction: column;
		}
	}

	.defaults-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.9rem;
		cursor: pointer;
	}

	.info-btn {
		all: unset;
		color: var(--color-text);
	}

	.info-btn:hover {
		color: var(--accent-color);
	}

	.paths-list {
		margin: 0;
		padding-left: 1.5rem;
		font-size: 0.9rem;
		color: var(--color-text-secondary);
		line-height: 1.6;
	}

	.paths-list code {
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.78rem;
		background: var(--color-button-bg);
		padding: 0.1rem 0.3rem;
		border-radius: 2px;
	}

	.field-label {
		display: block;
		margin: 0.5rem 0;
	}

	input[type='text'] {
		width: 100%;
		padding: 0.4rem 0.6rem;
		font-size: 1rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		box-sizing: border-box;
	}

	input[type='text'].mono-input {
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.9rem;
	}

	.team-server-select {
		padding: 0.4rem 0.3rem;
		font-size: 1rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		min-width: 200px;
		width: 100%;
	}

	textarea {
		width: 100%;
		max-width: 100%;
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.9rem;
		line-height: 1.45;
		padding: 0.65rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		box-sizing: border-box;
	}

	.hint {
		margin: 0 0 0.3rem 0;
		font-size: 1rem;
		color: var(--color-text-faded);
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.notice {
		font-size: 0.85rem;
		color: var(--color-notice);
	}

	.refresh-status {
		margin: 0.4rem 0 0;
		font-size: 0.85rem;
		color: var(--color-notice);
	}

	.refresh-error {
		color: var(--color-danger, #d32f2f);
	}

	.advanced-zone {
		padding-top: 1rem;
	}

	.advanced-zone h2 {
		margin: 0 0 0.5rem;
		font-size: 1rem;
	}

	.advanced-description {
		margin: 0 0 0.75rem;
		font-size: 0.85rem;
		color: var(--color-text-faded);
	}

	.danger-zone {
		padding-top: 1rem;
		border-top: var(--divider-width) solid var(--color-divider);
	}

	.danger-zone h2 {
		margin: 0 0 0.5rem;
		font-size: 1rem;
		color: var(--color-danger-hover);
	}

	.danger-description {
		margin: 0 0 0.75rem;
		font-size: 0.85rem;
		color: var(--color-text-faded);
	}

	.refresh-days-select {
		padding: 0.25rem 0.5rem;
		border: 1px solid var(--color-border-input);
		border-radius: 4px;
		background: var(--color-background-surface);
		color: var(--color-text);
		font-size: 0.85rem;
	}
</style>
