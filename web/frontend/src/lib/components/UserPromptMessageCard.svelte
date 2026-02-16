<script lang="ts">
	import { fmtTime } from '$lib/utils';
	import { renderMarkdown, messageModel } from '$lib/messageUtils';
	import type { MessageRead } from '$lib/types';

	interface Props {
		message: MessageRead;
	}

	let { message }: Props = $props();
</script>

<div class="message-header">
	<strong>{message.role}</strong> &middot; {fmtTime(message.timestamp)}
	{#if messageModel(message)}
		<span class="message-model">{messageModel(message)}</span>
	{/if}
</div>
<div class="message-content markdown-body">
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	{@html renderMarkdown(message.content)}
</div>

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

	.message-content {
		font-size: 1rem;
		margin-top: 0.35rem;
	}

	.markdown-body {
		font-size: 1rem;
	}
</style>
