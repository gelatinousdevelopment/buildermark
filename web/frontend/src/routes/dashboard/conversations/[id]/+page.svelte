<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getConversation } from '$lib/api';
	import { stars, fmtTime } from '$lib/utils';
	import { SvelteSet } from 'svelte/reactivity';
	import type { ConversationDetail, TurnRead, Rating } from '$lib/types';

	type TimelineItem =
		| { kind: 'turn'; turn: TurnRead; time: number }
		| { kind: 'rating'; rating: Rating; time: number };

	let conversation: ConversationDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	let timeline: TimelineItem[] = $derived.by(() => {
		if (!conversation) return [];

		const matchedTurnIds = new SvelteSet<string>();
		const matchedRatingIds = new SvelteSet<string>();
		const items: TimelineItem[] = [];

		// Match each rating to the closest /zrate user turn within 120s
		for (const rating of conversation.ratings) {
			const ratingTime = new Date(rating.createdAt).getTime();
			let bestTurn: TurnRead | null = null;
			let bestDelta = Infinity;

			for (const turn of conversation.turns) {
				if (turn.role !== 'user' || !turn.content.startsWith('/zrate')) continue;
				if (matchedTurnIds.has(turn.id)) continue;
				const delta = Math.abs(turn.timestamp - ratingTime);
				if (delta < bestDelta && delta <= 120_000) {
					bestDelta = delta;
					bestTurn = turn;
				}
			}

			if (bestTurn) {
				matchedTurnIds.add(bestTurn.id);
				matchedRatingIds.add(rating.id);
				items.push({ kind: 'rating', rating, time: bestTurn.timestamp });
			}
		}

		// Add unmatched turns
		for (const turn of conversation.turns) {
			if (!matchedTurnIds.has(turn.id)) {
				items.push({ kind: 'turn', turn, time: turn.timestamp });
			}
		}

		// Add unmatched ratings
		for (const rating of conversation.ratings) {
			if (!matchedRatingIds.has(rating.id)) {
				items.push({ kind: 'rating', rating, time: new Date(rating.createdAt).getTime() });
			}
		}

		items.sort((a, b) => a.time - b.time);
		return items;
	});

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

	{#if timeline.length === 0}
		<p>No turns or ratings.</p>
	{:else}
		{#each timeline as item (item.kind === 'turn' ? item.turn.id : item.rating.id)}
			{#if item.kind === 'turn'}
				<div class="turn">
					<div class="turn-header">
						<strong>{item.turn.role}</strong> &middot; {fmtTime(item.turn.timestamp)}
					</div>
					<div class="turn-content">{item.turn.content}</div>
				</div>
			{:else}
				<div class="rating-card">
					<div class="turn-header">
						<strong>Rating</strong> &middot; {fmtTime(item.rating.createdAt)}
					</div>
					<div class="rating-stars">{stars(item.rating.rating)}</div>
					{#if item.rating.note}
						<div class="rating-field"><strong>Note:</strong> {item.rating.note}</div>
					{/if}
					{#if item.rating.analysis}
						<div class="rating-field"><strong>Analysis:</strong> {item.rating.analysis}</div>
					{/if}
				</div>
			{/if}
		{/each}
	{/if}
{/if}

<style>
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

	.rating-card {
		margin-bottom: 1rem;
		padding: 0.75rem;
		border: 2px solid #f0c040;
		border-radius: 4px;
		background: #fffbea;
	}

	.rating-stars {
		font-size: 1.1rem;
		margin-bottom: 0.25rem;
	}

	.rating-field {
		font-size: 0.9rem;
		margin-top: 0.25rem;
	}
</style>
