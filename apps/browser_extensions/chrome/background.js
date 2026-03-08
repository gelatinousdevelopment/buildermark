const GRAY_ICONS = {
  16: 'icons/icon16.png',
  32: 'icons/icon32.png',
  48: 'icons/icon48.png',
  128: 'icons/icon128.png',
};

const BLUE_ICONS = {
  16: 'icons/blue_icon16.png',
  32: 'icons/blue_icon32.png',
  48: 'icons/blue_icon48.png',
  128: 'icons/blue_icon128.png',
};

const ACTIVE_URL_PATTERNS = [
  /https?:\/\/(?:[^/]+\.)?claude\.ai\/(?:project\/[^/]+\/)?chat\/([a-f0-9-]+)(?:[/?#]|$)/i,
  /https?:\/\/chatgpt\.com\/codex\/(?:s\/)?([a-zA-Z0-9_-]+)(?:[/?#]|$)/i,
  /https?:\/\/codex\.openai\.com\/(?:s\/)?([a-zA-Z0-9_-]+)(?:[/?#]|$)/i,
  /https?:\/\/(?:[^/]+\.)?claude\.ai\/code\/([^/?#]+)(?:[/?#]|$)/i,
];

function isActiveUrl(url) {
  if (!url) return false;
  return ACTIVE_URL_PATTERNS.some(pattern => pattern.test(url));
}

function setTabIcon(tabId, url) {
  const path = isActiveUrl(url) ? BLUE_ICONS : GRAY_ICONS;
  chrome.action.setIcon({ tabId, path });
}

function clearBadgeState(tabId) {
  chrome.action.setBadgeText({ text: '', tabId });
  chrome.action.setTitle({ title: 'Buildermark', tabId });
}

function refreshTabIcon(tabId) {
  chrome.tabs.get(tabId, tab => {
    if (chrome.runtime.lastError || !tab) return;
    setTabIcon(tabId, tab.url || '');
  });
}

function refreshActiveTabIcon() {
  chrome.tabs.query({ active: true, currentWindow: true }, tabs => {
    if (chrome.runtime.lastError || !tabs || tabs.length === 0) return;
    const tab = tabs[0];
    if (typeof tab.id !== 'number') return;
    setTabIcon(tab.id, tab.url || '');
  });
}

chrome.tabs.onActivated.addListener(({ tabId }) => {
  refreshTabIcon(tabId);
});

chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (typeof changeInfo.url === 'string' || changeInfo.status === 'loading') {
    clearBadgeState(tabId);
  }

  if (typeof changeInfo.url === 'string' || changeInfo.status === 'complete') {
    setTabIcon(tabId, (tab && tab.url) || changeInfo.url || '');
  }
});

chrome.runtime.onInstalled.addListener(() => {
  refreshActiveTabIcon();
});

chrome.runtime.onStartup.addListener(() => {
  refreshActiveTabIcon();
});

refreshActiveTabIcon();

// Listen for badge state changes from the content script.
chrome.runtime.onMessage.addListener((message, sender) => {
  if (message.type !== 'setBadge' || !sender.tab) return;

  const tabId = sender.tab.id;

  switch (message.state) {
    case 'importing':
      chrome.action.setBadgeText({ text: '...', tabId });
      chrome.action.setBadgeBackgroundColor({ color: '#4a9eff', tabId });
      chrome.action.setTitle({ title: 'Buildermark: Importing...', tabId });
      break;
    case 'done':
      chrome.action.setBadgeText({ text: '\u2713', tabId });
      chrome.action.setBadgeBackgroundColor({ color: '#4ecdc4', tabId });
      chrome.action.setTitle({ title: 'Buildermark: Imported', tabId });
      break;
    case 'already':
      chrome.action.setBadgeText({ text: '\u2713', tabId });
      chrome.action.setBadgeBackgroundColor({ color: '#888', tabId });
      chrome.action.setTitle({ title: 'Buildermark: Already imported', tabId });
      break;
    case 'error':
      chrome.action.setBadgeText({ text: '!', tabId });
      chrome.action.setBadgeBackgroundColor({ color: '#ff6b6b', tabId });
      chrome.action.setTitle({ title: 'Buildermark: Import failed', tabId });
      break;
  }
});
