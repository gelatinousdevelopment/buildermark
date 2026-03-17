<script lang="ts">
	import './local.css';
	import './markdown.css';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import Icon from '$lib/Icon.svelte';
	import ReadOnlyDialog from '$lib/components/ReadOnlyDialog.svelte';
	import { navStore } from '$lib/stores/nav.svelte';
	import { layoutStore } from '$lib/stores/layout.svelte';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import ServerStatusIndicator from '$lib/components/ServerStatusIndicator.svelte';
	import UpdatePill from '$lib/components/UpdatePill.svelte';
	import Popover from '$lib/components/Popover.svelte';
	import type { Project } from '$lib/types';
	import { env } from '$env/dynamic/public';

	const READ_ONLY = (env.PUBLIC_READ_ONLY ?? 'false') === 'true';

	let { children, data } = $props();

	let animatedEl: HTMLDivElement;
	let svgTemplate: Node;
	let projects: Project[] = $derived(
		(data.projects ?? []).toSorted((a: Project, b: Project) => a.label.localeCompare(b.label))
	);

	onMount(() => {
		websocketStore.connect();
		const svg = animatedEl?.querySelector('svg');
		if (svg) {
			svgTemplate = svg.cloneNode(true);
		}
	});

	function restartAnimation() {
		if (!svgTemplate || !animatedEl) return;
		const current = animatedEl.querySelector('svg');
		if (current) {
			current.replaceWith(svgTemplate.cloneNode(true));
		}
	}

	onDestroy(() => {
		websocketStore.disconnect();
	});

	let projectId = $derived(page.params.project_id);
	let showReadOnlyDialog = $state(READ_ONLY);
	let bigBrand = $derived(data.projects && data.projects.length == 0);
	const projectTabs = [
		{
			label: 'Conversations',
			segment: 'conversations',
			route: '/projects/[project_id]/conversations' as const
		},
		{
			label: 'Commits',
			segment: 'commits',
			route: '/projects/[project_id]/commits' as const
		},
		{
			label: 'Insights',
			segment: 'insights',
			route: '/projects/[project_id]/insights' as const
		},
		{
			label: 'Export',
			segment: 'export',
			route: '/projects/[project_id]/export' as const
		},
		{
			label: 'Settings',
			segment: 'settings',
			route: '/projects/[project_id]/settings' as const
		}
	];

	function isTabSelected(segment: string): boolean {
		return page.url.pathname.includes('/' + segment);
	}

	function handleKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
			e.preventDefault();
			/* eslint-disable svelte/no-navigation-without-resolve */
			goto(
				projectId
					? `${resolve('/search')}?project=${encodeURIComponent(projectId)}`
					: resolve('/search')
			);
			/* eslint-enable svelte/no-navigation-without-resolve */
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<svelte:head
	><title
		>{navStore.projectName
			? navStore.projectName + ' | Buildermark Local'
			: 'Buildermark Local'}</title
	></svelte:head
>

<div class="site" class:fixed-height={layoutStore.fixedHeight}>
	<header class:bigBrand>
		<section>
			<div class="brand">
				<a href={resolve('/projects')} class="item wordmark">
					<Icon name="buildermarkWordmark" width="176px" />
				</a>
				<Popover position="below" padding="1.3rem 1.5rem 1rem 1.3rem">
					{#snippet popover()}
						<a href={resolve('/')} class="logo-popover-text">Buildermark Local</a>
						{#if projects.length > 0}
							<ul class="logo-popover-projects">
								{#each projects as project (project.id)}
									<li>
										<a
											href={resolve('/projects/[project_id]', {
												project_id: project.id
											})}>{project.label || project.path}</a
										>
									</li>
								{/each}
							</ul>
						{/if}
					{/snippet}
					<a
						href={resolve('/projects')}
						class="item icon"
						onmouseenter={restartAnimation}
						data-sveltekit-preload-data="off"
					>
						<div class="static"><Icon name="buildermarkTall" width="29px" /></div>
						<div class="animated" bind:this={animatedEl}>
							<Icon name="buildermarkTallAnimated" width="29px" overflow="hidden" />
						</div>
					</a>
				</Popover>
			</div>
		</section>
		<hr class="divider" />
		<section>
			<nav class="breadcrumbs">
				{#if projectId}
					<!-- <div class="chevron-right"><Icon name="chevronRight" width="15px" /></div> -->
					<!-- <a
						href={resolve('/projects/[project_id]', { project_id: projectId })}
						class="item project"
						style:font-weight="400">gelatinousdevelopment</a
					> -->
					<!-- <div class="chevron-right" style:margin="0 0.5rem">/</div> -->
					<!-- <div class="chevron-right"><Icon name="chevronRight" width="15px" /></div> -->
					<a
						href={resolve('/projects/[project_id]', { project_id: projectId })}
						class="item project"
						class:selected={page.route.id === '/projects/[project_id]'}
						style:font-weight="bold">{navStore.projectName || ''}</a
					>
					<div class="chevron-right"><Icon name="chevronRight" width="15px" /></div>
					{#each projectTabs as tab (tab.segment)}
						<a
							href={resolve(tab.route, { project_id: projectId })}
							class="item"
							class:selected={isTabSelected(tab.segment)}>{tab.label}</a
						>
					{/each}
				{:else if !bigBrand}
					<a href={resolve('/projects')} class="item" class:selected={page.route.id === '/projects'}
						>Projects</a
					>
					<a
						href={resolve('/projects/import')}
						class="item"
						class:selected={page.route.id === '/projects/import'}>Import</a
					>
				{/if}
			</nav>
		</section>
		<section style:flex="1"></section>
		{#if !READ_ONLY && websocketStore.updateStatus.state !== 'none'}
			<section>
				<nav class="right pill">
					<UpdatePill />
				</nav>
			</section>
		{/if}
		{#if READ_ONLY}
			<section>
				<nav class="right pill">
					<button class="read-only-pill" onclick={() => (showReadOnlyDialog = true)}>
						Read-only mode
					</button>
				</nav>
			</section>
		{/if}
		{#if !bigBrand}
			<hr class="divider" />
			<section>
				<nav class="right">
					<!-- eslint-disable svelte/no-navigation-without-resolve -->
					<a
						href={projectId
							? `${resolve('/search')}?project=${encodeURIComponent(projectId)}`
							: resolve('/search')}
						class="item"
						class:selected={page.route.id === '/search'}
						title="Search"><Icon name="search" width="20px" /></a
					>
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
				</nav>
			</section>
		{/if}
		<hr class="divider" />
		<section>
			<nav class="right">
				<a
					href={resolve('/plugins')}
					class="item"
					class:selected={page.route.id === '/plugins'}
					title="Plugins"><Icon name="puzzlePiece" width="18px" /></a
				>
			</nav>
		</section>
		<hr class="divider" />
		<section>
			<nav class="right">
				<a
					href={resolve('/settings')}
					class="item"
					class:selected={page.route.id === '/settings'}
					title="Buildermark Local Settings"><Icon name="gear" width="18px" /></a
				>
			</nav>
		</section>
		<hr class="divider" />
		<section>
			<nav class="right">
				{#if READ_ONLY}
					<a
						href="https://github.com/gelatinousdevelopment/buildermark"
						class="item"
						title="Buildermark on GitHub"
						target="_blank"><Icon name="github" width="15px" /></a
					>
				{:else}
					<ServerStatusIndicator />
				{/if}
			</nav>
		</section>
	</header>

	<div class="dashboard-content">
		{@render children()}
	</div>

	<ReadOnlyDialog open={showReadOnlyDialog} onclose={() => (showReadOnlyDialog = false)} />

	<footer>
		<div class="content">
			<a href="https://buildermark.dev" target="_blank">Buildermark</a> is a trademark of
			<a href="https://geldev.com" target="_blank">Gelatinous Development Studio</a>
			&bull; Buildermark Local is
			<a href="https://github.com/gelatinousdevelopment/buildermark" target="_blank">open source</a>
		</div>
	</footer>
</div>

<style>
	.site {
		display: flex;
		flex-direction: column;
		min-height: 100vh;
	}

	.site.fixed-height {
		height: 100vh;
	}

	.site.fixed-height .dashboard-content {
		min-height: 0;
	}

	header {
		align-items: stretch;
		border-bottom: 0.5px solid var(--color-divider);
		border-top: 0.5px solid var(--color-divider);
		display: flex;
		font-size: 1rem;
		padding: 0;
		background: var(--color-background-content);
	}

	header section {
		align-items: center;
		display: flex;
	}

	header section .item {
		align-content: center;
		box-sizing: border-box;
		font-size: 1.1rem;
		font-weight: 400;
		height: 40px;
		padding: 0 1rem;
		white-space: nowrap;
	}

	header section .item.project {
		font-size: 1.1rem;
		font-weight: 600;
	}

	header section .item:hover {
		background: var(--accent-color-ultralight);
	}

	header hr.divider {
		background: var(--color-divider);
		border: 0;
		margin: 0;
		min-width: 0.5px;
		width: 0.5px;

		/*display: none;*/
	}

	header .brand {
		margin: 0;
		font-weight: 600;
	}

	header .brand a {
		align-items: center;
		color: var(--color-text-secondary);
		display: flex;
		flex-direction: row;
		font-size: 1rem;
		gap: 0.6rem;
		text-decoration: none;
		padding: 0 1.5rem;
	}

	header .brand a.wordmark {
		display: none;
	}
	header .brand a.icon {
		display: flex;
		align-items: flex-end;
	}
	header.bigBrand .brand a.wordmark {
		display: flex;
	}
	header.bigBrand .brand a.icon {
		display: none;
	}

	header .brand a.icon .static {
		display: block;
	}
	header .brand a.icon .animated {
		display: none;
	}

	header .brand a:hover {
		background: var(--accent-color);
		color: var(--accent-color-ultralight);
		position: relative;
	}

	header .brand .logo-popover-text {
		font-size: 1.1rem;
		font-weight: bold;
		text-align: left;
		display: block;
		padding: 0;
	}

	header .brand .logo-popover-text:hover {
		background: none;
		color: var(--accent-color);
		text-decoration: underline;
	}

	.logo-popover-projects {
		list-style: none;
		margin: 0.5rem 0 0 0;
		padding: 0;
		gap: 0;
	}

	.logo-popover-projects li {
		margin: 0;
		padding: 0.2rem 0;
	}

	.logo-popover-projects li a {
		color: var(--color-link);
		display: block;
		font-size: 1.1rem;
		font-weight: normal;
		padding: 0.2rem 0;
		text-decoration: none;
	}

	.logo-popover-projects li a:hover {
		background: none;
		color: var(--color-link-hover);
		text-decoration: underline;
	}

	header .brand a.icon:hover .static {
		display: none;
	}
	header .brand a.icon:hover .animated {
		display: block;
	}

	header .brand a :global(.icon),
	header .brand a .static,
	header .brand a .animated {
		align-items: flex-end;
	}

	header nav {
		align-items: center;
		display: flex;
		gap: 1.5rem;
	}

	header nav.breadcrumbs {
		gap: 0rem;
		padding-left: 0.5rem;
	}

	header nav.breadcrumbs .chevron-right {
		opacity: 0.4;
	}

	header nav.breadcrumbs a {
		padding: 0 0.8rem;
		margin: 0 0rem;
	}

	header nav.breadcrumbs a:hover {
		text-decoration: none;
	}

	header nav.breadcrumbs a.selected {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
		padding: 0.4rem 0.8rem calc(0.4rem - 2px) 0.8rem;
		border-bottom: 3px solid var(--accent-color);
		margin-bottom: -1px;
	}

	header nav.right {
		gap: 1.5rem;
		min-width: 50px;
	}

	header nav.right.pill {
		padding-right: 0.7rem;
	}

	header nav.right .read-only-pill {
		background: #727272;
		border-radius: 999px;
		border: 0;
		color: #eee;
		cursor: pointer;
		font-size: 0.8rem;
		font-weight: 600;
		height: 20px;
		padding: 0 0.8rem;
		text-transform: uppercase;
	}

	header nav.right .read-only-pill:hover {
		background: var(--accent-color);
		color: var(--accent-color-ultralight);
	}

	header nav a {
		color: var(--color-text-secondary);
		text-decoration: none;
	}

	header nav a:hover {
		color: var(--accent-color);
		text-decoration: underline;
	}

	header nav.right a {
		width: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	header nav.right a.selected {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
	}

	.dashboard-content {
		align-items: stretch;
		background: var(--color-background-page);
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		flex: 1;
		margin: 0 auto;
		max-width: 100vw;
		width: 100vw;
	}

	footer {
		background: var(--color-background-page);
		padding: 1rem;
	}

	.site.fixed-height footer {
		display: none;
	}

	footer .content {
		font-size: 0.9rem;
		opacity: 0.6;
		text-align: center;
	}

	footer:hover .content {
		opacity: 1;
	}

	footer a {
		text-decoration: none;
	}

	footer a:hover {
		text-decoration: underline;
	}
</style>
