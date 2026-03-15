<script lang="ts">
	import gridLine2Svg from '$lib/icons/gridLine2.svg?raw';
	import githubSvg from '$lib/icons/github.svg?raw';
	import chevronRightSvg from '$lib/icons/chevronRight.svg?raw';
	import wrenchSvg from '$lib/icons/wrench.svg?raw';
	import gearSvg from '$lib/icons/gear.svg?raw';
	import branchSvg from '$lib/icons/branch.svg?raw';
	import externalLinkSvg from '$lib/icons/externalLink.svg?raw';
	import plusSvg from '$lib/icons/plus.svg?raw';
	import documentSvg from '$lib/icons/document.svg?raw';
	import userSvg from '$lib/icons/user.svg?raw';
	import searchSvg from '$lib/icons/search.svg?raw';
	import checkSvg from '$lib/icons/check.svg?raw';
	import xSvg from '$lib/icons/x.svg?raw';
	import puzzlePieceSvg from '$lib/icons/puzzlePiece.svg?raw';
	import infoSvg from '$lib/icons/info.svg?raw';
	import buildermarkSvg from '$lib/icons/buildermark.svg?raw';
	import buildermarkWordmarkSvg from '$lib/icons/buildermarkWordmark.svg?raw';
	import buildermarkTallSvg from '$lib/icons/buildermarkTall.svg?raw';
	import buildermarkTallAnimatedSvg from '$lib/icons/buildermarkTallAnimated.svg?raw';
	import cloudSvg from '$lib/icons/cloud.svg?raw';
	import commitSvg from '$lib/icons/commit.svg?raw';
	import starSvg from '$lib/icons/star.svg?raw';
	import deleteSvg from '$lib/icons/delete.svg?raw';

	export let name: Icon;
	export let width: string = '100%';
	export let height: string = '100%';
	export let fillColor: string | undefined = undefined;
	export let overflow: string = 'visible';

	const icons: { [key in Icon]: string } = {
		gridLine2: gridLine2Svg,
		github: githubSvg,
		chevronRight: chevronRightSvg,
		wrench: wrenchSvg,
		gear: gearSvg,
		branch: branchSvg,
		externalLink: externalLinkSvg,
		plus: plusSvg,
		document: documentSvg,
		user: userSvg,
		search: searchSvg,
		check: checkSvg,
		x: xSvg,
		puzzlePiece: puzzlePieceSvg,
		info: infoSvg,
		buildermark: buildermarkSvg,
		buildermarkWordmark: buildermarkWordmarkSvg,
		buildermarkTall: buildermarkTallSvg,
		buildermarkTallAnimated: buildermarkTallAnimatedSvg,
		cloud: cloudSvg,
		commit: commitSvg,
		star: starSvg,
		delete: deleteSvg
	};
</script>

{#if name && icons[name]}
	<div class="icon" style:width style:height style:--fill-color={fillColor} style:overflow>
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
		width: 100% !important;
	}
	.icon :global(svg *) {
		color: var(--fill-color);
	}
</style>
