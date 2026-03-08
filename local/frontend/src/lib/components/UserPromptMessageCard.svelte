<script lang="ts">
	import Icon from '$lib/Icon.svelte';
	import { fmtTime } from '$lib/utils';
	import { renderMarkdown, messageModel, planPromptTitle } from '$lib/messageUtils';
	import type { MessageRead } from '$lib/types';

	interface Props {
		message: MessageRead;
	}

	let { message }: Props = $props();
	let planTitle = $derived(planPromptTitle(message.content));
</script>

<div class="message-header" class:plan-header={Boolean(planTitle)}>
	<strong class="message-role">
		{#if planTitle}
			<span class="plan-icon"><Icon name="document" width="14px" /></span>
		{/if}
		{message.role}
	</strong>
	&middot; {fmtTime(message.timestamp)}
	{#if messageModel(message)}
		<span class="message-model">{messageModel(message)}</span>
	{/if}
</div>
<div class="message-content markdown-body" class:plan-content={Boolean(planTitle)}>
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	{@html renderMarkdown(message.content)}
</div>

<style>
	.message-header {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.plan-header {
		color: var(--color-relationship-foreground);
	}

	.message-model {
		opacity: 0.7;
	}

	.message-role {
		align-items: center;
		display: flex;
		gap: 0.4rem;
	}

	.plan-icon {
		color: var(--color-relationship-icon);
		display: inline-flex;
		flex-shrink: 0;
	}

	.message-content {
		font-size: 1rem;
		margin-top: 0.35rem;
	}

	.plan-content {
		color: var(--color-relationship-foreground);
	}

	.markdown-body {
		font-size: 1rem;
	}
</style>
