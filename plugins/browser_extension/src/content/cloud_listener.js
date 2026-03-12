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
    } else if (message.type === 'triggerImport') {
      // Manual import trigger — send any held data.
      if (window._buildermarkCloudListener) {
        window._buildermarkCloudListener._send();
      }
    }
  });
}

class CloudListener {
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
      if (!event.data || event.data.type !== "buildermark-cloud-intercept") return;

      const { agent, matchId, url, data } = event.data;
      if (!matchId || !data) return;

      console.log("[Buildermark] Received cloud intercept — agent:", agent, "matchId:", matchId, "url:", url);

      // For codex, prefer the most complete response (one with PR data).
      if (agent === 'codex_cloud' && this._pending && this._pending.agent === 'codex_cloud') {
        const prevHasPr = this._codexHasPr(this._pending.data);
        const newHasPr = this._codexHasPr(data);
        if (prevHasPr && !newHasPr) {
          console.log("[Buildermark] Keeping previous codex data — has PR diff, new one does not");
          this._scheduleSend();
          return;
        }
      }

      this._pending = { agent, matchId, url, data };
      this._scheduleSend();
    });
  }

  _scheduleSend() {
    if (this._debounceTimer) {
      clearTimeout(this._debounceTimer);
    }

    if (!_autoImport) {
      // Hold data without sending — user must click Import.
      _setPageState('pending');
      this.setBadge('pending');
      return;
    }

    this._debounceTimer = setTimeout(() => {
      this._debounceTimer = null;
      this._send();
    }, 3000);
  }

  _codexHasPr(data) {
    if (!data?.turn_mapping) return false;
    return Object.values(data.turn_mapping).some(wrapper =>
      wrapper.turn?.output_items?.some(item => item.type === 'pr')
    );
  }

  async _send() {
    const payload = this._pending;
    if (!payload) return;
    this._pending = null;

    try {
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
      const result = await BuildermarkAPI.importConversation(params);
      console.log("[Buildermark] Import result:", JSON.stringify(result));
      _setPageState(result.alreadyExisted ? 'already' : 'done');
      this.setBadge(result.alreadyExisted ? "already" : "done");
    } catch (e) {
      console.warn("[Buildermark] Failed to import cloud data:", e.message);
      _setPageState('error');
      this.setBadge("error");
    }
  }
}
