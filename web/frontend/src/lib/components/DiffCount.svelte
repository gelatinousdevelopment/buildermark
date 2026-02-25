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
		color: #5f6b7a;
	}

	.badge {
		display: inline-block;
		padding: 0.05em 0.45em;
		border-radius: 4px;
		font-weight: 500;
		line-height: 1.4;
	}

	.badge.added {
		color: #1a7f37;
		background: #dafbe1;
		border: 1px solid #aceebb;
	}

	.badge.removed {
		color: #cf222e;
		background: #ffebe9;
		border: 1px solid #ffcecb;
	}
</style>
