/**
 * Generic fetch interceptor for cloud coding agent sessions.
 * Injected into the page's main world to intercept fetch responses
 * and forward the full JSON payload to the content script via postMessage.
 */
(function () {
  const INTERCEPT_PATTERNS = [
    { pattern: /\/v1\/sessions\/(session_[a-zA-Z0-9]+)\/events/, agent: "claude_cloud" },
    { pattern: /\/backend-api\/wham\/tasks\/(task_e_[a-zA-Z0-9_]+)\/turns/, agent: "codex_cloud", validate: body => body && body.turn_mapping && Object.keys(body.turn_mapping).length > 0 },
  ];
  const originalFetch = window.fetch;

  window.fetch = async function (...args) {
    const response = await originalFetch.apply(this, args);
    try {
      const url = typeof args[0] === 'string' ? args[0] : args[0]?.url;
      if (!url) return response;
      for (const { pattern, agent, validate } of INTERCEPT_PATTERNS) {
        const match = url.match(pattern);
        if (!match) continue;
        const clone = response.clone();
        clone.json().then(body => {
          if (!body) return;
          if (validate && !validate(body)) {
            console.log('[Buildermark] Skipping intercepted response — validation failed, keys:', body ? Object.keys(body).join(', ') : 'null');
            return;
          }
          window.postMessage({
            type: 'buildermark-cloud-intercept',
            agent,
            matchId: match[1],
            url: window.location.href,
            data: body,
          }, '*');
        }).catch(() => {});
        break;
      }
    } catch (e) {}
    return response;
  };
})();
