<script lang="ts">
	import { resolve } from '$app/paths';
	import { scanHistory, updateLocalSettings } from '$lib/api';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import {
		settingsStore,
		type ContentWidth,
		type CommitSortOrder,
		type Theme
	} from '$lib/stores/settings.svelte';
	import TeamServersSection from '$lib/TeamServersSection.svelte';

	let { data } = $props();

	const contentWidthOptions: { value: ContentWidth; label: string; description: string }[] = [
		{ value: 'default', label: 'Default', description: '1540px' },
		{ value: 'wider', label: 'Wider', description: '1800px' },
		{ value: 'full', label: 'Full', description: '100%' }
	];

	const themeOptions: { value: Theme; label: string }[] = [
		{ value: 'system', label: 'System' },
		{ value: 'light', label: 'Light' },
		{ value: 'dark', label: 'Dark' }
	];

	const commitSortOrderOptions: { value: CommitSortOrder; label: string }[] = [
		{ value: 'desc', label: 'Newest First' },
		{ value: 'asc', label: 'Oldest First' }
	];

	let projects = $derived(data.projects);
	let projectError = $derived(data.projectError);
	let localSettings = $derived(data.localSettings);
	let localSettingsError = $derived(data.localSettingsError);

	let extraAgentHomesText = $derived(data.localSettings?.extraAgentHomes?.join('\n') ?? '');
	let extraLocalUserEmailsText = $derived(
		data.localSettings?.extraLocalUserEmails?.join('\n') ?? ''
	);
	let savingSettings = $state(false);
	let settingsNotice: string | null = $state(null);
	let settingsError: string | null = $state(null);

	let historyImportDays = $state('7');
	let historyImportAgent = $state('');
	let historyImportReplaceDerivedDiffs = $state(false);
	let importingHistory = $state(false);
	let historyImportError: string | null = $state(null);
	let historyImportResult: string | null = $state(null);

	const historyImportDayOptions = ['1', '3', '5', '7', '14', '30', '60', '90', '180', '365', 'all'];

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
		return `${days} ${days === '1' ? 'day' : 'days'}`;
	}

	const missingSearchPaths = $derived(
		localSettings?.conversationSearchPaths.filter((entry) => !entry.exists) ?? []
	);

	async function saveSettings() {
		savingSettings = true;
		settingsNotice = null;
		settingsError = null;
		try {
			const homes = extraAgentHomesText
				.split('\n')
				.map((line) => line.trim())
				.filter(Boolean);
			const emails = extraLocalUserEmailsText
				.split('\n')
				.map((line) => line.trim())
				.filter(Boolean);
			const updated = await updateLocalSettings(homes, emails);
			localSettings = updated;
			extraAgentHomesText = updated.extraAgentHomes.join('\n');
			extraLocalUserEmailsText = updated.extraLocalUserEmails.join('\n');
			settingsNotice = 'Saved. Watchers updated.';
		} catch (e) {
			settingsError = e instanceof Error ? e.message : 'Failed to save settings';
		} finally {
			savingSettings = false;
		}
	}

	async function importHistory() {
		if (importingHistory) return;
		importingHistory = true;
		historyImportError = null;
		historyImportResult = null;
		websocketStore.clearJob('history_scan');
		try {
			await scanHistory(
				historyImportTimeframe(historyImportDays),
				historyImportAgent,
				historyImportReplaceDerivedDiffs
			);
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

<div class="outer">
	<div class="page">
		<h1>Buildermark Local Settings</h1>

		{#if localSettingsError}
			<p class="error">{localSettingsError}</p>
		{:else if localSettings}
			<div class="columns">
				<div class="column">
					<div class="section">
						<h2>User Interface</h2>
						<p class="label">Theme</p>
						<fieldset class="radio-group">
							{#each themeOptions as option (option.value)}
								<label class="radio-option">
									<input
										type="radio"
										name="theme"
										value={option.value}
										checked={settingsStore.theme === option.value}
										onchange={() => (settingsStore.theme = option.value)}
									/>
									<span class="radio-label">{option.label}</span>
								</label>
							{/each}
						</fieldset>

						<br />
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

						<br />
						<p class="label">Default Commit Sort</p>
						<fieldset class="radio-group">
							{#each commitSortOrderOptions as option (option.value)}
								<label class="radio-option">
									<input
										type="radio"
										name="commit-sort-order"
										value={option.value}
										checked={settingsStore.commitSortOrder === option.value}
										onchange={() => (settingsStore.commitSortOrder = option.value)}
									/>
									<span class="radio-label">{option.label}</span>
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
						{#if missingSearchPaths.length > 0}
							<p class="muted">Some configured agent folders are not currently found:</p>
							<ul>
								{#each missingSearchPaths as entry (entry.agent + entry.path)}
									<li><code>{entry.path}</code> ({entry.agent})</li>
								{/each}
							</ul>
						{/if}
						<br />
						<label class="label" for="extra-local-user-emails">Extra Local User Emails</label>
						<p class="muted">
							One email per line. Commits by these email addresses are treated as local user
							commits. Default: noreply@anthropic.com
						</p>
						<textarea
							id="extra-local-user-emails"
							rows="4"
							bind:value={extraLocalUserEmailsText}
							placeholder="noreply@anthropic.com"
							class="mono-area full-width"
						></textarea>
						<br />
						<div class="actions">
							<button class="bordered prominent" onclick={saveSettings} disabled={savingSettings}>
								{savingSettings ? 'Saving...' : 'Save Settings'}
							</button>
						</div>
						{#if settingsError}<p class="error">{settingsError}</p>{/if}
						{#if settingsNotice}<p class="status">{settingsNotice}</p>{/if}
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
													class="project-name">Project Settings</a
												>
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						{/if}
					</div>

					<div class="section">
						<TeamServersSection />
					</div>

					<div class="section import-history">
						<h2>Re-import Conversation History</h2>
						<div class="history-import-controls">
							<div>
								<label for="history-agent-select">Agent: </label>
								<select
									id="history-agent-select"
									bind:value={historyImportAgent}
									disabled={importingHistory}
								>
									<option value="">All Agents</option>
									{#each localSettings?.localAgents ?? [] as agent (agent)}
										<option value={agent}>{agent}</option>
									{/each}
								</select>
							</div>
							<div>
								<label for="history-days-select">Import Window: </label>
								<select
									id="history-days-select"
									bind:value={historyImportDays}
									disabled={importingHistory}
								>
									{#each historyImportDayOptions as option (option)}
										<option value={option}>{historyOptionLabel(option)}</option>
									{/each}
								</select>
							</div>
							<div>
								<label class="history-import-checkbox">
									<input
										type="checkbox"
										bind:checked={historyImportReplaceDerivedDiffs}
										disabled={importingHistory}
									/>
									<span>Replace derived diffs</span>
								</label>
							</div>
							<div>
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
						</div>
						{#if importingHistory && importStatusMessage}
							<p class="import-status">{importStatusMessage}</p>
						{:else}
							<p class="import-status">This may take a while.</p>
						{/if}
						{#if historyImportError}
							<p class="error">{historyImportError}</p>
						{:else if historyImportResult}
							<p class="status">{historyImportResult}</p>
						{/if}
					</div>
				</div>
			</div>
		{/if}
	</div>
</div>

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
		border: 0.5px solid var(--color-divider);
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
		padding: 1rem 1.5rem 1.5rem 1.5rem;
	}

	.column:first-of-type {
		flex: 1.67;
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
		gap: 0.2rem;
	}

	@media (max-width: 1200px) {
		.columns {
			flex-direction: column;
		}
	}

	ul {
		margin: 0.5rem;
		padding: 0 1rem;
	}

	ul li {
		font-size: 0.9rem;
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
		align-items: flex-start;
		flex-direction: column;
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

	.history-import-checkbox {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		color: var(--color-text);
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
		width: 0.6rem;
		height: 0.6rem;
		border: 2px solid var(--color-spinner-border);
		border-top-color: var(--color-spinner-top-on-content);
		border-radius: 50%;
		animation: spin 0.3s linear infinite;
		display: inline-block;
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
</style>
