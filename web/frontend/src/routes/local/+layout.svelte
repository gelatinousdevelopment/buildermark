<script lang="ts">
	import './local.css';
	import './markdown.css';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import Icon from '$lib/Icon.svelte';
	import { navStore } from '$lib/stores/nav.svelte';
	import { layoutStore } from '$lib/stores/layout.svelte';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import ServerStatusIndicator from '$lib/components/ServerStatusIndicator.svelte';

	let { children } = $props();

	onMount(() => {
		websocketStore.connect();
	});

	onDestroy(() => {
		websocketStore.disconnect();
	});

	let projectId = $derived(page.params.project_id);

	const projectTabs = [
		{
			label: 'Conversations',
			segment: 'conversations',
			route: '/local/projects/[project_id]/conversations' as const
		},
		{
			label: 'Commits',
			segment: 'commits',
			route: '/local/projects/[project_id]/commits' as const
		},
		{
			label: 'Insights',
			segment: 'insights',
			route: '/local/projects/[project_id]/insights' as const
		},
		{
			label: 'Settings',
			segment: 'settings',
			route: '/local/projects/[project_id]/settings' as const
		}
	];

	function isTabSelected(segment: string): boolean {
		return page.url.pathname.includes('/' + segment);
	}
</script>

<div class="site" class:fixed-height={layoutStore.fixedHeight}>
	<header>
		<section>
			<div class="brand">
				<a href={resolve('/local/projects')} class="item"
					><Icon name="wrench" width="22px" />
					<div class="text">
						<div class="title">Buildermark</div>
						<div class="subtitle">Local</div>
					</div></a
				>
			</div>
		</section>
		<hr class="divider" />
		<section>
			<nav class="breadcrumbs">
				{#if projectId}
					<!-- <div class="chevron-right"><Icon name="chevronRight" width="15px" /></div> -->
					<!-- <a
						href={resolve('/local/projects/[project_id]', { project_id: projectId })}
						class="item project"
						style:font-weight="400">gelatinousdevelopment</a
					> -->
					<!-- <div class="chevron-right" style:margin="0 0.5rem">/</div> -->
					<!-- <div class="chevron-right"><Icon name="chevronRight" width="15px" /></div> -->
					<a
						href={resolve('/local/projects/[project_id]', { project_id: projectId })}
						class="item project"
						class:selected={page.route.id === '/local/projects/[project_id]'}
						style:font-weight="bold">{navStore.projectName || projectId}</a
					>
					<div class="chevron-right"><Icon name="chevronRight" width="15px" /></div>
					{#each projectTabs as tab (tab.segment)}
						<a
							href={resolve(tab.route, { project_id: projectId })}
							class="item"
							class:selected={isTabSelected(tab.segment)}>{tab.label}</a
						>
					{/each}
				{:else}
					<a
						href={resolve('/local/projects')}
						class="item"
						class:selected={page.route.id === '/local/projects'}>Projects</a
					>
					<!-- <div class="chevron-right"><Icon name="chevronRight" width="15px" /></div> -->
					<a
						href={resolve('/local/projects/add')}
						class="item"
						class:selected={page.route.id === '/local/projects/add'}>Import</a
					>
				{/if}
			</nav>
		</section>
		<!-- <hr class="divider" /> -->
		<section style:flex="1"></section>
		<hr class="divider" />
		<section>
			<nav class="right">
				<a href="https://github.com/gelatinousdevelopment" class="item"
					><Icon name="github" width="15px" /></a
				>
			</nav>
		</section>
		<hr class="divider" />
		<section>
			<nav class="right">
				<a href={resolve('/local/settings')} class="item"><Icon name="gear" width="17px" /></a>
			</nav>
		</section>
		<hr class="divider" />
		<section>
			<ServerStatusIndicator />
		</section>
	</header>

	<div class="dashboard-content">
		{@render children()}
	</div>

	<footer>
		<div class="content">
			<a href="https://buildermark.dev" target="_blank">Buildermark</a> brand &copy; 2026
			<a href="https://geldev.com" target="_blank">Gelatinous Development Studio</a>
			&bull; Buildermark Local is
			<a href="https://github.com/gelatinousdevelopment/buildermark" target="_blank">open source</a>
			&bull; Support with a
			<a href="https://buildermark.dev" target="_blank">team server license</a>
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
		padding: 0 1.3rem;
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
	}

	header .brand {
		margin: 0;
		font-weight: 600;
	}

	header .brand a {
		align-items: center;
		color: var(--color-text);
		display: flex;
		flex-direction: row;
		font-size: 1rem;
		gap: 0.6rem;
		text-decoration: none;
	}

	header .brand a:hover {
		background: var(--accent-color);
		color: var(--accent-color-ultralight);
		position: relative;
	}

	header .brand .text {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
		justify-content: center;
	}

	header .brand .text .title {
		font-size: 1rem;
		font-weight: 600;
	}

	header .brand .text .subtitle {
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
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
		/*border-radius: 4px;*/
	}

	header nav.breadcrumbs a:hover {
		text-decoration: none;
	}

	header nav.breadcrumbs a.selected {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
		padding: 0.4rem 0.8rem calc(0.4rem - 2px) 0.8rem;
		/*margin: -0.4rem 0.2rem;*/
		border-bottom: 3px solid var(--accent-color);
		margin-bottom: -1px;
	}

	header nav.right {
		gap: 1.5rem;
	}

	header nav a {
		color: #555;
		text-decoration: none;
	}

	header nav a:hover {
		color: var(--accent-color);
		text-decoration: underline;
	}

	.dashboard-content {
		background: var(--color-background-page);
		margin: 0 auto;
		/*max-width: 100rem;*/
		/*padding: 1rem;*/
		flex: 1;

		display: flex;
		flex-direction: column;
		align-items: stretch;

		width: 100vw;
		max-width: 100vw;
		box-sizing: border-box;
	}

	footer {
		background: var(--color-background-page);
		padding: 1rem;
	}

	footer .content {
		font-size: 0.9rem;
		opacity: 0.5;
		text-align: center;
	}

	footer:hover .content {
		opacity: 1;
	}
</style>
