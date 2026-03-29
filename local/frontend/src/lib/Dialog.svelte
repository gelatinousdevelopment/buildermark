<script lang="ts">
	import { fade, fly } from 'svelte/transition';
	import type { Snippet } from 'svelte';

	interface Props {
		open: boolean;
		title?: string;
		onclose: () => void;
		children: Snippet;
		actions: Snippet;
		width?: string;
	}

	let { open, title, onclose, children, actions, width }: Props = $props();
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="dialog-overlay" onkeydown={(e) => e.key === 'Escape' && onclose()}>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<div
			class="dialog-backdrop"
			in:fade={{ duration: 100 }}
			out:fade={{ duration: 250 }}
			onclick={onclose}
		></div>
		<div
			class="dialog-panel"
			style:max-width={width ?? '440px'}
			in:fly={{ duration: 100, y: -60 }}
			out:fly={{ duration: 100, y: -60 }}
		>
			{#if title}
				<h3>{title}</h3>
			{/if}
			<div class="dialog-body">
				{@render children()}
			</div>
			<div class="dialog-actions">
				{@render actions()}
			</div>
		</div>
	</div>
{/if}

<style>
	.dialog-overlay {
		position: fixed;
		inset: 0;
		z-index: 1000;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.dialog-backdrop {
		position: absolute;
		inset: 0;
		background: var(--color-modal-backdrop);
	}

	.dialog-panel {
		border: 0.5px solid var(--color-popover-border);
		position: relative;
		background: var(--color-modal-bg);
		border-radius: 8px;
		padding: 1.5rem;
		width: 90%;
		box-shadow: 0 4px 24px var(--color-popover-shadow);
	}

	.dialog-panel h3 {
		margin: 0 0 0.75rem;
		font-size: 1.1rem;
	}

	.dialog-body :global(p) {
		margin: 0 0 0.75rem;
		font-size: 0.9rem;
		line-height: 1.45;
	}

	.dialog-body :global(label) {
		display: block;
		margin-bottom: 0.35rem;
		font-weight: 600;
	}

	.dialog-body :global(input[type='text']) {
		width: 100%;
		padding: 0.4rem 0.6rem;
		font-size: 1rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		box-sizing: border-box;
		margin-bottom: 0.75rem;
	}

	.dialog-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 1rem;
	}
</style>
