<script lang="ts">
	import { onMount, tick } from 'svelte';
	import Icon from '$lib/Icon.svelte';
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
		if (open && !prevOpen && closing) {
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
		const targetScale = pillRect.width / panelRect.width / 2;

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
				wrapper.classList.remove('closing-animation');
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
			<a
				href="https://buildermark.dev"
				target="_blank"
				class="logo-wordmark"
				style:color="var(--color-text)"><Icon name="buildermarkWordmark" width="140px" /></a
			>
			<hr style:height="2px" style:background="var(--color-text)" />
			<h2>Rate, measure, and benchmark your AI coding sessions.</h2>
			<p>
				This website is a read-only demo of Buildermark Local. You can browse all of the prompts
				that I wrote to create it in less than a month.
			</p>
			<h2>Open source, local-first, and free.</h2>
			<p>
				Buildermark Local runs on your dev computer (macOS, Linux, and Windows) with a very light
				footprint (written in Go), a UI on localhost, and is <a
					href="https://github.com/gelatinousdevelopment/buildermark"
					target="_blank">open source on github</a
				>. Nothing leaves your machine, not even usage data.
			</p>
			<p style:display="flex" style:gap="0.5rem" style:align-items="center">
				<span>Download at</span>
				<a
					href="https://buildermark.dev"
					target="_blank"
					class="bordered prominent small"
					style:width="fit-content">buildermark.dev</a
				>
				<span>or</span>
				<a
					href="https://github.com/gelatinousdevelopment/buildermark/releases"
					target="_blank"
					class="bordered prominent small"
					style:width="fit-content">GitHub</a
				>
			</p>
		</div>
		{#snippet actions()}
			<button class="bordered prominent" onclick={handleClose}>Close</button>
		{/snippet}
	</Dialog>
</div>

<style>
	.read-only-dialog h2 {
		font-size: 1rem;
		font-weight: bold;
		margin: 1.2rem 0 0.8rem 0;
		line-height: 1.3;
	}

	.read-only-dialog p {
		font-size: 1rem;
		line-height: 1.3;
	}

	:global(.closing-animation .dialog-panel) {
		/*animation: close-to-pill 300ms cubic-bezier(0.4, 0, 0.7, 1) forwards;*/
		animation: close-to-pill 250ms ease-in forwards;
		transform-origin: var(--close-origin);
	}

	:global(.closing-animation .dialog-backdrop) {
		animation: backdrop-fade 200ms ease-out forwards;
	}

	@keyframes close-to-pill {
		0% {
			transform: perspective(400px) rotateX(0deg) scale(1);
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
			filter: brightness(1);
		}
		25% {
			filter: brightness(0.7);
		}
		50% {
			filter: brightness(1);
		}
		75% {
			filter: brightness(0.7);
		}
		100% {
			filter: brightness(1);
		}
	}

	.mobile-read-only-dialog :global(.dialog-panel) {
		transform: scale(var(--mobile-dialog-scale));
		padding: 2.5rem;
	}

	.mobile-read-only-dialog :global(.dialog-panel .logo-wordmark .icon) {
		color: var(--color-text);
		width: 220px !important;
	}

	.mobile-read-only-dialog :global(.dialog-panel h3) {
		font-size: 1.6rem;
	}

	.mobile-read-only-dialog :global(.icon) {
		width: 220px;
	}

	.mobile-read-only-dialog .read-only-dialog h2 {
		font-size: 1.5rem;
		margin: 1.8rem 0 1rem 0;
	}

	.mobile-read-only-dialog .read-only-dialog p,
	.mobile-read-only-dialog .read-only-dialog span {
		font-size: 1.5rem;
	}

	.mobile-read-only-dialog .read-only-dialog a {
		font-size: 1.5rem;
	}

	.mobile-read-only-dialog :global(.dialog-body p) {
		font-size: 1.5rem;
		margin: 1rem 0 1rem 0;
	}

	.mobile-read-only-dialog :global(.bordered) {
		font-size: 1.4rem;
		padding: 0.5rem 1.2rem;
	}

	.mobile-read-only-dialog :global(.dialog-actions button) {
		border-width: 1px;
		font-size: 1.4rem;
		padding: 1rem 2rem;
		margin-top: 1rem;
		width: 100%;
	}
</style>
