// Safari supports both browser.* and chrome.* APIs.
const extensionApi = typeof browser !== 'undefined' ? browser : chrome;
const actionApi = extensionApi.browserAction || extensionApi.action;

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
  try {
    const result = actionApi.setIcon({ tabId, path });
    if (result && typeof result.catch === 'function') {
      result.catch(() => {});
    }
  } catch (e) {
    // Ignore tabs that disappear while events are processed.
  }
}

function clearBadgeState(tabId) {
  try {
    const badgeResult = actionApi.setBadgeText({ text: '', tabId });
    if (badgeResult && typeof badgeResult.catch === 'function') {
      badgeResult.catch(() => {});
    }
    const titleResult = actionApi.setTitle({ title: 'Buildermark', tabId });
    if (titleResult && typeof titleResult.catch === 'function') {
      titleResult.catch(() => {});
    }
  } catch (e) {
    // Ignore tabs that disappear while events are processed.
  }
}

function getTab(tabId, callback) {
  const getFn = extensionApi.tabs && extensionApi.tabs.get;
  if (typeof getFn !== 'function') {
    callback(null);
    return;
  }

  try {
    if (getFn.length <= 1) {
      getFn(tabId)
        .then(tab => callback(tab || null))
        .catch(() => callback(null));
      return;
    }
  } catch (e) {
    // Fall through to callback-style API.
  }

  try {
    getFn(tabId, tab => {
      if (extensionApi.runtime && extensionApi.runtime.lastError) {
        callback(null);
        return;
      }
      callback(tab || null);
    });
  } catch (e) {
    callback(null);
  }
}

function queryActiveTab(callback) {
  const queryFn = extensionApi.tabs && extensionApi.tabs.query;
  if (typeof queryFn !== 'function') {
    callback(null);
    return;
  }

  const query = { active: true, currentWindow: true };
  try {
    if (queryFn.length <= 1) {
      queryFn(query)
        .then(tabs => callback((tabs && tabs[0]) || null))
        .catch(() => callback(null));
      return;
    }
  } catch (e) {
    // Fall through to callback-style API.
  }

  try {
    queryFn(query, tabs => {
      if (extensionApi.runtime && extensionApi.runtime.lastError) {
        callback(null);
        return;
      }
      callback((tabs && tabs[0]) || null);
    });
  } catch (e) {
    callback(null);
  }
}

function refreshTabIcon(tabId) {
  getTab(tabId, tab => {
    if (!tab) return;
    setTabIcon(tabId, tab.url || '');
  });
}

function refreshActiveTabIcon() {
  queryActiveTab(tab => {
    if (!tab || typeof tab.id !== 'number') return;
    setTabIcon(tab.id, tab.url || '');
  });
}

extensionApi.tabs.onActivated.addListener(({ tabId }) => {
  refreshTabIcon(tabId);
});

extensionApi.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (typeof changeInfo.url === 'string' || changeInfo.status === 'loading') {
    clearBadgeState(tabId);
  }

  if (typeof changeInfo.url === 'string' || changeInfo.status === 'complete') {
    setTabIcon(tabId, (tab && tab.url) || changeInfo.url || '');
  }
});

extensionApi.runtime.onInstalled.addListener(() => {
  refreshActiveTabIcon();
});

extensionApi.runtime.onStartup.addListener(() => {
  refreshActiveTabIcon();
});

refreshActiveTabIcon();

// Listen for badge state changes from the content script.
extensionApi.runtime.onMessage.addListener((message, sender) => {
  if (message.type !== 'setBadge' || !sender.tab) return;

  const tabId = sender.tab.id;

  switch (message.state) {
    case 'importing':
      actionApi.setBadgeText({ text: '...', tabId });
      actionApi.setBadgeBackgroundColor({ color: '#4a9eff', tabId });
      actionApi.setTitle({ title: 'Buildermark: Importing...', tabId });
      break;
    case 'done':
      actionApi.setBadgeText({ text: '\u2713', tabId });
      actionApi.setBadgeBackgroundColor({ color: '#4ecdc4', tabId });
      actionApi.setTitle({ title: 'Buildermark: Imported', tabId });
      break;
    case 'already':
      actionApi.setBadgeText({ text: '\u2713', tabId });
      actionApi.setBadgeBackgroundColor({ color: '#888', tabId });
      actionApi.setTitle({ title: 'Buildermark: Already imported', tabId });
      break;
    case 'error':
      actionApi.setBadgeText({ text: '!', tabId });
      actionApi.setBadgeBackgroundColor({ color: '#ff6b6b', tabId });
      actionApi.setTitle({ title: 'Buildermark: Import failed', tabId });
      break;
  }
});
