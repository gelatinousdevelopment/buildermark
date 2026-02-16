<script lang="ts">
	import './local.css';
	import './markdown.css';
	import { resolve } from '$app/paths';

	let { children } = $props();
</script>

<div class="dashboard-header">
	<h1><a href={resolve('/local')}>zrate</a></h1>
	<nav>
		<a href={resolve('/local')}>Dashboard</a>
		<a href={resolve('/local/projects')}>Projects</a>
	</nav>
</div>

<div class="dashboard-content">
	{@render children()}
</div>
