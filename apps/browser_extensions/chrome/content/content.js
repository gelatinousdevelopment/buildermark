// Inject the fetch interceptor into the page's main world.
const script = document.createElement("script");
script.src = chrome.runtime.getURL("web_accessible/fetch_interceptor.js");
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

// Also watch for SPA navigation (URL changes without full page reload).
let lastUrl = window.location.href;
const observer = new MutationObserver(() => {
  if (window.location.href !== lastUrl) {
    lastUrl = window.location.href;
    // Immediately reset state so the popup updates.
    _setPageState("waiting");
    setBadge("waiting");
  }
});
observer.observe(document.documentElement, { childList: true, subtree: true });
