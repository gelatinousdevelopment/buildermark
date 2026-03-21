<script lang="ts">
	import { relationshipCache } from './relationshipCache.svelte';
</script>

<div
	data-testid="source-conversation"
	class:source={relationshipCache.hoveredConversationId === 'conv-1'}
></div>
<div
	data-testid="highlighted-conversation"
	class:highlighted={relationshipCache.highlightedConversationIds.has('conv-1')}
></div>
<div data-testid="source-commit" class:source={relationshipCache.hoveredCommitHash === 'commit-1'}></div>
<div
	data-testid="highlighted-commit"
	class:highlighted={relationshipCache.highlightedCommitHashes.has('commit-1')}
></div>
