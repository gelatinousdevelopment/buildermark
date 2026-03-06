<script lang="ts">
	import {
		isDiffMessage,
		messageModel,
		groupModelLabel,
		groupTimeSpan,
		formatDuration
	} from '$lib/messageUtils';
	import type { MessageRead } from '$lib/types';
	import type { SvelteSet } from 'svelte/reactivity';
	import DiffMessageCard from './DiffMessageCard.svelte';
	import LogMessageCard from './LogMessageCard.svelte';
	import AgentTag from './AgentTag.svelte';

	interface Props {
		messages: MessageRead[];
		agent?: string;
		expanded: boolean;
		wideMode?: boolean;
		selectedMessageId?: string | null;
		expandedMessages: SvelteSet<string>;
		onToggleMessage: (id: string) => void;
		onSelectMessage?: (message: MessageRead) => void;
		/** When provided, the group header becomes clickable to collapse. */
		onToggle?: () => void;
		/** Render agent tag in subtle mode in group header and message rows. */
		subtleAgentTag?: boolean;
	}

	let {
		messages,
		agent = 'agent',
		expanded,
		wideMode = false,
		selectedMessageId = null,
		expandedMessages,
		onToggleMessage,
		onSelectMessage,
		onToggle,
		subtleAgentTag = false
	}: Props = $props();

	let timeSpan = $derived(groupTimeSpan(messages));
	let groupedModelLabel = $derived(groupModelLabel(messages, agent));

	function selectMessage(message: MessageRead) {
		onSelectMessage?.(message);
	}

	function handleKeydown(e: KeyboardEvent, message: MessageRead, expanded: boolean) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			handleLogItemClick(message, expanded);
		}
	}

	function handleLogItemClick(message: MessageRead, expanded: boolean) {
		if (!wideMode && !expanded) onToggleMessage(message.id);
		selectMessage(message);
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div
	class="log-group-header"
	class:log-group-header-clickable={onToggle}
	role={onToggle ? 'button' : undefined}
	tabindex={onToggle ? 0 : undefined}
	onclick={(e: MouseEvent) => {
		if (onToggle) {
			e.stopPropagation();
			onToggle();
		}
	}}
	onkeydown={(e: KeyboardEvent) => {
		if (onToggle && (e.key === 'Enter' || e.key === ' ')) {
			e.preventDefault();
			e.stopPropagation();
			onToggle();
		}
	}}
>
	<strong>
		{messages.length}
		{messages.length == 1 ? 'message' : 'messages'} from
		{#if groupedModelLabel === agent}
			<AgentTag {agent} subtle={subtleAgentTag} />
		{:else}
			{groupedModelLabel}
		{/if}
		{#if timeSpan > 0}
			&nbsp;({formatDuration(timeSpan)})
		{/if}
	</strong>
</div>
{#if expanded}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="log-group-items"
		onclick={(e: MouseEvent) => e.stopPropagation()}
		onkeydown={(e: KeyboardEvent) => e.stopPropagation()}
	>
		{#each messages as logMessage (logMessage.id)}
			{@const logSelected = selectedMessageId === logMessage.id}
			{@const logExpanded = wideMode ? logSelected : expandedMessages.has(logMessage.id)}
			{@const logInteractive = wideMode || !logExpanded}
			{#if isDiffMessage(logMessage)}
				<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
				<div
					class="log-item"
					class:log-item-selected={logSelected}
					class:log-item-collapsed={!logExpanded}
					role={logInteractive ? 'button' : undefined}
					tabindex={logInteractive ? 0 : undefined}
					onclick={() => handleLogItemClick(logMessage, logExpanded)}
					onkeydown={logInteractive
						? (e: KeyboardEvent) => handleKeydown(e, logMessage, logExpanded)
						: undefined}
				>
					<DiffMessageCard
						timestamp={logMessage.timestamp}
						role={logMessage.role === 'agent' ? agent : logMessage.role}
						model={messageModel(logMessage)}
						content={logMessage.content}
						expanded={logExpanded}
						{subtleAgentTag}
						onToggle={!wideMode && logExpanded ? () => onToggleMessage(logMessage.id) : undefined}
					/>
				</div>
			{:else}
				<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
				<div
					class="log-item"
					class:log-item-selected={logSelected}
					class:log-item-collapsed={!logExpanded}
					role={logInteractive ? 'button' : undefined}
					tabindex={logInteractive ? 0 : undefined}
					onclick={() => handleLogItemClick(logMessage, logExpanded)}
					onkeydown={logInteractive
						? (e: KeyboardEvent) => handleKeydown(e, logMessage, logExpanded)
						: undefined}
				>
					<LogMessageCard
						message={logMessage}
						expanded={logExpanded}
						onToggle={!wideMode && logExpanded ? () => onToggleMessage(logMessage.id) : undefined}
					/>
				</div>
			{/if}
		{/each}
	</div>
{/if}

<style>
	.log-group-header {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.15rem 0.3rem;
		border: 1px solid transparent;
		border-radius: 5px;
		margin: -1px;
	}

	.log-group-header strong {
		color: var(--color-text-tertiary);
		font-size: 0.9rem;
		font-weight: normal;
	}

	.log-group-header.log-group-header-clickable {
		cursor: pointer;
		border: 1px solid transparent;
		/*margin: calc(-0.15rem - 1px) calc(-0.3rem - 1px);*/
	}

	.log-group-header.log-group-header-clickable:hover {
		background: var(--accent-color-ultralight);
		border-color: var(--accent-color);
		color: var(--accent-color);
	}

	.log-group-header.log-group-header-clickable strong {
		color: var(--accent-color);
	}

	.log-group-items {
		margin-top: 0.5rem;
	}

	.log-item {
		margin-bottom: 0.5rem;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--color-border-light);
		border-radius: 4px;
	}

	.log-item-collapsed {
		padding: 0.5rem 0.75rem;
		background: var(--color-background-subtle);
		cursor: pointer;
	}

	.log-item-selected {
		border-color: var(--color-selected-border);
		background: var(--color-selected-bg);
	}

	.log-item-selected:hover {
		border-color: var(--color-selected-border-hover);
		background: var(--color-selected-bg-hover);
	}

	.log-item-collapsed:hover {
		border-color: var(--accent-color);
		background: var(--accent-color-ultralight);
	}
</style>
