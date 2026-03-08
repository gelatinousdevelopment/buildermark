/**
 * Content script listener for Claude Code Cloud fetch interception.
 * Receives events from the page-world fetch interceptor via postMessage,
 * debounces, and sends to the Buildermark server.
 */

// Current page import state, queryable by the popup.
let _buildermarkPageState = 'waiting';

function _setPageState(state) {
  _buildermarkPageState = state;
  try {
    chrome.runtime.sendMessage({ type: 'pageStateChanged', state });
  } catch {
    // Popup may not be open — ignore.
  }
}

// Listen for state queries from the popup.
if (typeof chrome !== 'undefined' && chrome.runtime && chrome.runtime.onMessage) {
  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message.type === 'getPageState') {
      sendResponse({ state: _buildermarkPageState });
    }
  });
}

class ClaudeCodeCloudListener {
  constructor(setBadge) {
    this.setBadge = setBadge;
    this._pending = null;
    this._debounceTimer = null;
    this._listening = false;
  }

  start() {
    if (this._listening) return;
    this._listening = true;

    window.addEventListener("message", (event) => {
      if (event.source !== window) return;
      if (!event.data || event.data.type !== "buildermark-claude-code-events") return;

      const { sessionId, url, data } = event.data;
      if (!sessionId || !data) return;

      console.log("[Buildermark] Received postMessage — sessionId:", sessionId, "url:", url, "events:", data?.length);

      // Each fetch response is a complete snapshot — replace previous.
      this._pending = { sessionId, url, events: data };
      this._scheduleSend();
    });
  }

  _scheduleSend() {
    if (this._debounceTimer) {
      clearTimeout(this._debounceTimer);
    }
    this._debounceTimer = setTimeout(() => {
      this._debounceTimer = null;
      this._send();
    }, 3000);
  }

  async _send() {
    const payload = this._pending;
    if (!payload) return;
    this._pending = null;

    try {
      _setPageState('importing');
      this.setBadge("loading");
      const params = {
        ...payload,
        agent: "claude_cloud",
      };
      console.log(
        "[Buildermark] Sending import-web request — url:",
        params.url,
        "agent:",
        params.agent,
        "events:",
        params.events?.length,
        "sessionId:",
        params.sessionId,
      );
      const result = await BuildermarkAPI.importConversation(params);
      console.log("[Buildermark] Import result:", JSON.stringify(result));
      _setPageState(result.alreadyExisted ? 'already' : 'done');
      this.setBadge(result.alreadyExisted ? "exists" : "done");
    } catch (e) {
      console.warn("[Buildermark] Failed to import Claude Code events:", e.message);
      _setPageState('error');
      this.setBadge("error");
    }
  }
}
