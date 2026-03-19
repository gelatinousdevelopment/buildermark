import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { getCommitConversationLinks } from '$lib/api';

const CACHE_TTL_MS = 30 * 60 * 1000; // 30 minutes

interface CacheEntry {
	commitToConversations: Record<string, string[]>;
	conversationToCommits: Record<string, string[]>;
	fetchedAt: number;
}

// The cache is keyed by projectId.
const cache = new SvelteMap<string, CacheEntry>();

// Reactive state for the currently hovered item's related IDs.
let _hoveredConversationId = $state<string | null>(null);
let _hoveredCommitHash = $state<string | null>(null);
let _highlightedConversationIds: SvelteSet<string> = new SvelteSet();
let _highlightedCommitHashes: SvelteSet<string> = new SvelteSet();

function getCacheEntry(projectId: string): CacheEntry | null {
	const entry = cache.get(projectId);
	if (!entry) return null;
	if (Date.now() - entry.fetchedAt > CACHE_TTL_MS) {
		cache.delete(projectId);
		return null;
	}
	return entry;
}

function ensureCacheEntry(projectId: string): CacheEntry {
	let entry = getCacheEntry(projectId);
	if (!entry) {
		entry = {
			commitToConversations: {},
			conversationToCommits: {},
			fetchedAt: Date.now()
		};
		cache.set(projectId, entry);
	}
	return entry;
}

function mergeCacheEntry(projectId: string, data: Partial<CacheEntry>): void {
	const existing = ensureCacheEntry(projectId);
	if (data.commitToConversations) {
		for (const [hash, convIds] of Object.entries(data.commitToConversations)) {
			existing.commitToConversations[hash] = convIds;
		}
	}
	if (data.conversationToCommits) {
		for (const [convId, hashes] of Object.entries(data.conversationToCommits)) {
			existing.conversationToCommits[convId] = hashes;
		}
	}
	if (data.fetchedAt) {
		existing.fetchedAt = data.fetchedAt;
	}
}

export const relationshipCache = {
	get hoveredConversationId() {
		return _hoveredConversationId;
	},
	get hoveredCommitHash() {
		return _hoveredCommitHash;
	},
	get highlightedConversationIds() {
		return _highlightedConversationIds;
	},
	get highlightedCommitHashes() {
		return _highlightedCommitHashes;
	},

	/**
	 * Load commit-conversation relationships for the given commit hashes and
	 * conversation IDs. Merges into the cache and does not block on completion.
	 * Only fetches items not already cached.
	 */
	async loadRelationships(
		projectId: string,
		commitHashes: string[],
		conversationIds: string[]
	): Promise<void> {
		if (commitHashes.length === 0) return;

		// Filter out commit hashes already in cache.
		const entry = getCacheEntry(projectId);
		let hashesToFetch = commitHashes;
		if (entry) {
			hashesToFetch = commitHashes.filter((h) => !(h in entry.commitToConversations));
		}
		if (hashesToFetch.length === 0) return;

		try {
			const links = await getCommitConversationLinks(projectId, hashesToFetch, conversationIds);
			mergeCacheEntry(projectId, {
				commitToConversations: links.commitToConversations,
				conversationToCommits: links.conversationToCommits,
				fetchedAt: Date.now()
			});
		} catch {
			// Silently fail — relationship data is supplementary
		}
	},

	/**
	 * Called when hovering over a conversation row. Looks up related commit
	 * hashes from the cache.
	 */
	hoverConversation(projectId: string, conversationId: string | null): void {
		_hoveredConversationId = conversationId;
		_hoveredCommitHash = null;
		if (!conversationId) {
			_highlightedCommitHashes = new SvelteSet();
			_highlightedConversationIds = new SvelteSet();
			return;
		}
		const entry = getCacheEntry(projectId);
		const hashes = entry?.conversationToCommits[conversationId] ?? [];
		_highlightedCommitHashes = new SvelteSet(hashes);
		_highlightedConversationIds = new SvelteSet();
	},

	/**
	 * Called when hovering over a commit row. Looks up related conversation
	 * IDs from the cache and updates the reactive highlight sets.
	 */
	hoverCommit(projectId: string, commitHash: string | null): void {
		_hoveredCommitHash = commitHash;
		_hoveredConversationId = null;
		if (!commitHash) {
			_highlightedConversationIds = new SvelteSet();
			_highlightedCommitHashes = new SvelteSet();
			return;
		}
		const entry = getCacheEntry(projectId);
		const convIds = entry?.commitToConversations[commitHash] ?? [];
		_highlightedConversationIds = new SvelteSet(convIds);
		_highlightedCommitHashes = new SvelteSet();
	},

	/**
	 * Clear all hover state.
	 */
	clearHover(): void {
		_hoveredConversationId = null;
		_hoveredCommitHash = null;
		_highlightedConversationIds = new SvelteSet();
		_highlightedCommitHashes = new SvelteSet();
	},

	clearProject(projectId: string): void {
		cache.delete(projectId);
		if (_hoveredConversationId || _hoveredCommitHash) {
			this.clearHover();
		}
	}
};
