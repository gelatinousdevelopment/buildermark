<script lang="ts">
	import './dashboard.css';
	import { resolve } from '$app/paths';

	let { children } = $props();
</script>

<div class="dashboard-header">
	<h1><a href={resolve('/dashboard')}>zrate</a></h1>
	<nav>
		<a href={resolve('/dashboard')}>Dashboard</a>
		<a href={resolve('/dashboard/commits')}>Commits</a>
	</nav>
</div>

<div class="dashboard-content">
	{@render children()}
</div>
