<script lang="ts">
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import { clearUpdateStatus } from '$lib/api';
	import { API_URL } from '$lib/config';
	import Popover from '$lib/components/Popover.svelte';

	let applying = $state(false);
	let applyError = $state('');

	let status = $derived(websocketStore.updateStatus);
	let isInstalled = $derived(status.state === 'installed');
	let isAvailable = $derived(status.state === 'available');
	let isLinux = $derived(status.platform === 'linux');

	const releaseUrl = $derived(
		status.version
			? `https://github.com/gelatinousdevelopment/buildermark/releases/tag/${status.version}`
			: ''
	);

	async function applyUpdate() {
		applying = true;
		applyError = '';
		try {
			const url = API_URL
				? `${API_URL}/api/v1/update-apply`
				: `${window.location.origin}/api/v1/update-apply`;
			const resp = await fetch(url, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' }
			});
			const data = await resp.json();
			if (!data.ok) {
				applyError = data.error || 'Update failed';
			}
		} catch {
			applyError = 'Failed to connect to server';
		} finally {
			applying = false;
		}
	}

	function openNativeSettings() {
		window.open('buildermark://settings/update', '_self');
	}

	function dismiss() {
		websocketStore.clearUpdateStatus();
	}

	async function ignore() {
		applyError = '';
		try {
			await clearUpdateStatus();
			websocketStore.clearUpdateStatus();
		} catch (error) {
			applyError = error instanceof Error ? error.message : 'Failed to ignore update';
		}
	}
</script>

<Popover position="below" padding="1rem 1.2rem">
	{#snippet popover()}
		<!-- eslint-disable svelte/no-navigation-without-resolve -->
		<div class="update-popover">
			{#if isInstalled}
				<div class="update-title">Updated to {status.version}</div>
				{#if status.previousVersion}
					<div class="update-detail">from {status.previousVersion}</div>
				{/if}
				{#if releaseUrl}
					<a href={releaseUrl} target="_blank" class="update-link">View release notes</a>
				{/if}
				<button class="update-action bordered small prominent" onclick={dismiss}>Dismiss</button>
			{:else if isAvailable}
				<div class="update-title">Version {status.version} is available</div>
				{#if releaseUrl}
					<a href={releaseUrl} target="_blank" class="update-link">View release notes</a>
				{/if}
				<div style:display="flex" style:gap="0.5rem">
					{#if isLinux}
						<button
							class="update-action bordered small prominent"
							onclick={applyUpdate}
							disabled={applying}
						>
							{applying ? 'Updating...' : 'Update Now'}
						</button>
					{:else}
						<button class="update-action bordered small prominent" onclick={openNativeSettings}>
							Open Update Settings
						</button>
					{/if}
					<button class="update-action bordered small" onclick={ignore}> Ignore </button>
				</div>
				{#if applyError}
					<div class="update-error">{applyError}</div>
				{/if}
			{/if}
		</div>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->
	{/snippet}
	<button class="update-pill" class:installed={isInstalled} class:available={isAvailable}>
		{#if isInstalled}
			{status.version} Installed
		{:else if isAvailable}
			{status.version} Available
		{/if}
	</button>
</Popover>

<style>
	.update-pill {
		border-radius: 999px;
		border: 0;
		color: light-dark(#fff, var(--accent-color-ultralight));
		cursor: pointer;
		font-size: 0.8rem;
		font-weight: 600;
		height: 20px;
		padding: 0 0.8rem;
		white-space: nowrap;
	}

	.update-pill.installed {
		background: light-dark(#2e8b3e, #3da64d);
	}

	.update-pill.installed:hover {
		background: light-dark(#247032, #4dba5c);
	}

	.update-pill.available {
		background: var(--accent-color-darker);
	}

	.update-pill.available:hover {
		background: var(--accent-color-darkest);
	}

	.update-popover {
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
		white-space: normal;
		min-width: 200px;
	}

	.update-title {
		font-weight: 600;
		font-size: 1rem;
	}

	.update-detail {
		font-size: 0.9rem;
		color: var(--color-text-secondary);
	}

	.update-link {
		font-size: 0.9rem;
	}

	.update-action {
		margin-top: 0.5rem;
	}

	.update-error {
		color: var(--status-color-red, #e53e3e);
		font-size: 0.85rem;
	}
</style>
