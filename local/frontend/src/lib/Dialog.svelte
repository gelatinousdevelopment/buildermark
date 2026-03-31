<script lang="ts">
	import { fade } from 'svelte/transition';
	import { quadInOut } from 'svelte/easing';
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

	function flyScale(
		node: HTMLElement,
		{ delay = 0, duration = 400, easing = quadInOut, x = 0, y = 0, scale = 0.9 } = {}
	) {
		const style = getComputedStyle(node);
		const o = +style.opacity;
		const transform = style.transform === 'none' ? '' : style.transform;

		return {
			delay,
			duration,
			easing,
			css: (t: number) => {
				const u = 1 - t;
				return `
					transform: ${transform} translate(${u * x}px, ${u * y}px) scale(${scale + (1 - scale) * t});
					opacity: ${t * o};
				`;
			}
		};
	}
</script>

<svelte:window onkeydown={(e) => open && e.key === 'Escape' && onclose()} />

{#if open}
	<div class="dialog-overlay">
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="dialog-backdrop"
			in:fade={{ duration: 100 }}
			out:fade={{ duration: 250 }}
			onclick={onclose}
		></div>
		<div
			class="dialog-panel"
			style:max-width={width ?? '440px'}
			in:flyScale={{ duration: 100, y: -100, scale: 0.8 }}
			out:flyScale={{ duration: 100, y: -60, scale: 0.9 }}
		>
			<div class="dialog-body">
				{#if title}
					<h3>{title}</h3>
				{/if}
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
		background: linear-gradient(
			180deg,
			var(--color-modal-bg),
			color-mix(in hsl, var(--color-modal-bg), transparent 10%)
		);
		backdrop-filter: blur(10px);
		border: var(--divider-width) solid var(--color-popover-border);
		border-radius: 12px;
		box-sizing: border-box;
		box-shadow: 0 4px 24px light-dark(rgba(0, 0, 0, 0.4), rgba(0, 0, 0, 0.8));
		display: flex;
		flex-direction: column;
		max-height: 90vh;
		overflow: hidden;
		padding: 0;
		position: relative;
		width: 90%;
	}

	@supports (corner-shape: squircle) {
		.dialog-panel {
			border-radius: 22px;
			corner-shape: squircle;
		}
	}

	.dialog-panel h3 {
		font-size: 1.1rem;
		margin: 0 0 1rem 0;
		padding: 0;
	}

	.dialog-body {
		flex: 1 1 auto;
		min-height: 0;
		overflow-y: auto;
		padding: 2.2rem 2rem 0 2rem;
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
		border-top: 0.5px solid light-dark(rgb(0, 0, 0, 0.2), black);
		display: flex;
		justify-content: flex-end;
		gap: 1rem;
		margin: 0;
		padding: 1.5rem 2rem 1.5rem 2rem;
		/*background: white;*/
		position: relative;
	}

	.dialog-actions::after {
		content: '';
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		height: 1px;
		background: light-dark(white, rgb(255, 255, 255, 0.1));
	}
</style>
