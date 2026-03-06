<script lang="ts">
	import Popover from '$lib/components/Popover.svelte';
	import { websocketStore, getWsUrl } from '$lib/stores/websocket.svelte';

	let isActive = $derived(websocketStore.hasActiveJob);
	let activeMessage = $derived.by(() => {
		for (const job of Object.values(websocketStore.activeJobs)) {
			if (job.state === 'running' && job.message) return job.message;
		}
		return null;
	});
</script>

<Popover position="below" fixed={true} width="280px" padding="0.75rem" wrapWidth="100%">
	<button class="status-button" title="Server: {websocketStore.connectionState}">
		<div class="status-container">
			<div class="status-dot {websocketStore.connectionState}" class:active={isActive}></div>
			{#if isActive}
				<div class="activity-ring"></div>
				<div class="activity-ring2"></div>
			{/if}
		</div>
	</button>
	{#snippet popover()}
		<div class="popover-content">
			<div class="connection-row">
				<div class="status-dot-small {websocketStore.connectionState}"></div>
				<span class="connection-label">
					{websocketStore.connectionState === 'connected'
						? 'Connected to ' + getWsUrl()
						: websocketStore.connectionState === 'connecting'
							? 'Connecting to ' + getWsUrl()
							: 'Disconnected'}
				</span>
			</div>
			{#if isActive && activeMessage}
				<div class="activity-message">{activeMessage}</div>
			{/if}
		</div>
	{/snippet}
</Popover>

<style>
	.status-button {
		background: none;
		border: none;
		align-content: center;
		box-sizing: border-box;
		height: 40px;
		padding: 0 1.3rem;
		cursor: default;
		width: 100%;
	}

	.status-button:hover {
		background: var(--accent-color-ultralight);
	}

	.status-container {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.status-dot {
		border-radius: 99px;
		width: 1rem;
		height: 1rem;
		background: var(--color-status-disconnected);
		transition: background 300ms ease;
	}

	.status-dot.connected {
		background: var(--color-status-green);
	}

	.status-dot.connecting {
		background: var(--color-status-connecting);
		animation: pulse 1.5s ease-in-out infinite;
	}

	.status-dot.disconnected {
		background: var(--color-status-disconnected);
	}

	.activity-ring {
		position: absolute;
		inset: -2px;
		border-radius: 50%;
		background: conic-gradient(
			var(--color-status-green) 0deg,
			var(--color-status-green) 60deg,
			transparent 180deg
		);
		-webkit-mask: radial-gradient(
			farthest-side,
			transparent calc(100% - 1px),
			#000 calc(100% - 1px)
		);
		mask: radial-gradient(farthest-side, transparent calc(100% - 1px), #000 calc(100% - 1px));
		animation: spin 0.4s linear infinite reverse;
	}

	.activity-ring2 {
		position: absolute;
		inset: -4px;
		border-radius: 50%;
		background: conic-gradient(
			transparent 0deg,
			var(--color-status-green) 180deg,
			var(--color-status-green) 220deg,
			transparent 220deg
		);
		-webkit-mask: radial-gradient(
			farthest-side,
			transparent calc(100% - 1px),
			#000 calc(100% - 1px)
		);
		mask: radial-gradient(farthest-side, transparent calc(100% - 1px), #000 calc(100% - 1px));
		animation: spin 0.8s linear infinite;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.4;
		}
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.popover-content {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.connection-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.status-dot-small {
		border-radius: 99px;
		width: 0.5rem;
		height: 0.5rem;
		background: var(--color-status-disconnected);
		flex-shrink: 0;
	}

	.status-dot-small.connected {
		background: var(--color-status-green);
	}

	.status-dot-small.connecting {
		background: var(--color-status-connecting);
	}

	.status-dot-small.disconnected {
		background: var(--color-status-disconnected);
	}

	.connection-label {
		font-size: 0.85rem;
		color: var(--color-text);
	}

	.activity-message {
		font-size: 0.8rem;
		color: var(--color-text-secondary);
		white-space: normal;
		word-break: break-word;
		line-height: 1.4;
	}
</style>
