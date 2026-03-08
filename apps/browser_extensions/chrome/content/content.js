// Inject the fetch interceptor into the page's main world.
const script = document.createElement("script");
script.src = chrome.runtime.getURL("fetch_interceptor.js");
(document.head || document.documentElement).appendChild(script);
script.onload = () => script.remove();

// Badge communication with background script.
function setBadge(state) {
  try {
    chrome.runtime.sendMessage({ type: "setBadge", state });
  } catch (e) {
    // Extension context may be invalidated — ignore.
  }
}

// Start the listener.
const listener = new ClaudeCodeCloudListener(setBadge);
listener.start();

// Run the import when the page loads.
runImport(setBadge);

// Also watch for SPA navigation (URL changes without full page reload).
let lastUrl = window.location.href;
const observer = new MutationObserver(() => {
  if (window.location.href !== lastUrl) {
    lastUrl = window.location.href;
    // Immediately reset state so the popup updates.
    _setPageState("waiting");
    setBadge("waiting");
    runImport(setBadge);
  }
});
observer.observe(document.body, { childList: true, subtree: true });
