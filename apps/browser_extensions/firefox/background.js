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

function clearBadgeState(tabId) {
  browser.browserAction.setBadgeText({ text: '', tabId }).catch(() => {});
  browser.browserAction.setTitle({ title: 'Buildermark', tabId }).catch(() => {});
}

async function refreshTabIcon(tabId) {
  try {
    const tab = await browser.tabs.get(tabId);
    const path = isActiveUrl(tab && tab.url) ? BLUE_ICONS : GRAY_ICONS;
    await browser.browserAction.setIcon({ tabId, path });
  } catch (e) {
    // Ignore tabs that disappear while events are processed.
  }
}

async function refreshActiveTabIcon() {
  try {
    const tabs = await browser.tabs.query({ active: true, currentWindow: true });
    if (!tabs || tabs.length === 0) return;
    const tab = tabs[0];
    if (typeof tab.id !== 'number') return;
    const path = isActiveUrl(tab.url) ? BLUE_ICONS : GRAY_ICONS;
    await browser.browserAction.setIcon({ tabId: tab.id, path });
  } catch (e) {
    // Ignore startup race conditions.
  }
}

browser.tabs.onActivated.addListener(({ tabId }) => {
  refreshTabIcon(tabId);
});

browser.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (typeof changeInfo.url === 'string' || changeInfo.status === 'loading') {
    clearBadgeState(tabId);
  }

  if (typeof changeInfo.url === 'string' || changeInfo.status === 'complete') {
    const path = isActiveUrl((tab && tab.url) || changeInfo.url || '') ? BLUE_ICONS : GRAY_ICONS;
    browser.browserAction.setIcon({ tabId, path }).catch(() => {});
  }
});

browser.runtime.onInstalled.addListener(() => {
  refreshActiveTabIcon();
});

browser.runtime.onStartup.addListener(() => {
  refreshActiveTabIcon();
});

refreshActiveTabIcon();

// Listen for badge state changes from the content script.
browser.runtime.onMessage.addListener((message, sender) => {
  if (message.type !== 'setBadge' || !sender.tab) return;

  const tabId = sender.tab.id;

  switch (message.state) {
    case 'importing':
      browser.browserAction.setBadgeText({ text: '...', tabId });
      browser.browserAction.setBadgeBackgroundColor({ color: '#4a9eff', tabId });
      browser.browserAction.setTitle({ title: 'Buildermark: Importing...', tabId });
      break;
    case 'done':
      browser.browserAction.setBadgeText({ text: '\u2713', tabId });
      browser.browserAction.setBadgeBackgroundColor({ color: '#4ecdc4', tabId });
      browser.browserAction.setTitle({ title: 'Buildermark: Imported', tabId });
      break;
    case 'already':
      browser.browserAction.setBadgeText({ text: '\u2713', tabId });
      browser.browserAction.setBadgeBackgroundColor({ color: '#888', tabId });
      browser.browserAction.setTitle({ title: 'Buildermark: Already imported', tabId });
      break;
    case 'error':
      browser.browserAction.setBadgeText({ text: '!', tabId });
      browser.browserAction.setBadgeBackgroundColor({ color: '#ff6b6b', tabId });
      browser.browserAction.setTitle({ title: 'Buildermark: Import failed', tabId });
      break;
  }
});
