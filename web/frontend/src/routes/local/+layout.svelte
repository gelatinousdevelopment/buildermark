<script lang="ts">
	import './local.css';
	import './markdown.css';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Icon from '$lib/Icon.svelte';
	import { navStore } from '$lib/stores/nav.svelte';

	let { children } = $props();

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
		}
	];

	function isTabSelected(segment: string): boolean {
		return page.url.pathname.includes('/' + segment);
	}
</script>

<div class="site">
	<header>
		<section>
			<div class="brand">
				<a href={resolve('/local')} class="item"
					><Icon name="wrench" width="13px" /> Strigidev Local</a
				>
			</div>
		</section>
		<hr class="divider" />
		<section>
			<nav class="breadcrumbs">
				<a
					href={resolve('/local/projects')}
					class="item"
					class:selected={page.route.id === '/local/projects'}>Projects</a
				>
				{#if projectId}
					<div class="chevron-right"><Icon name="chevronRight" width="15px" /></div>
					<a
						href={resolve('/local/projects/[project_id]', { project_id: projectId })}
						class="item"
						class:selected={page.route.id === '/local/projects/[project_id]'}
						>{navStore.projectName || projectId}</a
					>
					<div class="chevron-right"><Icon name="chevronRight" width="15px" /></div>
					{#each projectTabs as tab (tab.segment)}
						<a
							href={resolve(tab.route, { project_id: projectId })}
							class="item"
							class:selected={isTabSelected(tab.segment)}>{tab.label}</a
						>
					{/each}
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
			<nav class="right">
				<button class="item" title="Binary Status"><div class="status-dot running"></div></button>
			</nav>
		</section>
	</header>

	<div class="dashboard-content">
		{@render children()}
	</div>
</div>

<style>
	.site {
		display: flex;
		flex-direction: column;
		min-height: 100vh;
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

	header section button {
		background: none;
		border: none;
	}

	header section .item {
		align-content: center;
		box-sizing: border-box;
		height: 32px;
		padding: 0 1rem;
		white-space: nowrap;
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
		gap: 0.5rem;
		text-decoration: none;
	}

	header .brand a:hover {
		background: var(--accent-color);
		color: var(--accent-color-ultralight);
		position: relative;
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
		padding: 0 0.6rem;
		margin: 0 0rem;
		/*border-radius: 4px;*/
	}

	header nav.breadcrumbs a.selected {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
		padding: 0.4rem 0.8rem;
		margin: -0.4rem 0.2rem;
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

	.status-dot {
		border-radius: 99px;
		width: 1rem;
		height: 1rem;
		background: var(--color-status-green);
	}
</style>
