<script lang="ts">
	interface Props {
		agent: string;
		subtle?: boolean;
	}

	let { agent, subtle = false }: Props = $props();

	function normalizeAgentName(name: string): string {
		return name
			.trim()
			.replace(/[^a-zA-Z0-9]/g, '-')
			.toLowerCase();
	}

	let normalized = $derived(normalizeAgentName(agent));
</script>

{#if subtle}
	<span class="agent-subtle" style={`--agent-tag-bg: var(--agent-color-${normalized}, #777);`}>
		<span class="agent-subtle-dot" aria-hidden="true"></span>
		<span>{agent}</span>
	</span>
{:else}
	<span
		class="agent-tag"
		style={`--agent-tag-bg: var(--agent-color-${normalized}, #777); --agent-tag-fg: var(--agent-foreground-color-${normalized}, #fff);`}
	>
		{agent}
	</span>
{/if}

<style>
	.agent-tag {
		display: inline-flex;
		align-items: center;
		padding: 0.16rem 0.6rem;
		border-radius: 999px;
		background: var(--agent-tag-bg);
		color: var(--agent-tag-fg);
		font-size: 0.75rem;
		font-weight: 600;
		line-height: 1.2;
		margin: -0.1rem 0;
		text-transform: uppercase;
		white-space: nowrap;
	}

	.agent-subtle {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		color: inherit;
		font: inherit;
		margin: 0 0.2rem;
		white-space: nowrap;
	}

	.agent-subtle-dot {
		width: 0.5rem;
		height: 0.5rem;
		border-radius: 999px;
		background: var(--agent-tag-bg);
		flex-shrink: 0;
	}
</style>
