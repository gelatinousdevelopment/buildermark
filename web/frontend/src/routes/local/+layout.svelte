<script lang="ts">
	import './local.css';
	import './markdown.css';
	import { resolve } from '$app/paths';
	import Icon from '$lib/Icon.svelte';

	let { children } = $props();
</script>

<header>
	<section>
		<div class="brand"><a href={resolve('/local')}>Strigidev</a></div>
	</section>
	<hr class="divider" />
	<section>
		<nav>
			<a href={resolve('/local')}>Dashboard</a>
			<a href={resolve('/local/projects')}>Projects</a>
		</nav>
	</section>
	<hr class="divider" />
	<section style:flex="1"></section>
	<hr class="divider" />
	<section>
		<nav class="right">
			<a href={resolve('/local')}><Icon name="gridLine2" width="18px" /></a>
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
		display: flex;
		align-items: stretch;
		border-bottom: 1px solid var(--color-divider);
		padding: 0;
	}

	header section {
		display: flex;
		align-items: center;
		padding: 0.8rem 1.5rem;
	}

	header hr.divider {
		background: var(--color-divider);
		border: 0;
		width: 1px;
		margin: 0;
	}

	header .brand {
		margin: 0;
		font-size: 1rem;
		font-weight: 600;
	}

	header .brand a {
		text-decoration: none;
		color: var(--color-text);
	}

	header nav {
		align-items: center;
		display: flex;
		gap: 1rem;
	}

	header nav.right {
		gap: 1.5rem;
	}

	header nav a {
		color: #555;
		text-decoration: none;
	}

	header nav a:hover {
		color: #000;
	}

	.dashboard-content {
		padding: 1.5rem;
		max-width: 1200px;
	}

	.status-dot {
		border-radius: 99px;
		width: 1rem;
		height: 1rem;
		background: var(--color-status-green);
	}
</style>
