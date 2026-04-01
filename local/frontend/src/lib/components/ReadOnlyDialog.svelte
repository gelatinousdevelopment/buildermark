<script lang="ts">
	import { onMount, tick } from 'svelte';
	import Dialog from '$lib/Dialog.svelte';

	interface Props {
		open: boolean;
		onclose: () => void;
		pillButton?: HTMLButtonElement;
	}

	let { open = $bindable(), onclose, pillButton }: Props = $props();

	let closing = $state(false);
	let dialogOpen = $derived(open || closing);

	let isMobileDevice = $state(false);
	let mobileDialogScale = $state(1);
	let mobileDialogWidth = $state('500px');
	let wrapper: HTMLDivElement;
	let orientationKey = $state(0);

	onMount(() => {
		if (window.screen.width < 900) {
			isMobileDevice = true;
		}
	});

	$effect(() => {
		if (!isMobileDevice) return;
		const handler = () => {
			orientationKey++;
		};
		window.addEventListener('resize', handler);
		return () => window.removeEventListener('resize', handler);
	});

	$effect(() => {
		// eslint-disable-next-line @typescript-eslint/no-unused-expressions
		orientationKey;
		if (open && isMobileDevice && wrapper) {
			const isLandscape = window.innerWidth > window.innerHeight;
			mobileDialogWidth = isLandscape ? '900px' : '500px';
			tick().then(() => {
				const panel = wrapper.querySelector('.dialog-panel') as HTMLElement | null;
				if (panel) {
					const widthScale = (1300 * 0.9) / panel.offsetWidth;
					const heightScale = (window.innerHeight * 0.9) / panel.offsetHeight;
					mobileDialogScale = Math.min(widthScale, heightScale);
				}
			});
		}
	});

	// Cancel closing animation if dialog re-opens after parent set open=false
	let prevOpen = $state(open);
	$effect(() => {
		if (open && !prevOpen) {
			closing = false;
			wrapper?.classList.remove('closing-animation');
		}
		prevOpen = open;
	});

	function handleClose() {
		if (!pillButton || !wrapper) {
			onclose();
			return;
		}

		const panel = wrapper.querySelector('.dialog-panel') as HTMLElement | null;
		if (!panel) {
			onclose();
			return;
		}

		closing = true;

		const panelRect = panel.getBoundingClientRect();
		const pillRect = pillButton.getBoundingClientRect();

		const pillCenterX = pillRect.left + pillRect.width / 2;
		const pillCenterY = pillRect.top + pillRect.height / 2;
		const targetScale = pillRect.width / panelRect.width / 4;

		// transform-origin at pill center relative to panel
		const originX = pillCenterX - panelRect.left;
		const originY = pillCenterY - panelRect.top;

		// Compensate for scale not reaching 0: the residual offset is s * (center - origin)
		const compensateX = targetScale * (originX - panelRect.width / 2);
		const compensateY = targetScale * (originY - panelRect.height / 2);

		wrapper.style.setProperty('--close-origin', `${originX}px ${originY}px`);
		wrapper.style.setProperty('--close-dx', `${compensateX}px`);
		wrapper.style.setProperty('--close-dy', `${compensateY}px`);
		wrapper.style.setProperty('--close-scale', `${targetScale}`);

		wrapper.classList.add('closing-animation');

		panel.addEventListener(
			'animationend',
			() => {
				closing = false;
				onclose();

				// Flash the pill to draw attention
				if (pillButton) {
					pillButton.classList.add('pill-flash');
					pillButton.addEventListener(
						'animationend',
						() => pillButton.classList.remove('pill-flash'),
						{ once: true }
					);
				}
			},
			{ once: true }
		);
	}
</script>

<div
	class:mobile-read-only-dialog={isMobileDevice}
	style:--mobile-dialog-scale={mobileDialogScale}
	bind:this={wrapper}
>
	<Dialog
		open={dialogOpen}
		onclose={handleClose}
		width={isMobileDevice ? mobileDialogWidth : '500px'}
	>
		<div class="read-only-dialog">
			<div class="header">
				<img src="/buildermark-app-icon.png" width="36" height="36" alt="app icon" />
				<span>Buildermark</span>
			</div>
			<div class="content">
				<h2>Rate and measure your coding agent workflow.</h2>
				<p>
					This website is a read-only demo of Buildermark. You can browse a snapshot of my AI agent
					conversations that wrote 94% of the code in the project, as of March 30, 2026.
				</p>
				<p>See your work in the same way by running Buildermark on your system.</p>
				<h2>Open source, local-first, and private.</h2>
				<p>
					Buildermark runs on your dev computer (macOS, Linux, and Windows) with a very light
					footprint (written in Go) and a UI on localhost. Nothing leaves your machine, not even
					usage data.
				</p>
				<p>
					Download at
					<a href="https://buildermark.dev" target="_blank" style:width="fit-content"
						>buildermark.dev</a
					>
					or
					<a
						href="https://github.com/gelatinousdevelopment/buildermark/releases"
						target="_blank"
						style:width="fit-content">GitHub</a
					>
				</p>
			</div>
		</div>
		{#snippet actions()}
			<a class="bordered" style:font-weight="bold" href="https://buildermark.dev"
				>Back to homepage</a
			>
			<button class="bordered prominent" style:min-width="7rem" onclick={handleClose}
				>Dismiss</button
			>
		{/snippet}
	</Dialog>
</div>

<style>
	.header {
		align-items: center;
		display: flex;
		font-weight: 600;
		gap: 0.7rem;
		justify-content: center;
		margin: 0 0 1.8rem 0;
	}

	.header img {
		filter: drop-shadow(0 0.5px 1px rgb(0, 0, 0, 0.3));
	}

	.header span {
		background: linear-gradient(
			0deg,
			color-mix(in hsl, var(--accent-color), black 20%),
			color-mix(in hsl, var(--accent-color), white 20%)
		);
		background-clip: text;
		font-size: 1.8rem;
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
	}

	.content {
		margin-bottom: 1.7rem;
	}

	.read-only-dialog h2 {
		font-size: 1.1rem;
		font-weight: bold;
		margin: 1.2rem 0 0.4rem 0;
		line-height: 1.3;
	}

	.read-only-dialog p {
		font-size: 1rem;
		line-height: 1.4;
	}

	:global(.closing-animation .dialog-panel) {
		animation: close-to-pill 200ms ease-out forwards;
		transform-origin: var(--close-origin);
	}

	:global(.closing-animation .dialog-backdrop) {
		animation: backdrop-fade 200ms ease-out forwards;
	}

	@keyframes close-to-pill {
		0% {
			transform: perspective(600px) rotateX(0deg) scale(1);
			opacity: 1;
		}
		90% {
			opacity: 1;
		}
		98% {
			opacity: 0;
		}
		100% {
			transform: perspective(400px) rotateX(45deg) translate(var(--close-dx), var(--close-dy))
				scale(var(--close-scale));
			opacity: 0;
		}
	}

	@keyframes backdrop-fade {
		0% {
			opacity: 1;
		}
		100% {
			opacity: 0;
		}
	}

	:global(.pill-flash) {
		animation: pill-flash 300ms ease-out;
	}

	@keyframes pill-flash {
		0% {
			filter: brightness(0.7);
			transform: scale(0.9);
		}
		50% {
			transform: scale(1);
		}
		100% {
			filter: brightness(1);
		}
	}

	.mobile-read-only-dialog :global(.dialog-panel .header) {
		gap: 1rem;
	}

	.mobile-read-only-dialog :global(.dialog-panel .header img) {
		width: 50px;
		height: 50px;
	}

	.mobile-read-only-dialog :global(.dialog-panel .header span) {
		font-size: 2.6rem;
	}

	.mobile-read-only-dialog :global(.dialog-panel) {
		transform: scale(var(--mobile-dialog-scale));
		padding: 0.5rem;
	}

	.mobile-read-only-dialog :global(.dialog-panel h3) {
		font-size: 1.6rem;
	}

	.mobile-read-only-dialog .read-only-dialog h2 {
		font-size: 1.6rem;
	}

	.mobile-read-only-dialog .read-only-dialog p,
	.mobile-read-only-dialog .read-only-dialog span,
	.mobile-read-only-dialog .read-only-dialog a {
		font-size: 1.5rem;
	}

	.mobile-read-only-dialog :global(.dialog-body p) {
		font-size: 1.5rem;
		margin: 0.5rem 0 1.5rem 0;
	}

	.mobile-read-only-dialog :global(.bordered) {
		font-size: 1.5rem;
	}

	.mobile-read-only-dialog :global(.dialog-actions) {
		flex-direction: column;
	}

	.mobile-read-only-dialog :global(.dialog-actions button) {
		border-width: 1px;
		font-size: 1.4rem;
		padding: 1rem 2rem;
		margin-top: 1rem;
		width: 100%;
	}
</style>
