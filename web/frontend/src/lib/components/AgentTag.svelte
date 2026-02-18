<script lang="ts">
	interface Props {
		agent: string;
	}

	let { agent }: Props = $props();

	function normalizeAgentName(name: string): string {
		return name
			.trim()
			.replace(/[^a-zA-Z0-9]/g, '-')
			.toLowerCase();
	}

	let normalized = $derived(normalizeAgentName(agent));
</script>

<span
	class="agent-tag"
	style={`--agent-tag-bg: var(--agent-color-${normalized}, #777); --agent-tag-fg: var(--agent-foreground-color-${normalized}, #fff);`}
>
	{agent}
</span>

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
</style>
