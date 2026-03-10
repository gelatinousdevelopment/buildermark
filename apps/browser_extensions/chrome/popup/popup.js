const API_BASE = "http://localhost:7022/api/v1";

const serverDot = document.getElementById("server-dot");
const pageDot = document.getElementById("page-dot");
const pageStatus = document.getElementById("page-status");
const openLink = document.getElementById("open-link");
const autoImportCheckbox = document.getElementById("auto-import");
const importBtn = document.getElementById("import-btn");

// Open link in new tab instead of navigating the popup.
openLink.addEventListener("click", (e) => {
  e.preventDefault();
  chrome.tabs.create({ url: openLink.href });
  window.close();
});

// URL patterns that indicate a supported page (must match background.js).
const ACTIVE_URL_PATTERNS = [
  /https?:\/\/(?:[^/]+\.)?claude\.ai\/(?:project\/[^/]+\/)?chat\/([a-f0-9-]+)(?:[/?#]|$)/i,
  /https?:\/\/chatgpt\.com\/codex\/(?:s\/)?([a-zA-Z0-9_-]+)(?:[/?#]|$)/i,
  /https?:\/\/codex\.openai\.com\/(?:s\/)?([a-zA-Z0-9_-]+)(?:[/?#]|$)/i,
  /https?:\/\/(?:[^/]+\.)?claude\.ai\/code\/([^/?#]+)(?:[/?#]|$)/i,
];

function isActiveUrl(url) {
  if (!url) return false;
  return ACTIVE_URL_PATTERNS.some((p) => p.test(url));
}

// Page status labels and colors.
const PAGE_STATES = {
  ignored: { text: "Not a supported page", dot: "gray" },
  waiting: { text: "Waiting for conversation data...", dot: "yellow" },
  importing: { text: "Sending conversation data...", dot: "blue" },
  done: { text: "Finished importing", dot: "green" },
  already: { text: "Already imported", dot: "green" },
  error: { text: "Import failed", dot: "red" },
  server_unavailable: { text: "Local server not reachable", dot: "red" },
  pending: { text: "Ready to import", dot: "blue" },
};

// Query the content script directly for the import state.
async function checkPageState() {
  try {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (!tab) {
      setPageState("ignored");
      return;
    }

    // Not a supported URL — no content script will be running.
    if (!isActiveUrl(tab.url)) {
      setPageState("ignored");
      return;
    }

    // Ask the content script for its state.
    try {
      const response = await chrome.tabs.sendMessage(tab.id, { type: "getPageState" });
      setPageState(response?.state || "waiting");
    } catch {
      // Content script not loaded yet or not responding.
      setPageState("waiting");
    }
  } catch {
    setPageState("ignored");
  }
}

function setPageState(state) {
  const info = PAGE_STATES[state] || PAGE_STATES.ignored;
  pageDot.className = "status-dot " + info.dot;
  pageStatus.textContent = info.text;

  // Show Import button only when state is "pending".
  if (state === "pending") {
    importBtn.classList.remove("hidden");
  } else {
    importBtn.classList.add("hidden");
  }
}

// Listen for live state changes from content scripts while the popup is open.
chrome.runtime.onMessage.addListener((message) => {
  if (message.type === "pageStateChanged") {
    setPageState(message.state);
  }
});

// Re-check page state on navigation or tab switch.
chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
  if (changeInfo.url) {
    // Immediately reset on URL change before querying the content script.
    setPageState(isActiveUrl(changeInfo.url) ? "waiting" : "ignored");
    checkPageState();
  } else if (changeInfo.status === "complete") {
    checkPageState();
  }
});
chrome.tabs.onActivated.addListener(() => {
  checkPageState();
});

checkPageState();

// Auto-import toggle.
chrome.storage.local.get({ autoImport: true }, (result) => {
  autoImportCheckbox.checked = result.autoImport;
});

autoImportCheckbox.addEventListener("change", () => {
  const value = autoImportCheckbox.checked;
  chrome.storage.local.set({ autoImport: value });
  // Broadcast to content scripts in the active tab.
  chrome.tabs.query({ active: true, currentWindow: true }, ([tab]) => {
    if (tab) {
      chrome.tabs.sendMessage(tab.id, { type: "autoImportChanged", value }).catch(() => {});
    }
  });
});

// Import button — trigger manual import on active tab.
importBtn.addEventListener("click", () => {
  chrome.tabs.query({ active: true, currentWindow: true }, ([tab]) => {
    if (tab) {
      chrome.tabs.sendMessage(tab.id, { type: "triggerImport" }).catch(() => {});
    }
  });
});
