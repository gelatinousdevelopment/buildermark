<script lang="ts">
	import './local.css';
	import './markdown.css';
	import { resolve } from '$app/paths';
	import Icon from '$lib/Icon.svelte';

	let { children } = $props();
</script>

<header>
	<section>
		<div class="brand">
			<a href={resolve('/local')}><Icon name="wrench" width="13px" /> Strigidev Local</a>
		</div>
	</section>
	<hr class="divider" />
	<section>
		<nav class="breadcrumbs">
			<!-- <a href={resolve('/local')}>Dashboard</a> -->
			<a href={resolve('/local/projects')}>Projects</a>
			<div class="chevron-right"><Icon name="chevronRight" width="15px" /></div>
			<!-- <span class="chevron"></span> -->
			<a href={resolve('/local/projects/oiSyQa5jX3iGHhcaykB-5')}>zrate</a>
			<div class="chevron-right"><Icon name="chevronRight" width="15px" /></div>
			<!-- <span class="chevron"></span> -->
			<a href={resolve('/local/projects/oiSyQa5jX3iGHhcaykB-5/conversations')} class="selected"
				>Conversations</a
			>
			<a href={resolve('/local/projects/oiSyQa5jX3iGHhcaykB-5/commits')}>Commits</a>
			<!-- <span class="chevron"></span>
			<a href={resolve('/local/projects/oiSyQa5jX3iGHhcaykB-5/conversations')}
				>Feb 16, 2026 5:29:46 PM</a
			> -->
		</nav>
	</section>
	<!-- <hr class="divider" /> -->
	<section style:flex="1"></section>
	<hr class="divider" />
	<section>
		<nav class="right">
			<a href="https://github.com/gelatinousdevelopment"><Icon name="github" width="15px" /></a>
		</nav>
	</section>
	<hr class="divider" />
	<section>
		<nav class="right">
			<a href={resolve('/local')}><Icon name="gridLine2" width="15px" /></a>
		</nav>
	</section>
	<hr class="divider" />
	<section>
		<nav class="right">
			<div class="status-dot running"></div>
		</nav>
	</section>
</header>

<div class="dashboard-content">
	{@render children()}
</div>

<style>
	/* Layout */

	header {
		align-items: stretch;
		border-bottom: 0.5px solid var(--color-divider);
		border-top: 0.5px solid var(--color-divider);
		font-size: 1rem;
		display: flex;
		padding: 0;
	}

	header section {
		display: flex;
		align-items: center;
		padding: 0.6rem 1rem;
	}

	header hr.divider {
		background: var(--color-divider);
		border: 0;
		width: 0.5px;
		margin: 0;
	}

	header .brand {
		margin: 0;
		font-weight: 600;
	}

	header .brand a {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		text-decoration: none;
		color: var(--color-text);
		padding: 0.6rem 1rem;
		margin: -0.6rem -1rem;
	}

	header .brand a:hover {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
	}

	header nav {
		align-items: center;
		display: flex;
		gap: 1.5rem;
	}

	header nav.breadcrumbs {
		gap: 0rem;
	}

	header nav.breadcrumbs .chevron-right {
		opacity: 0.4;
	}

	header nav.breadcrumbs a {
		padding: 0.4rem 0.6rem;
		margin: -0.4rem 0rem;
		border-radius: 4px;
	}

	header nav.breadcrumbs a.selected {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
		padding: 0.4rem 0.8rem;
		margin: -0.4rem 0.2rem;
	}

	/*header nav.breadcrumbs .chevron {
		border-left: 1px solid var(--color-divider);
		height: 1.4em;
		transform: rotate(20deg);
	}*/

	header nav.right {
		gap: 1.5rem;
	}

	header nav a {
		color: #555;
		text-decoration: none;
	}

	header nav a:hover {
		color: #000;
		text-decoration: underline;
	}

	.dashboard-content {
		padding: 1rem;
	}

	.status-dot {
		border-radius: 99px;
		width: 1rem;
		height: 1rem;
		background: var(--color-status-green);
	}
</style>
