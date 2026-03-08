/**
 * Fetch interceptor for Claude Code Cloud sessions.
 * Injected into the page's main world to intercept fetch responses
 * to /v1/sessions/<sessionId>/events endpoints.
 */
(function () {
  const SESSION_EVENTS_RE = /\/v1\/sessions\/(session_[a-zA-Z0-9]+)\/events/;
  const originalFetch = window.fetch;

  window.fetch = async function (...args) {
    const response = await originalFetch.apply(this, args);

    try {
      const url = typeof args[0] === 'string' ? args[0] : args[0]?.url;
      if (!url) return response;

      const match = url.match(SESSION_EVENTS_RE);
      if (!match) return response;

      const sessionId = match[1];
      console.log('[Buildermark] Intercepted session events fetch for', sessionId, 'status:', response.status);
      const clone = response.clone();

      clone.json().then(body => {
        // The API returns { data: [...], has_more: bool } — unwrap to get the events array.
        const events = Array.isArray(body) ? body : Array.isArray(body?.data) ? body.data : null;
        console.log('[Buildermark] Parsed response — events:', events?.length ?? 0, 'isArray(body):', Array.isArray(body), 'has body.data:', Array.isArray(body?.data));
        if (!events || events.length === 0) return;
        const eventTypes = {};
        for (const ev of events) {
          const key = ev.type + (ev.subtype ? ':' + ev.subtype : '');
          eventTypes[key] = (eventTypes[key] || 0) + 1;
        }
        console.log('[Buildermark] Event type breakdown:', JSON.stringify(eventTypes));
        window.postMessage({
          type: 'buildermark-claude-code-events',
          sessionId,
          url: window.location.href,
          data: events,
        }, '*');
      }).catch((err) => {
        console.warn('[Buildermark] Failed to parse intercepted response as JSON:', err.message);
      });
    } catch (e) {
      // Never interfere with normal fetch behavior.
    }

    return response;
  };
})();
