/**
 * API client for communicating with the Buildermark local server.
 */
const API_BASE = 'http://localhost:7022/api/v1';

const BuildermarkAPI = {
  /**
   * Check if a conversation URL has already been imported.
   * @param {string} url
   * @returns {Promise<{ imported: boolean, conversationId?: string }>}
   */
  async checkUrl(url) {
    const resp = await fetch(`${API_BASE}/conversations/check-url?url=${encodeURIComponent(url)}`);
    const json = await resp.json();
    if (!json.ok) {
      throw new Error(json.error || 'Failed to check URL');
    }
    return json.data;
  },

  /**
   * Import a conversation from a web page.
   * @param {Object} params
   * @param {string} params.url - The page URL
   * @param {string} params.agent - Agent identifier (e.g., 'claude_cloud')
   * @param {string} params.title - Conversation title
   * @param {number} params.startedAt - Unix ms
   * @param {number} params.endedAt - Unix ms
   * @param {string} [params.repoUrl] - Normalized repo URL (e.g., 'github.com/owner/repo')
   * @param {Array<{ role: string, content: string, timestamp: number, model?: string }>} [params.messages]
   * @param {string} [params.sessionId] - Session ID for cloud events
   * @param {Array} [params.events] - Raw cloud event objects
   * @returns {Promise<{ imported: boolean, conversationId: string, alreadyExisted: boolean, messageCount?: number }>}
   */
  async importConversation(params) {
    const resp = await fetch(`${API_BASE}/conversations/import-web`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
    const json = await resp.json();
    if (!json.ok) {
      throw new Error(json.error || 'Failed to import conversation');
    }
    return json.data;
  },
};

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { BuildermarkAPI, API_BASE };
}
