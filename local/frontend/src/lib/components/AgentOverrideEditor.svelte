<script lang="ts">
	interface Props {
		agents: string[];
		initialValues: Record<string, number>;
		onsave: (values: Record<string, number>) => void;
		oncancel: () => void;
		onclear: () => void;
	}

	let { agents, initialValues, onsave, oncancel, onclear }: Props = $props();

	let values: Record<string, number> = $state({});

	$effect(() => {
		const v: Record<string, number> = {};
		for (const agent of agents) {
			v[agent] = initialValues[agent] ?? 0;
		}
		values = v;
	});

	let agentSum = $derived.by(() => {
		let sum = 0;
		for (const agent of agents) {
			sum += values[agent] ?? 0;
		}
		return sum;
	});

	let manualPercent = $derived(Math.max(0, 100 - agentSum));
	let isValid = $derived(agentSum >= 0 && agentSum <= 100);

	function handleSave() {
		if (!isValid) return;
		const result: Record<string, number> = {};
		for (const agent of agents) {
			const v = values[agent] ?? 0;
			if (v > 0) result[agent] = v;
		}
		onsave(result);
	}
</script>

<div class="override-editor">
	{#each agents as agent (agent)}
		<div class="agent-row">
			<label for="override-{agent}">{agent}</label>
			<input
				id="override-{agent}"
				type="number"
				min="0"
				max="100"
				bind:value={values[agent]}
				class="override-input"
			/>
			<span class="pct">%</span>
		</div>
	{/each}
	<div class="agent-row manual-row">
		<label for="override-manual">manual</label>
		<input
			id="override-manual"
			type="number"
			value={manualPercent}
			class="override-input"
			disabled
		/>
		<span class="pct">%</span>
	</div>
	{#if !isValid}
		<p class="validation-error">Must sum to 0-100</p>
	{/if}
	<div class="editor-actions">
		<button class="btn-override-action" onclick={handleSave} disabled={!isValid}>Save</button>
		<button class="btn-override-action" onclick={oncancel}>Cancel</button>
		<button class="btn-override-action btn-clear" onclick={onclear}>Clear Override</button>
	</div>
</div>

<style>
	.override-editor {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding: 0.5rem 0;
	}

	.agent-row {
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}

	.agent-row label {
		min-width: 7rem;
		font-size: 0.85rem;
		color: var(--color-text-secondary);
	}

	.override-input {
		width: 4rem;
		padding: 0.2rem 0.4rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		font-size: 0.85rem;
	}

	.override-input:disabled {
		opacity: 0.6;
	}

	.pct {
		font-size: 0.85rem;
		color: var(--color-text-tertiary);
	}

	.manual-row label {
		color: var(--color-text-tertiary);
	}

	.validation-error {
		color: var(--color-status-red);
		font-size: 0.8rem;
		margin: 0.25rem 0 0 0;
	}

	.editor-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.35rem;
	}

	.btn-override-action {
		padding: 0.15rem 0.5rem;
		font-size: 0.8rem;
		border: 1px solid var(--color-border-input);
		border-radius: 4px;
		background: var(--color-button-bg);
		cursor: pointer;
		color: var(--color-text-secondary);
	}

	.btn-override-action:hover {
		border-color: var(--accent-color);
		color: var(--accent-color);
	}

	.btn-override-action:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-clear {
		margin-left: auto;
		color: var(--color-status-red);
		border-color: var(--color-status-red);
	}

	.btn-clear:hover {
		background: var(--color-status-red);
		color: white;
	}
</style>
