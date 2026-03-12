/**
 * Generic cloud listener for coding agent fetch interception.
 * Receives intercepted data from the page-world fetch interceptor via postMessage,
 * debounces, and sends to the Buildermark server.
 */

// Current page import state, queryable by the popup.
let _buildermarkPageState = 'waiting';
let _autoImport = true;

// Initialize auto-import setting from storage.
if (typeof chrome !== 'undefined' && chrome.storage) {
  chrome.storage.local.get({ autoImport: true }, (result) => {
    _autoImport = result.autoImport;
  });
}

function _setPageState(state) {
  _buildermarkPageState = state;
  try {
    const result = chrome.runtime.sendMessage({ type: 'pageStateChanged', state });
    if (result && typeof result.catch === "function") {
      result.catch(() => {});
    }
  } catch {
    // Popup may not be open — ignore.
  }
}

// Listen for state queries and auto-import messages from the popup.
if (typeof chrome !== 'undefined' && chrome.runtime && chrome.runtime.onMessage) {
  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message.type === 'getPageState') {
      sendResponse({ state: _buildermarkPageState });
    } else if (message.type === 'autoImportChanged') {
      _autoImport = message.value;
      if (window._buildermarkCloudListener) {
        window._buildermarkCloudListener._scheduleSend();
      }
    } else if (message.type === 'triggerImport') {
      // Manual import trigger — send any held data.
      if (window._buildermarkCloudListener) {
        window._buildermarkCloudListener._send(true);
      }
    }
  });
}

class CloudListener {
  constructor(setBadge) {
    this.setBadge = setBadge;
    this._queue = [];
    this._debounceTimer = null;
    this._listening = false;
    this._sending = false;
  }

  start() {
    if (this._listening) return;
    this._listening = true;

    window.addEventListener("message", (event) => {
      if (event.source !== window) return;
      if (!event.data || event.data.type !== "buildermark-cloud-intercept") return;

      const { agent, matchId, url, data } = event.data;
      if (!matchId || !data) return;

      console.log("[Buildermark] Received cloud intercept — agent:", agent, "matchId:", matchId, "url:", url);

      this._queue.push({ agent, matchId, url, data });
      this._scheduleSend();
    });
  }

  _scheduleSend() {
    if (this._debounceTimer) {
      clearTimeout(this._debounceTimer);
      this._debounceTimer = null;
    }

    if (this._queue.length === 0) {
      return;
    }

    if (!_autoImport) {
      if (!this._sending) {
        _setPageState('pending');
        this.setBadge('pending');
      }
      return;
    }

    if (this._sending) {
      return;
    }

    this._debounceTimer = setTimeout(() => {
      this._debounceTimer = null;
      this._drainQueue();
    }, 3000);
  }

  async _send(force = false) {
    await this._drainQueue({ force });
  }

  async _drainQueue({ force = false } = {}) {
    if (this._sending || this._queue.length === 0) {
      return;
    }

    if (!_autoImport && !force) {
      _setPageState('pending');
      this.setBadge('pending');
      return;
    }

    this._sending = true;
    let finalState = null;

    try {
      while (this._queue.length > 0) {
        const payload = this._queue[0];

        _setPageState('importing');
        this.setBadge("importing");

        const params = {
          url: payload.url,
          agent: payload.agent,
          cloudData: payload.data,
        };
        console.log(
          "[Buildermark] Sending import-web request — url:",
          params.url,
          "agent:",
          params.agent,
        );

        let result;
        try {
          result = await BuildermarkAPI.importConversation(params);
        } catch (e) {
          console.warn("[Buildermark] Failed to import cloud data:", e.message);
          _setPageState('error');
          this.setBadge("error");
          return;
        }

        this._queue.shift();
        console.log("[Buildermark] Import result:", JSON.stringify(result));

        if (result?.status === 'no_messages') {
          console.log("[Buildermark] Cloud import returned no messages; skipping state change");
          continue;
        }

        finalState = result.alreadyExisted ? 'already' : 'done';
      }
    } finally {
      this._sending = false;
    }

    if (this._queue.length > 0) {
      if (_autoImport) {
        this._scheduleSend();
      } else {
        _setPageState('pending');
        this.setBadge('pending');
      }
      return;
    }

    if (finalState) {
      _setPageState(finalState);
      this.setBadge(finalState);
      return;
    }

    _setPageState('waiting');
    this.setBadge('waiting');
  }
}
