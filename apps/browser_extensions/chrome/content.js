// Register agents.
registerAgent(new ClaudeCloudAgent());
registerAgent(new CodexCloudAgent());

// Badge communication with background script.
function setBadge(state) {
  try {
    chrome.runtime.sendMessage({ type: 'setBadge', state });
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
    // Small delay to let the new page content render.
    setTimeout(() => runImport(setBadge), 2000);
  }
});
observer.observe(document.body, { childList: true, subtree: true });
