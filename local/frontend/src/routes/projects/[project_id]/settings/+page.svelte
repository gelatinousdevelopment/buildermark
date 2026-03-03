<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import {
		deleteProject,
		setProjectLabel,
		setProjectPath,
		setProjectOldPaths,
		setProjectIgnoreDiffPaths,
		setProjectIgnoreDefaultDiffPaths,
		setProjectTeamServer
	} from '$lib/api';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import Icon from '$lib/Icon.svelte';
	import Dialog from '$lib/Dialog.svelte';

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
	let ignoreDiffPaths = $state(project.ignoreDiffPaths ?? '');
	// svelte-ignore state_referenced_locally
	let ignoreDefaultDiffPaths = $state(project.ignoreDefaultDiffPaths ?? true);
	// svelte-ignore state_referenced_locally
	let teamServerId = $state(project.teamServerId ?? '');

	let showDefaultPaths = $state(false);
	let saving = $state(false);
	let error: string | null = $state(null);
	let notice: string | null = $state(null);

	let showDeleteModal = $state(false);
	let deleteConfirmName = $state('');
	let deleting = $state(false);
	let deleteError: string | null = $state(null);

	let projectDisplayName = $derived(label || path || project.path);
	let recomputeStatusMessage = $derived.by(() => {
		const job = websocketStore.getJob('diff_recompute');
		if (!job || !project.id) return null;
		if (job.message?.includes(project.id)) return job.message;
		return null;
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
			await setProjectIgnoreDiffPaths(project.id, ignoreDiffPaths);
			await setProjectIgnoreDefaultDiffPaths(project.id, ignoreDefaultDiffPaths);
			notice = 'Saved. Diff recompute started.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
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

<div class="page">
	<h1>Project Settings</h1>

	<div class="columns">
		<div class="column">
			<div class="section">
				<label class="field-label" for="project-label">Project Name</label>
				<input
					id="project-label"
					class="project-label"
					type="text"
					bind:value={label}
					placeholder="Project label"
				/>

				<label class="field-label" for="project-path">Path to Git Repository</label>
				<input
					id="project-path"
					type="text"
					bind:value={path}
					placeholder="/path/to/project"
					class="mono-input"
				/>
			</div>

			<div class="section">
				<h2>Ignore Paths for Diff Matching</h2>
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
					<ul class="default-paths-list">
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

			<div class="section">
				<h2>Alternate Coding Agent Paths</h2>
				<p class="hint">
					Need to track conversations from other user folders or mounted filesystems? Configure that
					in <a href={resolve('/settings')}>Global Settings</a>.
				</p>
			</div>

			<div class="actions">
				<button class="bordered prominent" disabled={saving} onclick={save}
					>{saving ? 'Saving...' : 'Save Settings'}</button
				>
				{#if notice}
					<span class="notice">{notice}</span>
				{/if}
				{#if recomputeStatusMessage}
					<span class="notice">{recomputeStatusMessage}</span>
				{/if}
			</div>
			{#if error}
				<p class="error">{error}</p>
			{/if}

			<br />

			<div class="danger-zone">
				<h2>Danger Zone</h2>
				<p class="danger-description">
					Permanently delete this project and all its data, including conversations, messages,
					ratings, and commits.
				</p>
				<button class="btn-danger" onclick={() => (showDeleteModal = true)}>Delete Project</button>
			</div>
		</div>

		<hr class="divider" />

		<div class="column">
			<div class="section">
				<h2>Team Server</h2>
				<select id="team-server-select" bind:value={teamServerId} class="team-server-select">
					<option value="">None</option>
					{#each teamServers as server (server.id)}
						<option value={server.id}>{server.label}</option>
					{/each}
				</select>
				<div class="hint">Coming soon.</div>
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
			class="btn-danger"
			disabled={deleteConfirmName !== projectDisplayName || deleting}
			onclick={confirmDeleteProject}
		>
			{deleting ? 'Deleting...' : 'Delete Project'}
		</button>
	{/snippet}
</Dialog>

<style>
	.page {
		padding: 0rem;
	}

	h1 {
		margin: 0;
		font-size: 1.2rem;
		padding: 1.5rem;
	}

	h2 {
		color: var(--accent-color-darkest);
		margin: 0.5rem 0;
		font-size: 1rem;
	}

	.columns {
		display: flex;
		flex-direction: row;
		gap: 0rem;
		flex: 1;
		border-top: 0.5px solid var(--color-divider);
	}

	.column {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2rem;
		padding: 1.5rem 2rem;
	}

	hr.divider {
		background: var(--color-divider);
		border: 0;
		margin: 0;
		min-width: 0.5px;
		width: 0.5px;
	}

	.section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
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
		margin-bottom: 0.5rem;
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

	.default-paths-list {
		margin: 0 0 1rem;
		padding-left: 1.5rem;
		font-size: 0.8rem;
		color: var(--color-text-secondary);
		line-height: 1.6;
	}

	.default-paths-list code {
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.78rem;
		background: var(--color-button-bg);
		padding: 0.1rem 0.3rem;
		border-radius: 2px;
	}

	.field-label {
		display: block;
		margin: 0.5rem 0;
		font-weight: 600;
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

	input[type='text'].project-label {
		font-size: 1.3rem;
	}

	.mono-input {
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.9rem;
	}

	.team-server-select {
		padding: 0.4rem 0.6rem;
		font-size: 1rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		min-width: 200px;
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

	.danger-zone {
		padding-top: 1rem;
		border-top: 0.5px solid var(--color-divider);
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

	.btn-danger {
		padding: 0.35rem 0.8rem;
		font-size: 0.9rem;
		font-weight: bold;
		line-height: 1.4;
		border: 1px solid var(--color-danger);
		border-radius: 3px;
		background: var(--color-danger);
		color: #fff;
		cursor: pointer;
	}

	.btn-danger:hover {
		background: var(--color-danger-hover);
		border-color: var(--color-danger-hover);
	}

	.btn-danger:disabled {
		opacity: 0.5;
		cursor: default;
	}
</style>
