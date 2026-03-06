<script lang="ts">
	import { fmtTime } from '$lib/utils';
	import { renderMarkdown, messageModel } from '$lib/messageUtils';
	import type { MessageRead } from '$lib/types';

	interface Props {
		message: MessageRead;
		agent: string;
	}

	let { message, agent }: Props = $props();

	function normalizeAgentName(name: string): string {
		return name
			.trim()
			.replace(/[^a-zA-Z0-9]/g, '-')
			.toLowerCase();
	}

	let normalized = $derived(normalizeAgentName(agent));
</script>

<div
	class="agent-message-card"
	style={`--card-bg: var(--agent-background-color-${normalized}, var(--color-background-content)); --card-border: var(--agent-border-color-${normalized}, var(--color-border-medium));`}
>
	<div class="message-header">
		<strong>{agent}</strong> &middot; final answer &middot; {fmtTime(message.timestamp)}
		{#if messageModel(message)}
			<span class="message-model">{messageModel(message)}</span>
		{/if}
	</div>
	<div class="message-content markdown-body">
		<!-- eslint-disable-next-line svelte/no-at-html-tags -->
		{@html renderMarkdown(message.content)}
	</div>
</div>

<style>
	.agent-message-card {
		background: var(--card-bg);
		border: 1px solid var(--card-border);
		border-radius: 8px;
		padding: 0.6rem 1rem;
	}

	.message-header {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.message-model {
		opacity: 0.7;
	}

	.message-content {
		font-size: 1rem;
		margin-top: 0.35rem;
	}

	.markdown-body {
		font-size: 1rem;
	}
</style>
