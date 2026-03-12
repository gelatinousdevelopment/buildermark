<script lang="ts">
	import { resolve } from '$app/paths';
	import { setPluginInstalled } from '$lib/api';
	import Icon from '$lib/Icon.svelte';
	import Popover from '$lib/components/Popover.svelte';
	import type { PluginHomeInfo, PluginInventoryResponse, PluginStatus } from '$lib/types';

	let { data } = $props();
	let inventory: PluginInventoryResponse | null = $state(null);
	let error: string | null = $state(null);
	let notice: string | null = $state(null);
	let busyKey: string | null = $state(null);
	let initialized = $state(false);

	const browserExtensions = [
		{
			name: 'Chrome',
			storeLabel: 'Chrome Web Store',
			storeUrl: 'https://chromewebstore.google.com/search/buildermark',
			sourceUrl: 'https://github.com/Buildermark/buildermark/tree/main/plugins/browser_extension'
		},
		{
			name: 'Safari',
			storeLabel: 'Mac App Store',
			storeUrl: 'https://apps.apple.com/us/search?term=buildermark',
			sourceUrl: 'https://github.com/Buildermark/buildermark/tree/main/plugins/browser_extension/safari'
		},
		{
			name: 'Firefox',
			storeLabel: 'Firefox Add-ons',
			storeUrl: 'https://addons.mozilla.org/en-US/firefox/search/?q=buildermark',
			sourceUrl: 'https://github.com/Buildermark/buildermark/tree/main/plugins/browser_extension'
		}
	] as const;

	$effect(() => {
		if (initialized) return;
		inventory = data.inventory;
		error = data.error;
		initialized = true;
	});

	function pluginFor(home: PluginHomeInfo, agent: string): PluginStatus | null {
		return home.plugins.find((plugin) => plugin.agent === agent) ?? null;
	}

	function actionLabel(plugin: PluginStatus): string {
		if (plugin.status === 'installed') return 'Uninstall';
		if (plugin.status === 'partial') return 'Repair';
		return 'Install';
	}

	function statusLabel(status: string): string {
		if (status === 'installed') return 'Installed';
		if (status === 'partial') return 'Partial';
		return 'Not installed';
	}

	function statusIcon(status: string): 'check' | 'x' {
		return status === 'installed' ? 'check' : 'x';
	}

	async function handlePluginToggle(homePath: string, plugin: PluginStatus) {
		const install = plugin.status !== 'installed';
		busyKey = `${homePath}:${plugin.agent}`;
		error = null;
		notice = null;

		try {
			inventory = await setPluginInstalled(homePath, plugin.agent, install);
			notice = install
				? `Installed ${plugin.name} in ${homePath}.`
				: `Removed ${plugin.name} from ${homePath}.`;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update plugin';
		} finally {
			busyKey = null;
		}
	}
</script>

<div class="plugins limited-content-width inset-when-limited-content-width">
	<div class="hero">
		<h1>Coding Agent Plugins & Skills</h1>
		<div class="description">
			<p>
				Install plugins for your agents, so you can rate and log analysis about each AI coding
				session by running the
				<code>bbrate</code> skill.
			</p>
			<p>For example, rate a conversation in Claude Code:</p>
			<code>› /bbrate <span class="faded">[0-5] [Optional note or feedback]</span></code>
		</div>
	</div>

	<div class="content">
		{#if error}
			<p class="error">{error}</p>
		{/if}
		{#if notice}
			<p class="status">{notice}</p>
		{/if}

		{#if inventory}
			<div class="table-wrap">
				<table class="data bordered hoverable plugins-table">
					<thead>
						<tr>
							<th>Home Folders</th>
							{#each inventory.agents as agent (agent.agent)}
								<th>
									<div class="agent-heading">
										<div>{agent.name}</div>
										<code>{agent.syntax}</code>
									</div>
								</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each inventory.homes as home (home.homePath)}
							<tr>
								<td class="home-cell">
									<div class="home-path">{home.homePath}</div>
								</td>
								{#each inventory.agents as agent (agent.agent)}
									{@const plugin = pluginFor(home, agent.agent)}
									<td>
										{#if plugin}
											<div class="plugin-cell">
												<Popover position="above" fixed={true} width="420px" padding="0.75rem">
													{#snippet popover()}
														<div class="plugin-popover">
															<div class="plugin-popover-title">{statusLabel(plugin.status)}</div>
															<div class="plugin-popover-paths">
																{#each plugin.paths as installPath (installPath)}
																	<code>{installPath}</code>
																{/each}
															</div>
														</div>
													{/snippet}
													<div class={`plugin-status ${plugin.status}`}>
														<Icon name={statusIcon(plugin.status)} width="14px" />
													</div>
												</Popover>
												<button
													class="bordered tiny"
													disabled={busyKey === `${home.homePath}:${plugin.agent}`}
													onclick={() => handlePluginToggle(home.homePath, plugin)}
												>
													{busyKey === `${home.homePath}:${plugin.agent}`
														? 'Working...'
														: actionLabel(plugin)}
												</button>
											</div>
										{:else}
											<span class="muted">Unavailable</span>
										{/if}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}

		<p class="muted">
			You can add extra home folders, such as from mounted virtual machine filesystems or other
			local accounts in <a href={resolve('/settings')}>Settings</a>.
		</p>

		<section class="browser-extensions">
			<h2>Browser Extension</h2>
			<p class="muted">
				Install Buildermark in your browser to rate sessions from web-based tools. You can install
				directly from each browser’s extension store or install from source in developer mode.
			</p>
			<div class="browser-extension-grid">
				{#each browserExtensions as extension (extension.name)}
					<article class="browser-extension-card">
						<h3>{extension.name}</h3>
						<div class="browser-extension-links">
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
							<a href={extension.storeUrl} target="_blank" rel="noreferrer noopener"
								>Install from {extension.storeLabel}</a
							>
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
							<a href={extension.sourceUrl} target="_blank" rel="noreferrer noopener"
								>Install from GitHub source</a
							>
						</div>
					</article>
				{/each}
			</div>
		</section>
	</div>
</div>

<style>
	.hero {
		border-bottom: 0.5px solid var(--color-divider);
	}

	.content {
		padding: 1.5rem;
	}

	.table-wrap {
		margin-bottom: 1.5rem;
		max-width: 100%;
		overflow-x: auto;
	}

	.plugins-table {
		table-layout: auto;
		width: max-content;
	}

	.plugins-table th {
		vertical-align: bottom;
	}

	.plugins-table th:first-child,
	.plugins-table td:first-child {
		width: 24rem;
	}

	.agent-heading {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.agent-heading code {
		background: var(--color-code-inline-bg);
		border: 1px solid var(--color-code-border);
		border-radius: 4px;
		color: var(--color-text-secondary);
		display: inline-block;
		font-family: var(--font-family-monospace);
		font-size: 0.76rem;
		font-weight: 500;
		padding: 0.12rem 0.35rem;
		width: fit-content;
	}

	.home-cell {
		vertical-align: top;
	}

	.home-path {
		color: var(--color-text-strong);
		font-family: var(--font-family-monospace);
		font-size: 0.9rem;
		line-height: 1.5;
		word-break: break-word;
	}

	.plugin-cell {
		align-items: center;
		display: flex;
		flex-direction: row;
		gap: 0.5rem;
		min-width: 8rem;
	}

	.plugin-status {
		align-items: center;
		border-radius: 999px;
		cursor: default;
		display: inline-flex;
		height: 24px;
		justify-content: center;
		padding: 0;
		white-space: nowrap;
		width: 24px;
	}

	.plugin-status.installed {
		background: color-mix(in srgb, var(--color-status-green) 18%, transparent);
		color: var(--color-status-green);
	}

	.plugin-status.partial {
		background: color-mix(in srgb, var(--color-status-yellow) 18%, transparent);
		color: var(--color-warning-text);
	}

	.plugin-status.missing {
		background: color-mix(in srgb, var(--color-status-red) 12%, transparent);
		color: var(--color-danger);
	}

	.plugin-status :global(svg) {
		display: block;
	}

	.plugin-popover {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
	}

	.plugin-popover-title {
		color: var(--color-text-strong);
		font-size: 0.85rem;
		font-weight: 600;
	}

	.plugin-popover-paths {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.plugin-popover-paths code {
		background: var(--color-code-inline-bg);
		border: 1px solid var(--color-code-border);
		border-radius: 4px;
		color: var(--color-text-secondary);
		display: block;
		font-family: var(--font-family-monospace);
		font-size: 0.82rem;
		padding: 0.2rem 0.4rem;
		word-break: break-word;
	}

	.muted {
		color: var(--color-text-secondary);
	}

	.status {
		color: var(--color-notice);
		display: none;
	}

	@media (max-width: 900px) {
		.plugins {
			padding: 1rem;
		}

		.plugins-table th:first-child,
		.plugins-table td:first-child {
			width: 18rem;
		}
	}

	.description {
		margin: -1rem 1.5rem 1.5rem 1.5rem;
	}

	.description p {
		margin: 0 0 0.5rem 0;
	}

	.description code {
		padding: 0.5rem 0.8rem;
		border: 1px solid var(--color-code-border);
		background: var(--color-code-inline-bg);
		margin-top: 0.5rem;
		border-radius: 4px;
		display: block;
		max-width: 34rem;
	}

	.description code .faded {
		opacity: 0.5;
	}

	.description p code {
		padding: 0.2rem 0.4rem;
		display: inline-block;
	}

	p.muted {
		margin: 0;
	}

	.browser-extensions {
		border-top: 0.5px solid var(--color-divider);
		margin-top: 1.5rem;
		padding-top: 1.5rem;
	}

	.browser-extensions h2 {
		font-size: 1.1rem;
		margin: 0;
	}

	.browser-extension-grid {
		display: grid;
		gap: 1rem;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		margin-top: 1rem;
	}

	.browser-extension-card {
		background: var(--color-surface);
		border: 1px solid var(--color-divider);
		border-radius: 6px;
		padding: 0.9rem;
	}

	.browser-extension-card h3 {
		font-size: 1rem;
		margin: 0;
	}

	.browser-extension-links {
		display: flex;
		flex-direction: column;
		gap: 0.45rem;
		margin-top: 0.6rem;
	}
</style>
