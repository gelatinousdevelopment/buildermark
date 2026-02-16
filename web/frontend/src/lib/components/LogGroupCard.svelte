<script lang="ts">
	import { isDiffMessage, messageModel, groupModelLabel } from '$lib/messageUtils';
	import type { MessageRead } from '$lib/types';
	import type { SvelteSet } from 'svelte/reactivity';
	import DiffMessageCard from './DiffMessageCard.svelte';
	import LogMessageCard from './LogMessageCard.svelte';

	interface Props {
		messages: MessageRead[];
		expanded: boolean;
		expandedMessages: SvelteSet<string>;
		onToggleMessage: (id: string) => void;
	}

	let { messages, expanded, expandedMessages, onToggleMessage }: Props = $props();

	function handleKeydown(e: KeyboardEvent, id: string) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			onToggleMessage(id);
		}
	}
</script>

<div class="log-group-header">
	<strong>{messages.length} logs from {groupModelLabel(messages)}</strong>
</div>
{#if expanded}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="log-group-items"
		onclick={(e: MouseEvent) => e.stopPropagation()}
		onkeydown={(e: KeyboardEvent) => e.stopPropagation()}
	>
		{#each messages as logMessage (logMessage.id)}
			{#if isDiffMessage(logMessage)}
				<div
					class="log-item log-item-collapsed"
					role="button"
					tabindex="0"
					onclick={() => onToggleMessage(logMessage.id)}
					onkeydown={(e: KeyboardEvent) => handleKeydown(e, logMessage.id)}
				>
					<DiffMessageCard
						timestamp={logMessage.timestamp}
						model={messageModel(logMessage)}
						content={logMessage.content}
						expanded={expandedMessages.has(logMessage.id)}
					/>
				</div>
			{:else}
				<div
					class="log-item log-item-collapsed"
					role="button"
					tabindex="0"
					onclick={() => onToggleMessage(logMessage.id)}
					onkeydown={(e: KeyboardEvent) => handleKeydown(e, logMessage.id)}
				>
					<LogMessageCard message={logMessage} expanded={expandedMessages.has(logMessage.id)} />
				</div>
			{/if}
		{/each}
	</div>
{/if}

<style>
	.log-group-header {
		font-size: 0.85rem;
		color: #666;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.log-group-items {
		margin-top: 0.5rem;
	}

	.log-item {
		margin-bottom: 0.5rem;
		padding: 0.75rem;
		border: 1px solid #eee;
		border-radius: 4px;
	}

	.log-item-collapsed {
		padding: 0.5rem 0.75rem;
		background: #fafafa;
		cursor: pointer;
	}

	.log-item-collapsed:hover {
		border-color: var(--accent-color);
		background: var(--accent-color-ultralight);
	}
</style>
