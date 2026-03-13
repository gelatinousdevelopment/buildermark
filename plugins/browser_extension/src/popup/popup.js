const API_BASE = "http://localhost:55022/api/v1";

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

// Page status labels and colors.
const PAGE_STATES = {
  ignored: { text: "Not a supported page", dot: "gray" },
  waiting: { text: "Waiting for conversation data...", dot: "yellow" },
  importing: { text: "Sending conversation data...", dot: "orange" },
  done: { text: "Finished importing", dot: "green" },
  already: { text: "Already imported", dot: "green" },
  error: { text: "Import failed", dot: "red" },
  server_unavailable: { text: "Local server not reachable", dot: "red" },
  pending: { text: "Ready to import", dot: "orange" },
};

// Read the active page state from the background so opening the popup does not
// require touching the current tab directly.
async function checkPageState() {
  try {
    const response = await chrome.runtime.sendMessage({ type: "getActiveTabState" });
    setPageState(response?.state || "ignored");
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
  if (message.type === "activeTabStateChanged") {
    setPageState(message.state);
  }
});

checkPageState();

// Auto-import toggle.
chrome.storage.local.get({ autoImport: true }, (result) => {
  autoImportCheckbox.checked = result.autoImport;
});

autoImportCheckbox.addEventListener("change", () => {
  const value = autoImportCheckbox.checked;
  chrome.storage.local.set({ autoImport: value });
  chrome.runtime.sendMessage({ type: "autoImportChanged", value }).catch(() => {});
});

// Import button — trigger manual import on active tab.
importBtn.addEventListener("click", () => {
  chrome.runtime.sendMessage({ type: "triggerImport" }).catch(() => {});
});
