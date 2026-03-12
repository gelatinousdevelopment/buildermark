const GRAY_ICONS = {
  16: "../icons/icon16.png",
  32: "../icons/icon32.png",
  48: "../icons/icon48.png",
  128: "../icons/icon128.png",
};

const BLUE_ICONS = {
  16: "../icons/blue_icon16.png",
  32: "../icons/blue_icon32.png",
  48: "../icons/blue_icon48.png",
  128: "../icons/blue_icon128.png",
};

const API_BASE = "http://localhost:7022/api/v1/";

// Track per-tab import state for the popup.
const tabStates = new Map();
let activeTabId = null;

async function handleApiRequest(endpoint, options = {}) {
  if (typeof endpoint !== "string" || !endpoint.startsWith(API_BASE)) {
    return {
      ok: false,
      error: "Blocked unexpected API endpoint",
    };
  }

  let response;

  try {
    response = await fetch(endpoint, options);
  } catch (error) {
    return {
      ok: false,
      error: error?.message || "Failed to reach Buildermark local server",
    };
  }

  let json;
  try {
    json = await response.json();
  } catch {
    return {
      ok: false,
      error: `Buildermark server returned ${response.status} ${response.statusText || "response"}`,
    };
  }

  if (!response.ok) {
    return {
      ok: false,
      error: json?.error || `Buildermark server returned ${response.status}`,
    };
  }

  return {
    ok: Boolean(json?.ok),
    data: json?.data,
    error: json?.ok ? undefined : json?.error || "Buildermark request failed",
  };
}

function setTabIcon(tabId, isSupported) {
  const path = isSupported ? BLUE_ICONS : GRAY_ICONS;
  chrome.action.setIcon({ tabId, path });
}

function clearBadgeState(tabId) {
  chrome.action.setBadgeText({ text: "", tabId });
  chrome.action.setTitle({ title: "Buildermark", tabId });
}

function notifyActiveTabState(state) {
  try {
    const result = chrome.runtime.sendMessage({ type: "activeTabStateChanged", state });
    if (result && typeof result.catch === "function") {
      result.catch(() => {});
    }
  } catch {
    // Popup may not be open.
  }
}

function applyTabState(tabId, state) {
  const isSupported = state !== "ignored";
  setTabIcon(tabId, isSupported);

  if (!isSupported || state === "waiting" || state === "pending") {
    clearBadgeState(tabId);
  }

  switch (state) {
    case "ignored":
    case "waiting":
      break;
    case "importing":
      chrome.action.setBadgeText({ text: "↑", tabId });
      chrome.action.setBadgeBackgroundColor({ color: "#4a9eff", tabId });
      chrome.action.setTitle({ title: "Buildermark: Importing...", tabId });
      break;
    case "done":
      chrome.action.setBadgeText({ text: "\u2713", tabId });
      chrome.action.setBadgeBackgroundColor({ color: "#4ecdc4", tabId });
      chrome.action.setTitle({ title: "Buildermark: Imported", tabId });
      break;
    case "already":
      chrome.action.setBadgeText({ text: "\u2713", tabId });
      chrome.action.setBadgeBackgroundColor({ color: "#aaa", tabId });
      chrome.action.setTitle({ title: "Buildermark: Already imported", tabId });
      break;
    case "error":
      chrome.action.setBadgeText({ text: "!", tabId });
      chrome.action.setBadgeBackgroundColor({ color: "#ff6b6b", tabId });
      chrome.action.setTitle({ title: "Buildermark: Import failed", tabId });
      break;
    case "server_unavailable":
      chrome.action.setBadgeText({ text: "!", tabId });
      chrome.action.setBadgeBackgroundColor({ color: "#ffb347", tabId });
      chrome.action.setTitle({ title: "Buildermark: Local server unavailable", tabId });
      break;
  }

  if (tabId === activeTabId) {
    notifyActiveTabState(state);
  }
}

if (chrome.tabs?.onActivated && typeof chrome.tabs.onActivated.addListener === "function") {
  chrome.tabs.onActivated.addListener(({ tabId }) => {
    activeTabId = tabId;
    applyTabState(tabId, getTabState(tabId));
  });
}

if (chrome.tabs?.onUpdated && typeof chrome.tabs.onUpdated.addListener === "function") {
  chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
    if (changeInfo.status === "loading") {
      tabStates.delete(tabId);
      applyTabState(tabId, "ignored");
    }
  });
}

if (chrome.tabs?.onRemoved && typeof chrome.tabs.onRemoved.addListener === "function") {
  chrome.tabs.onRemoved.addListener((tabId) => {
    tabStates.delete(tabId);
    if (activeTabId === tabId) {
      activeTabId = null;
    }
  });
}

function getTabState(tabId) {
  if (tabStates.has(tabId)) return tabStates.get(tabId);
  return "ignored";
}

function getActiveTabState() {
  if (typeof activeTabId !== "number") return "ignored";
  return getTabState(activeTabId);
}

function forwardMessageToActiveTab(message) {
  if (typeof activeTabId !== "number" || getActiveTabState() === "ignored") {
    return;
  }

  chrome.tabs.sendMessage(activeTabId, message).catch(() => {});
}

// Listen for messages from content scripts and popup.
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.type === "buildermarkApiRequest") {
    handleApiRequest(message.endpoint, message.options)
      .then((result) => sendResponse(result))
      .catch((error) => {
        sendResponse({
          ok: false,
          error: error?.message || "Unexpected Buildermark API error",
        });
      });
    return true;
  }

  if (message.type === "getTabState") {
    sendResponse({ state: getTabState(message.tabId) });
    return;
  }

  if (message.type === "getActiveTabState") {
    sendResponse({ state: getActiveTabState() });
    return;
  }

  if (message.type === "autoImportChanged") {
    forwardMessageToActiveTab({ type: "autoImportChanged", value: message.value });
    return;
  }

  if (message.type === "triggerImport") {
    forwardMessageToActiveTab({ type: "triggerImport" });
    return;
  }

  if ((message.type === "setBadge" || message.type === "pageStateChanged") && sender.tab) {
    const tabId = sender.tab.id;
    const state = message.state || "ignored";

    if (state === "ignored") {
      tabStates.delete(tabId);
    } else {
      tabStates.set(tabId, state);
    }

    if (sender.tab.active) {
      activeTabId = tabId;
    }

    applyTabState(tabId, state);
  }
});
