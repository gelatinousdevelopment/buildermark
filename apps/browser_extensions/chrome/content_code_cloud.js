// Inject the fetch interceptor into the page's main world.
const script = document.createElement('script');
script.src = chrome.runtime.getURL('shared/fetch_interceptor.js');
(document.head || document.documentElement).appendChild(script);
script.onload = () => script.remove();

// Badge communication with background script.
function setBadge(state) {
  try {
    chrome.runtime.sendMessage({ type: 'setBadge', state });
  } catch (e) {
    // Extension context may be invalidated — ignore.
  }
}

// Start the listener.
const listener = new ClaudeCodeCloudListener(setBadge);
listener.start();
