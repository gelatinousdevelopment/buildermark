<script lang="ts">
	import type { Rating } from '$lib/types';
	import { createRating } from '$lib/api';
	import { buildResumeCommand } from '$lib/agents';
	import AgentTag from './AgentTag.svelte';
	import { resolve } from '$app/paths';

	interface Props {
		conversationId: string;
		agent?: string;
		projectPath?: string;
		onrating?: (rating: Rating) => void;
	}

	let { conversationId, agent, projectPath, onrating }: Props = $props();

	let ratingValue = $state(0);
	let note = $state('');
	let submitting = $state(false);
	let error: string | null = $state(null);
	let resumeCommandCopied = $state(false);
	let resumeCommandError: string | null = $state(null);

	let resumeCommand = $derived(
		agent && projectPath !== undefined
			? buildResumeCommand(agent, conversationId, projectPath)
			: null
	);

	async function submit() {
		if (ratingValue < 1) return;
		submitting = true;
		error = null;
		try {
			const newRating = await createRating(conversationId, ratingValue, note);
			onrating?.(newRating);
			ratingValue = 0;
			note = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to submit rating';
		} finally {
			submitting = false;
		}
	}

	async function copyResumeCommand() {
		if (!resumeCommand) return;
		resumeCommandError = null;
		try {
			await navigator.clipboard.writeText(resumeCommand);
			resumeCommandCopied = true;
			setTimeout(() => (resumeCommandCopied = false), 2000);
		} catch (e) {
			resumeCommandError = e instanceof Error ? e.message : 'Failed to copy the resume command';
		}
	}
</script>

<div class="add-rating-form">
	<div class="add-rating-header">
		<strong>Add rating</strong>
	</div>
	<div class="inline-stars">
		{#each [1, 2, 3, 4, 5] as star (star)}
			<button
				class="star-btn"
				class:active={star <= ratingValue}
				onclick={() => (ratingValue = star)}
			>
				{star <= ratingValue ? '★' : '☆'}
			</button>
		{/each}
	</div>
	<input type="text" class="inline-note" placeholder="Optional note..." bind:value={note} />
	<div class="inline-actions">
		<button class="bordered small" disabled={submitting || ratingValue < 1} onclick={submit}>
			{submitting ? 'Submitting...' : 'Submit'}
		</button>
	</div>
	{#if error}
		<p class="inline-error">{error}</p>
	{/if}
	{#if resumeCommand}
		<div class="resume-command-section">
			<div class="resume-command-header">
				<strong>Ask agent to rate in terminal</strong>
			</div>
			<p class="resume-command-copy">
				Copy this command and paste it into your terminal to resume this
				<AgentTag agent={agent ?? ''} subtle={true} />
				conversation and run <code>rate-buildermark</code>.
			</p>
			<div class="resume-command-block">
				<code>{resumeCommand}</code>
				<button class="bordered tiny" onclick={copyResumeCommand}>
					{resumeCommandCopied ? 'Copied!' : 'Copy'}
				</button>
			</div>
			<p class="resume-command-note">
				Requires the matching <a href={resolve('/plugins')}>Buildermark plugin or skill</a> to already
				be installed.
			</p>
			{#if resumeCommandError}
				<p class="inline-error">{resumeCommandError}</p>
			{/if}
		</div>
	{/if}
</div>

<style>
	.add-rating-form {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		white-space: normal;
	}

	.add-rating-header {
		font-size: 0.85rem;
	}

	.star-btn {
		background: none;
		border: none;
		cursor: pointer;
		font-size: 1.1rem;
		padding: 0;
		line-height: 1;
		color: var(--color-border-medium);
	}

	.star-btn:hover,
	.star-btn.active {
		color: var(--color-rating-border);
	}

	.inline-stars {
		display: flex;
		gap: 2px;
	}

	.inline-note {
		padding: 0.25rem 0.5rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		font-size: 0.85rem;
	}

	.inline-actions {
		display: flex;
		gap: 0.5rem;
	}

	.inline-error {
		color: var(--color-error);
		font-size: 0.85rem;
		margin: 0;
	}

	.resume-command-section {
		border-top: 1px solid var(--color-border-medium);
		margin-top: 0.25rem;
		padding-top: 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.resume-command-header {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
	}

	.resume-command-copy,
	.resume-command-note {
		color: var(--color-text-secondary);
		font-size: 0.85rem;
		line-height: 1.4;
		margin: 0;
	}

	.resume-command-block {
		align-items: flex-start;
		background: var(--color-background-surface);
		border: 1px solid var(--color-border-medium);
		border-radius: 6px;
		display: flex;
		flex-wrap: wrap;
		gap: 0.65rem;
		justify-content: space-between;
		padding: 0.65rem 0.75rem;
	}

	.resume-command-block code {
		color: var(--color-text);
		flex: 1;
		font-family: var(--font-family-monospace);
		font-size: 0.8rem;
		line-height: 1.5;
		overflow-wrap: anywhere;
		white-space: pre-wrap;
	}
</style>
