<script lang="ts">
	interface Props {
		added: number;
		removed: number;
		files?: number;
		showFiles?: boolean;
		compact?: boolean;
	}

	let { added, removed, files = 0, showFiles = false, compact = false }: Props = $props();
</script>

<span class="diff-count" class:compact>
	{#if showFiles && files !== undefined}
		<span class="files-count">{files} {files === 1 ? 'file' : 'files'}</span>
	{/if}
	<span class="badge added">+{added}</span>
	<span class="badge removed">-{removed}</span>
</span>

<style>
	.diff-count {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		font-variant-numeric: tabular-nums;
		font-size: inherit;
	}

	.diff-count.compact {
		font-size: 0.8rem;
		margin: -3px 0;
	}

	.files-count {
		color: var(--color-text-secondary);
	}

	.badge {
		display: inline-block;
		padding: 0.05em 0.45em;
		border-radius: 4px;
		font-weight: 500;
		line-height: 1.4;
	}

	.badge.added {
		color: var(--color-diff-added-text);
		background: var(--color-diff-added-bg);
		border: 1px solid var(--color-diff-added-border);
	}

	.badge.removed {
		color: var(--color-diff-removed-text);
		background: var(--color-diff-removed-bg);
		border: 1px solid var(--color-diff-removed-border);
	}
</style>
