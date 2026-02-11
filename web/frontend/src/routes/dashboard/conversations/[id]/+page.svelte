<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getConversation } from '$lib/api';
	import { stars, fmtTime } from '$lib/utils';
	import type { ConversationDetail } from '$lib/types';

	let conversation: ConversationDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	onMount(async () => {
		try {
			const id = page.params.id;
			if (!id) throw new Error('Missing conversation ID');
			conversation = await getConversation(id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load conversation';
		} finally {
			loading = false;
		}
	});
</script>

<div class="breadcrumb">
	<a href={resolve('/dashboard')}>Dashboard</a> &rsaquo; Conversation
</div>

{#if loading}
	<p class="loading">Loading conversation...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if conversation}
	<h2>{conversation.title || conversation.id}</h2>
	<p>Agent: {conversation.agent} | Project: {conversation.projectId}</p>

	<div class="detail-section">
		<h3>Ratings</h3>
		{#if conversation.ratings.length === 0}
			<p>No ratings.</p>
		{:else}
			<table>
				<thead>
					<tr>
						<th>Rating</th>
						<th>Note</th>
						<th>Analysis</th>
						<th>Time</th>
					</tr>
				</thead>
				<tbody>
					{#each conversation.ratings as r (r.id)}
						<tr>
							<td>{stars(r.rating)}</td>
							<td>{r.note}</td>
							<td>{r.analysis}</td>
							<td>{fmtTime(r.createdAt)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>

	<div class="detail-section">
		<h3>Turns</h3>
		{#if conversation.turns.length === 0}
			<p>No turns.</p>
		{:else}
			{#each conversation.turns as turn (turn.id)}
				<div class="turn">
					<div class="turn-header">
						<strong>{turn.role}</strong> &middot; {fmtTime(turn.timestamp)}
					</div>
					<div class="turn-content">{turn.content}</div>
				</div>
			{/each}
		{/if}
	</div>
{/if}

<style>
	.detail-section {
		margin-bottom: 2rem;
	}

	.detail-section h3 {
		font-size: 1rem;
		margin-bottom: 0.5rem;
	}

	.turn {
		margin-bottom: 1rem;
		padding: 0.75rem;
		border: 1px solid #eee;
		border-radius: 4px;
	}

	.turn-header {
		font-size: 0.85rem;
		color: #666;
		margin-bottom: 0.25rem;
	}

	.turn-content {
		white-space: pre-wrap;
		font-size: 0.9rem;
	}
</style>
