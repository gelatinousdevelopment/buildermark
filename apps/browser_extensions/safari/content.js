// Register agents.
registerAgent(new ClaudeCloudAgent());
registerAgent(new CodexCloudAgent());

// Badge communication with background script.
// Safari supports both browser.* and chrome.* APIs via WebExtension compatibility.
const extensionApi = typeof browser !== 'undefined' ? browser : chrome;

function setBadge(state) {
  try {
    extensionApi.runtime.sendMessage({ type: 'setBadge', state });
  } catch (e) {
    // Extension context may be invalidated — ignore.
  }
}

// Run the import when the page loads.
runImport(setBadge);

// Also watch for SPA navigation (URL changes without full page reload).
let lastUrl = window.location.href;
const observer = new MutationObserver(() => {
  if (window.location.href !== lastUrl) {
    lastUrl = window.location.href;
    setTimeout(() => runImport(setBadge), 2000);
  }
});
observer.observe(document.body, { childList: true, subtree: true });
