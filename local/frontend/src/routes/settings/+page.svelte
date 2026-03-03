<script lang="ts">
	import { resolve } from '$app/paths';
	import { scanHistory, updateLocalSettings } from '$lib/api';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import { settingsStore, type ContentWidth } from '$lib/stores/settings.svelte';
	import TeamServersSection from '$lib/TeamServersSection.svelte';

	let { data } = $props();

	const contentWidthOptions: { value: ContentWidth; label: string; description: string }[] = [
		{ value: 'default', label: 'Default', description: '1540px' },
		{ value: 'wider', label: 'Wider', description: '1800px' },
		{ value: 'full', label: 'Full', description: '100%' }
	];

	let projects = $derived(data.projects);
	let projectError = $derived(data.projectError);
	let localSettings = $state(data.localSettings);
	let localSettingsError = $derived(data.localSettingsError);

	let extraAgentHomesText = $state(data.localSettings?.extraAgentHomes?.join('\n') ?? '');
	let savingAgentHomes = $state(false);
	let agentHomesNotice: string | null = $state(null);
	let agentHomesError: string | null = $state(null);

	let historyImportDays = $state('7');
	let importingHistory = $state(false);
	let historyImportError: string | null = $state(null);
	let historyImportResult: string | null = $state(null);

	const historyImportDayOptions = ['7', '14', '30', '60', '90', '180', '365', 'all'];

	let importStatusMessage = $derived(
		websocketStore.getJob('history_scan')?.state === 'running'
			? (websocketStore.getJob('history_scan')?.message ?? null)
			: null
	);

	function projectName(project: { label: string; path: string }): string {
		return project.label || project.path;
	}

	function historyImportTimeframe(days: string): string {
		if (days === 'all') {
			return '876000h';
		}
		return `${Number(days) * 24}h`;
	}

	function historyOptionLabel(days: string): string {
		if (days === 'all') {
			return 'All';
		}
		return `${days} days`;
	}

	const missingSearchPaths = $derived(
		localSettings?.conversationSearchPaths.filter((entry) => !entry.exists) ?? []
	);

	async function saveAgentHomes() {
		savingAgentHomes = true;
		agentHomesNotice = null;
		agentHomesError = null;
		try {
			const homes = extraAgentHomesText
				.split('\n')
				.map((line) => line.trim())
				.filter(Boolean);
			const updated = await updateLocalSettings(homes);
			localSettings = updated;
			extraAgentHomesText = updated.extraAgentHomes.join('\n');
			agentHomesNotice = 'Saved. Restart the server to apply watcher changes.';
		} catch (e) {
			agentHomesError = e instanceof Error ? e.message : 'Failed to save extra agent homes';
		} finally {
			savingAgentHomes = false;
		}
	}

	async function importHistory() {
		if (importingHistory) return;
		importingHistory = true;
		historyImportError = null;
		historyImportResult = null;
		websocketStore.clearJob('history_scan');
		try {
			await scanHistory(historyImportTimeframe(historyImportDays));
			const result = await websocketStore.waitForJob('history_scan');
			if (result.state === 'error') {
				historyImportError = result.message;
				return;
			}
			historyImportResult = result.message;
		} catch (e) {
			historyImportError = e instanceof Error ? e.message : 'Failed to import history';
		} finally {
			importingHistory = false;
			websocketStore.clearJob('history_scan');
		}
	}
</script>

<div class="settings limited-content-width inset-when-limited-content-width">
	<h1>Global Settings</h1>

	{#if localSettingsError}
		<p class="error">{localSettingsError}</p>
	{:else if localSettings}
		<div class="columns">
			<div class="column">
				<div class="section">
					<h2>User Interface</h2>
					<p class="label">Page Max Width</p>
					<fieldset class="radio-group">
						{#each contentWidthOptions as option (option.value)}
							<label class="radio-option">
								<input
									type="radio"
									name="content-width"
									value={option.value}
									checked={settingsStore.contentWidth === option.value}
									onchange={() => (settingsStore.contentWidth = option.value)}
								/>
								<span class="radio-label">{option.label}</span>
								<span class="radio-description">{option.description}</span>
							</label>
						{/each}
					</fieldset>
				</div>

				<div class="section">
					<h2>Local Environment</h2>
					<table class="data bordered striped hoverable">
						<tbody>
							<tr>
								<td class="label-cell">Home Folder</td>
								<td class="path">{localSettings.homePath}</td>
							</tr>
							<tr>
								<td class="label-cell">Agent Search Paths</td>
								<td>
									{#if localSettings.conversationSearchPaths.length === 0}
										<span class="muted">No agent watchers are currently registered.</span>
									{:else}
										{#each localSettings.conversationSearchPaths as entry, index (index)}
											<div class="search-path-entry">
												<span class="agent">{entry.agent}</span>
												<span class="path">{entry.path}</span>{#if !entry.exists}<span
														class="muted"
													>
														(not found)</span
													>{/if}
											</div>
										{/each}
									{/if}
								</td>
							</tr>
							<tr>
								<td class="label-cell">Sqlite Database</td>
								<td class="path">{localSettings.dbPath}</td>
							</tr>
							<tr>
								<td class="label-cell">Server Port</td>
								<td class="path">{localSettings.listenAddr}</td>
							</tr>
						</tbody>
					</table>
					<br />
					<label class="label" for="extra-agent-homes">Extra Agent Home Folders</label>
					<p class="muted">
						One path per line. You can enter user home folders (or .claude/.codex/.gemini folders)
						from mounted filesystems or other local accounts.
					</p>
					<textarea
						id="extra-agent-homes"
						rows="4"
						bind:value={extraAgentHomesText}
						placeholder="/mnt/vm/home/dev"
						class="mono-area full-width"
					></textarea>
					<div class="actions">
						<button class="bordered small" onclick={saveAgentHomes} disabled={savingAgentHomes}>
							{savingAgentHomes ? 'Saving...' : 'Save Agent Folders'}
						</button>
					</div>
					{#if agentHomesError}<p class="error">{agentHomesError}</p>{/if}
					{#if agentHomesNotice}<p class="status">{agentHomesNotice}</p>{/if}
					{#if missingSearchPaths.length > 0}
						<p class="muted">Some configured agent folders are not currently found:</p>
						<ul>
							{#each missingSearchPaths as entry (entry.agent + entry.path)}
								<li><code>{entry.path}</code> ({entry.agent})</li>
							{/each}
						</ul>
					{/if}
				</div>

				<div class="section">
					<h2>Re-import Conversation History</h2>
					<p class="muted">This may take a while.</p>
					<div class="history-import-controls">
						<label for="history-days-select">Import window</label>
						<select
							id="history-days-select"
							bind:value={historyImportDays}
							disabled={importingHistory}
						>
							{#each historyImportDayOptions as option (option)}
								<option value={option}>{historyOptionLabel(option)}</option>
							{/each}
						</select>
						<button
							class="bordered small import-btn"
							onclick={importHistory}
							disabled={importingHistory}
						>
							{#if importingHistory}
								<span class="spinner" aria-hidden="true"></span>
								Importing...
							{:else}
								Import
							{/if}
						</button>
					</div>
					{#if importingHistory && importStatusMessage}
						<p class="import-status">{importStatusMessage}</p>
					{/if}
					{#if historyImportError}
						<p class="error">{historyImportError}</p>
					{:else if historyImportResult}
						<p class="status">{historyImportResult}</p>
					{/if}
				</div>
			</div>

			<hr class="divider" />

			<div class="column">
				<div class="section">
					<h2>Tracked Projects</h2>
					{#if projectError}
						<p class="error">{projectError}</p>
					{:else if projects.length === 0}
						<p class="muted">No tracked projects yet.</p>
					{:else}
						<table class="data bordered striped hoverable">
							<tbody>
								{#each projects as project (project.id)}
									<tr>
										<td>
											<a
												href={resolve('/projects/[project_id]', {
													project_id: project.id
												})}
												class="project-name">{projectName(project)}</a
											>
										</td>
										<td>
											<a
												href={resolve('/projects/[project_id]/settings', {
													project_id: project.id
												})}
												class="project-name">Settings</a
											>
										</td>
										<td><span class="project-path">{project.path}</span></td>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				</div>

				<div class="section">
					<TeamServersSection />
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.settings {
		background: var(--color-background-content);
		padding: 0rem;
		flex: 1;
		display: flex;
		flex-direction: column;
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

	.label {
		margin: 0;
		font-size: 0.9rem;
		color: var(--color-text-secondary);
		text-transform: uppercase;
	}

	.mono-area {
		font-family:
			ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New',
			monospace;
	}

	.full-width {
		width: 100%;
		box-sizing: border-box;
	}

	.path {
		font-family:
			ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New',
			monospace;
		word-break: break-all;
	}

	.muted {
		color: var(--color-text-faded);
		font-size: 0.85rem;
		margin: 0;
	}

	.label-cell {
		white-space: nowrap;
		vertical-align: top;
		color: var(--color-text-secondary);
		text-transform: uppercase;
		font-size: 0.9rem;
		letter-spacing: 0.02em;
	}

	.search-path-entry {
		padding: 0.15rem 0;
	}

	.history-import-controls {
		display: flex;
		gap: 0.6rem;
		align-items: center;
		flex-wrap: wrap;
	}

	.history-import-controls select {
		padding: 0.25rem 0.5rem;
		border: 1px solid var(--color-border-input);
		border-radius: 4px;
		background: var(--color-background-surface);
		color: var(--color-text);
		font-size: 0.85rem;
	}

	.import-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}

	.import-btn:disabled {
		opacity: 0.7;
		cursor: wait;
	}

	.spinner {
		width: 0.8rem;
		height: 0.8rem;
		border: 2px solid var(--color-spinner-border);
		border-top-color: var(--color-spinner-top);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.import-status {
		color: var(--color-text-secondary);
		font-size: 0.85rem;
		margin: 0.3rem 0 0 0;
		animation: fade-in 200ms ease;
	}

	@keyframes fade-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}

	.agent {
		display: inline-block;
		min-width: 4.5rem;
		font-weight: 600;
		text-transform: lowercase;
	}

	.radio-group {
		border: none;
		margin: 0.8rem 0 0 0;
		padding: 0;
		display: flex;
		flex-direction: row;
		gap: 2rem;
	}

	.radio-option {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.radio-option input[type='radio'] {
		margin: 0;
	}

	.radio-label {
		font-weight: 600;
	}

	.radio-description {
		color: var(--color-text-faded);
	}
</style>
