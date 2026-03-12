<script lang="ts">
	import { onMount } from 'svelte';
	import { debugSendNotification, debugGetWSClients } from '$lib/api';
	import { websocketStore } from '$lib/stores/websocket.svelte';

	let notifKind = $state('debug_test');
	let notifTitle = $state('Test notification');
	let notifBody = $state('This is a test notification from the debug page');
	let notifUrl = $state('/projects');
	let sending = $state(false);
	let sendResult: string | null = $state(null);

	let wsClients = $derived(websocketStore.wsClients);

	// Fetch initial client counts on mount (before any ws_clients event arrives)
	onMount(async () => {
		try {
			const data = await debugGetWSClients();
			// The websocket store will take over once a ws_clients event fires,
			// but this seeds the initial values.
			initialClients = data;
		} catch {
			// ignore — the WS will update us
		}
	});

	let initialClients: { frontend: number; notification: number } | null = $state(null);

	let displayClients = $derived(
		wsClients.frontend > 0 || wsClients.notification > 0 ? wsClients : (initialClients ?? wsClients)
	);

	async function sendTestNotification() {
		sending = true;
		sendResult = null;
		try {
			await debugSendNotification(notifKind, notifTitle, notifBody, notifUrl);
			sendResult = 'Sent!';
		} catch (e) {
			sendResult = e instanceof Error ? e.message : 'Failed to send';
		} finally {
			sending = false;
		}
	}
</script>

<div class="debug limited-content-width inset-when-limited-content-width">
	<h1>Debug</h1>

	<div class="sections">
		<div class="section">
			<h2>Send Test Notification</h2>
			<p class="muted">
				Sends a notification over the dedicated notifications WebSocket (<code
					>/api/v1/notifications/ws</code
				>).
			</p>
			<div class="form">
				<label>
					<span class="field-label">Kind</span>
					<input type="text" bind:value={notifKind} />
				</label>
				<label>
					<span class="field-label">Title</span>
					<input type="text" bind:value={notifTitle} />
				</label>
				<label>
					<span class="field-label">Body</span>
					<input type="text" bind:value={notifBody} />
				</label>
				<label>
					<span class="field-label">URL</span>
					<input type="text" bind:value={notifUrl} placeholder="/projects" />
				</label>
				<div class="actions">
					<button class="bordered prominent" onclick={sendTestNotification} disabled={sending}>
						{sending ? 'Sending...' : 'Send Notification'}
					</button>
					{#if sendResult}
						<span class="result">{sendResult}</span>
					{/if}
				</div>
			</div>
		</div>

		<div class="section">
			<h2>WebSocket Clients</h2>
			<p class="muted">Updates in realtime as clients connect and disconnect.</p>
			<table class="data bordered striped">
				<thead>
					<tr>
						<th>WebSocket</th>
						<th>Endpoint</th>
						<th>Connected Clients</th>
					</tr>
				</thead>
				<tbody>
					<tr>
						<td>Frontend</td>
						<td><code>/api/v1/ws</code></td>
						<td class="count">{displayClients.frontend}</td>
					</tr>
					<tr>
						<td>Notifications</td>
						<td><code>/api/v1/notifications/ws</code></td>
						<td class="count">{displayClients.notification}</td>
					</tr>
				</tbody>
			</table>
			<p class="muted connection-state">
				WebSocket: <strong>{websocketStore.connectionState}</strong>
			</p>
		</div>
	</div>
</div>

<style>
	.debug {
		background: var(--color-background-content);
		padding: 0;
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
		margin: 0 0 0.2rem 0;
		font-size: 1rem;
	}

	.sections {
		display: flex;
		flex-direction: column;
		gap: 2rem;
		padding: 0 1.5rem 1.5rem 1.5rem;
		border-top: 0.5px solid var(--color-divider);
	}

	.section {
		padding-top: 1rem;
	}

	.section + .section {
		border-top: 0.5px solid var(--color-divider);
	}

	.muted {
		color: var(--color-text-faded);
		font-size: 0.85rem;
		margin: 0 0 0.5rem 0;
	}

	.form {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		max-width: 500px;
	}

	.form label {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.field-label {
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--color-text-secondary);
		text-transform: uppercase;
	}

	.form input {
		padding: 0.4rem 0.6rem;
		border: 1px solid var(--color-border-input);
		border-radius: 4px;
		background: var(--color-background-surface);
		color: var(--color-text);
		font-size: 0.9rem;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.8rem;
		margin-top: 0.4rem;
	}

	.result {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
	}

	table {
		max-width: 500px;
	}

	th {
		text-align: left;
		font-size: 0.8rem;
		text-transform: uppercase;
		color: var(--color-text-secondary);
	}

	td.count {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	code {
		font-size: 0.85em;
		background: var(--color-background-surface);
		padding: 0.1rem 0.3rem;
		border-radius: 3px;
	}

	.connection-state {
		margin-top: 0.5rem;
	}
</style>
