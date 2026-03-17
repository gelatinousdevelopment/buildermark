<script lang="ts">
	import { onMount, tick } from 'svelte';
	import Icon from '$lib/Icon.svelte';
	import Dialog from '$lib/Dialog.svelte';

	interface Props {
		open: boolean;
		onclose: () => void;
	}

	let { open = $bindable(), onclose }: Props = $props();

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
</script>

<div
	class:mobile-read-only-dialog={isMobileDevice}
	style:--mobile-dialog-scale={mobileDialogScale}
	bind:this={wrapper}
>
	<Dialog {open} {onclose} width={isMobileDevice ? mobileDialogWidth : '500px'}>
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
			<!-- <h2>Download</h2> -->
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
			<p style:margin-top="1rem">
				<a href="https://buildermark.dev" target="_blank">Buildermark Team Server</a> is coming soon.
			</p>
		</div>
		{#snippet actions()}
			<button class="bordered small" onclick={onclose}>Close</button>
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
