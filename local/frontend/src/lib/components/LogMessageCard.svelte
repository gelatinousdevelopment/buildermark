<script lang="ts">
	import { fmtTime } from '$lib/utils';
	import {
		renderMarkdown,
		messageTypeLabel,
		messageSummary,
		messageModel
	} from '$lib/messageUtils';
	import type { MessageRead } from '$lib/types';

	interface Props {
		message: MessageRead;
		expanded: boolean;
		/** When provided, the header becomes clickable to collapse. */
		onToggle?: () => void;
		/** Render only the message body and omit metadata/header elements. */
		contentOnly?: boolean;
		showRawJson?: boolean;
	}

	let { message, expanded, onToggle, contentOnly = false, showRawJson = false }: Props = $props();
</script>

{#if !contentOnly}
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div
		class="message-header"
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
		<strong>{messageTypeLabel(message)}</strong> &middot;
		{fmtTime(message.timestamp)}
		{#if messageModel(message)}
			<span class="message-model">{messageModel(message)}</span>
		{/if}
		<span class="expansion-indicator">
			<span class="chevron">{expanded ? '▾' : '▸'}</span>
		</span>
	</div>
	<div class="message-summary">{messageSummary(message)}</div>
{/if}
{#if contentOnly || expanded}
	<div class="message-content markdown-body">
		<!-- eslint-disable-next-line svelte/no-at-html-tags -->
		{@html renderMarkdown(message.content)}
	</div>
	{#if showRawJson && message.rawJson}
		<pre class="json"><div class="heading">Raw JSON</div>{JSON.stringify(
				JSON.parse(message.rawJson),
				null,
				2
			)}</pre>
	{/if}
{/if}

<style>
	.message-header {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.message-model {
		color: var(--color-text-faded);
	}

	.expansion-indicator {
		margin-left: auto;
		color: var(--color-text-tertiary);
	}

	.chevron {
		display: inline-block;
		width: 0.8rem;
	}

	.message-summary {
		font-size: 1rem;
		color: var(--color-text-strong);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.message-content {
		font-size: 1rem;
		margin-top: 0.35rem;
	}

	.markdown-body {
		font-size: 1rem;
	}

	pre.json {
		background: var(--color-code-block-json-background);
		border: 0.5px solid var(--color-code-block-json-border);
		border-radius: 5px;
		padding: 0.5rem;
		font-family: var(--font-family-monospace);
		font-size: 0.85rem;
		white-space: pre-wrap;
		text-wrap: normal;
	}

	pre.json .heading {
		font-weight: bold;
		margin-bottom: 0.5rem;
	}
</style>
