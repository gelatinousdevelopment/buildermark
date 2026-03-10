/**
 * API client for communicating with the Buildermark local server.
 */
const API_BASE = 'http://localhost:7022/api/v1';

function sendApiRequest(endpoint, options = {}) {
  return new Promise((resolve, reject) => {
    chrome.runtime.sendMessage(
      {
        type: 'buildermarkApiRequest',
        endpoint,
        options,
      },
      (response) => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }

        if (!response) {
          reject(new Error('No response from extension background worker'));
          return;
        }

        if (!response.ok) {
          reject(new Error(response.error || 'Extension API request failed'));
          return;
        }

        resolve(response.data);
      },
    );
  });
}

const BuildermarkAPI = {
  /**
   * Check if a conversation URL has already been imported.
   * @param {string} url
   * @returns {Promise<{ imported: boolean, conversationId?: string }>}
   */
  async checkUrl(url) {
    return sendApiRequest(`${API_BASE}/conversations/check-url?url=${encodeURIComponent(url)}`);
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
    return sendApiRequest(`${API_BASE}/conversations/import-web`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
  },
};

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { BuildermarkAPI, API_BASE };
}
