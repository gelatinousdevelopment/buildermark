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

const ACTIVE_URL_PATTERNS = [
  /https?:\/\/(?:[^/]+\.)?claude\.ai\/(?:project\/[^/]+\/)?chat\/([a-f0-9-]+)(?:[/?#]|$)/i,
  /https?:\/\/chatgpt\.com\/codex\/(?:s\/)?([a-zA-Z0-9_-]+)(?:[/?#]|$)/i,
  /https?:\/\/codex\.openai\.com\/(?:s\/)?([a-zA-Z0-9_-]+)(?:[/?#]|$)/i,
  /https?:\/\/(?:[^/]+\.)?claude\.ai\/code\/([^/?#]+)(?:[/?#]|$)/i,
];
const API_BASE = "http://localhost:7022/api/v1/";

// Track per-tab import state for the popup.
const tabStates = new Map();

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

function isActiveUrl(url) {
  if (!url) return false;
  return ACTIVE_URL_PATTERNS.some((pattern) => pattern.test(url));
}

function setTabIcon(tabId, url) {
  const path = isActiveUrl(url) ? BLUE_ICONS : GRAY_ICONS;
  chrome.action.setIcon({ tabId, path });
}

function clearBadgeState(tabId) {
  chrome.action.setBadgeText({ text: "", tabId });
  chrome.action.setTitle({ title: "Buildermark", tabId });
}

function refreshTabIcon(tabId) {
  chrome.tabs.get(tabId, (tab) => {
    if (chrome.runtime.lastError || !tab) return;
    setTabIcon(tabId, tab.url || "");
  });
}

function refreshActiveTabIcon() {
  chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
    if (chrome.runtime.lastError || !tabs || tabs.length === 0) return;
    const tab = tabs[0];
    if (typeof tab.id !== "number") return;
    setTabIcon(tab.id, tab.url || "");
  });
}

chrome.tabs.onActivated.addListener(({ tabId }) => {
  refreshTabIcon(tabId);
});

chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (typeof changeInfo.url === "string" || changeInfo.status === "loading") {
    clearBadgeState(tabId);
    tabStates.delete(tabId);
  }

  if (typeof changeInfo.url === "string" || changeInfo.status === "complete") {
    setTabIcon(tabId, (tab && tab.url) || changeInfo.url || "");
  }
});

chrome.runtime.onInstalled.addListener(() => {
  refreshActiveTabIcon();
});

chrome.runtime.onStartup.addListener(() => {
  refreshActiveTabIcon();
});

refreshActiveTabIcon();

function getTabState(tabId, url) {
  if (tabStates.has(tabId)) return tabStates.get(tabId);
  if (isActiveUrl(url)) return "waiting";
  return "ignored";
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
    chrome.tabs.get(message.tabId, (tab) => {
      if (chrome.runtime.lastError || !tab) {
        sendResponse({ state: "ignored" });
        return;
      }
      sendResponse({ state: getTabState(message.tabId, tab.url || "") });
    });
    return true; // async response
  }

  if (message.type !== "setBadge" || !sender.tab) return;

  const tabId = sender.tab.id;
  tabStates.set(tabId, message.state);

  console.log("message.state", message.state);
  switch (message.state) {
    case "ignored":
    case "waiting":
      clearBadgeState(tabId);
      break;
    case "importing":
      chrome.action.setBadgeText({ text: "...", tabId });
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
      chrome.action.setBadgeBackgroundColor({ color: "#888", tabId });
      chrome.action.setTitle({ title: "Buildermark: Already imported", tabId });
      break;
    case "error":
      chrome.action.setBadgeText({ text: "!", tabId });
      chrome.action.setBadgeBackgroundColor({ color: "#ff6b6b", tabId });
      chrome.action.setTitle({ title: "Buildermark: Import failed", tabId });
      break;
  }
});
