(function initializeChromeCompat(root) {
  if (!root.browser) {
    return;
  }

  const browserApi = root.browser;
  const compatChrome = {
    runtime: {
      getURL(path) {
        return browserApi.runtime.getURL(path);
      },
      onInstalled: browserApi.runtime.onInstalled,
      onMessage: browserApi.runtime.onMessage,
      onStartup: browserApi.runtime.onStartup,
      sendMessage(...args) {
        return wrapAsyncCall(browserApi.runtime.sendMessage, browserApi.runtime, args);
      },
      lastError: null,
    },
    storage: {
      local: {
        get(...args) {
          return wrapAsyncCall(browserApi.storage.local.get, browserApi.storage.local, args);
        },
        set(...args) {
          return wrapAsyncCall(browserApi.storage.local.set, browserApi.storage.local, args);
        },
      },
    },
  };

  if (browserApi.tabs) {
    compatChrome.tabs = {
      create(...args) {
        return wrapAsyncCall(browserApi.tabs.create, browserApi.tabs, args);
      },
      get(...args) {
        return wrapAsyncCall(browserApi.tabs.get, browserApi.tabs, args);
      },
      onActivated: browserApi.tabs.onActivated,
      onRemoved: browserApi.tabs.onRemoved,
      onUpdated: browserApi.tabs.onUpdated,
      query(...args) {
        return wrapAsyncCall(browserApi.tabs.query, browserApi.tabs, args);
      },
      sendMessage(...args) {
        return wrapAsyncCall(browserApi.tabs.sendMessage, browserApi.tabs, args);
      },
    };
  }

  if (browserApi.action) {
    compatChrome.action = {
      setBadgeBackgroundColor(...args) {
        return wrapAsyncCall(browserApi.action.setBadgeBackgroundColor, browserApi.action, args);
      },
      setBadgeText(...args) {
        return wrapAsyncCall(browserApi.action.setBadgeText, browserApi.action, args);
      },
      setIcon(...args) {
        return wrapAsyncCall(browserApi.action.setIcon, browserApi.action, args);
      },
      setTitle(...args) {
        return wrapAsyncCall(browserApi.action.setTitle, browserApi.action, args);
      },
    };
  }

  root.chrome = compatChrome;

  function wrapAsyncCall(fn, thisArg, args) {
    const callback = typeof args[args.length - 1] === "function" ? args.pop() : null;
    const missingApiError = typeof fn !== "function"
      ? new Error("This extension API is not available in this context")
      : null;

    if (missingApiError) {
      if (!callback) {
        return Promise.reject(missingApiError);
      }

      compatChrome.runtime.lastError = { message: missingApiError.message };
      try {
        callback();
      } finally {
        compatChrome.runtime.lastError = null;
      }
      return undefined;
    }

    if (!callback) {
      return fn.apply(thisArg, args);
    }

    fn.apply(thisArg, args).then(
      (result) => {
        callback(result);
      },
      (error) => {
        compatChrome.runtime.lastError = {
          message: error instanceof Error ? error.message : String(error),
        };
        try {
          callback();
        } finally {
          compatChrome.runtime.lastError = null;
        }
      },
    );

    return undefined;
  }
})(globalThis);
