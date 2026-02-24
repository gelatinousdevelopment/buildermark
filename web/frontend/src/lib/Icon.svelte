<script lang="ts">
	import gridLine2Svg from '$lib/icons/gridLine2.svg?raw';
	import githubSvg from '$lib/icons/github.svg?raw';
	import chevronRightSvg from '$lib/icons/chevronRight.svg?raw';
	import wrenchSvg from '$lib/icons/wrench.svg?raw';
	import gearSvg from '$lib/icons/gear.svg?raw';
	import branchSvg from '$lib/icons/branch.svg?raw';
	import externalLinkSvg from '$lib/icons/externalLink.svg?raw';
	import plusSvg from '$lib/icons/plus.svg?raw';

	export let name: Icon;
	export let width: string = '100%';
	export let height: string = '100%';
	export let fillColor: string | undefined = undefined;

	const icons: { [key in Icon]: string } = {
		gridLine2: gridLine2Svg,
		github: githubSvg,
		chevronRight: chevronRightSvg,
		wrench: wrenchSvg,
		gear: gearSvg,
		branch: branchSvg,
		externalLink: externalLinkSvg,
		plus: plusSvg
	};
</script>

{#if name && icons[name]}
	<div class="icon" style:width style:height style:--fill-color={fillColor}>
		<!-- eslint-disable-next-line svelte/no-at-html-tags -->
		{@html icons[name]}
	</div>
{/if}

<style>
	.icon {
		--fill-color: var(--c-fg-deselected);
		align-items: center;
		display: flex;
		font-size: 0;
		justify-content: center;
		margin: 0;
		padding: 0;
		white-space: collapse;
	}
	.icon :global(svg) {
		height: auto !important;
		overflow: visible;
		width: 100% !important;
	}
	.icon :global(svg *) {
		color: var(--fill-color);
	}
</style>
