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
	}

	let { message, expanded }: Props = $props();
</script>

<div class="message-header">
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
{#if expanded}
	<div class="message-content markdown-body">
		<!-- eslint-disable-next-line svelte/no-at-html-tags -->
		{@html renderMarkdown(message.content)}
	</div>
{/if}

<style>
	.message-header {
		font-size: 0.85rem;
		color: #666;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.message-model {
		color: #9a9a9a;
	}

	.expansion-indicator {
		margin-left: auto;
		color: #888;
	}

	.chevron {
		display: inline-block;
		width: 0.8rem;
	}

	.message-summary {
		font-size: 1rem;
		color: #333;
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
</style>
