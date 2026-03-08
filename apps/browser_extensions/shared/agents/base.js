/**
 * Base class for agent conversation importers.
 * Each agent subclass defines URL patterns, selectors, and extraction logic.
 */
class BaseAgent {
  /** Human-readable name for this agent. */
  get name() {
    throw new Error('Subclass must implement name');
  }

  /** Agent identifier sent to the API (e.g., 'claude_cloud', 'codex_cloud'). */
  get agentId() {
    throw new Error('Subclass must implement agentId');
  }

  /**
   * URL pattern to match. Returns a RegExp.
   * The pattern should have a capture group for the conversation ID.
   */
  get urlPattern() {
    throw new Error('Subclass must implement urlPattern');
  }

  /**
   * Test if the current URL matches this agent's pattern.
   * @param {string} url
   * @returns {{ match: boolean, conversationId: string|null }}
   */
  matchUrl(url) {
    const match = url.match(this.urlPattern);
    if (!match) {
      return { match: false, conversationId: null };
    }
    return { match: true, conversationId: match[1] || null };
  }

  /**
   * Extract conversation title from the page.
   * @returns {string}
   */
  extractTitle() {
    // Placeholder — subclasses override with custom selectors/logic.
    return '';
  }

  /**
   * Extract messages from the page DOM.
   * @returns {Array<{ role: 'user'|'agent', content: string, timestamp: number }>}
   */
  extractMessages() {
    // Placeholder — subclasses override with custom selectors/logic.
    return [];
  }

  /**
   * Extract conversation date range.
   * @returns {{ startedAt: number, endedAt: number }} Unix milliseconds
   */
  extractDates() {
    // Placeholder — subclasses override with custom selectors/logic.
    return { startedAt: 0, endedAt: 0 };
  }

  /**
   * Extract the repository URL associated with this conversation.
   * Used to match the conversation to a local Buildermark project via git remote.
   * Should return a normalized URL like "github.com/owner/repo".
   * @returns {string|null}
   */
  extractRepoUrl() {
    // Placeholder — subclasses override with custom selectors/logic.
    return null;
  }

  /**
   * Check if the page content has fully loaded (enough to extract messages).
   * @returns {boolean}
   */
  isPageReady() {
    // Placeholder — subclasses can override for custom readiness checks.
    return document.readyState === 'complete';
  }
}

// Export for both module and non-module contexts.
if (typeof module !== 'undefined' && module.exports) {
  module.exports = { BaseAgent };
}
