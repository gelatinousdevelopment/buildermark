/**
 * Agent importer for Claude Code Cloud (claude.ai conversations).
 */
class ClaudeCloudAgent extends BaseAgent {
  get name() {
    return 'Claude Cloud';
  }

  get agentId() {
    return 'claude_cloud';
  }

  get urlPattern() {
    // Matches URLs like https://claude.ai/chat/<conversation-id>
    // or https://claude.ai/project/<project-id>/chat/<conversation-id>
    return /claude\.ai\/(?:project\/[^/]+\/)?chat\/([a-f0-9-]+)/;
  }

  extractTitle() {
    // TODO: Implement with actual Claude Cloud DOM selectors.
    // Likely a heading or sidebar element with the conversation title.
    const titleEl = document.querySelector('[data-testid="conversation-title"], h1.conversation-title, title');
    if (titleEl) {
      const text = titleEl.textContent.trim();
      // Remove " - Claude" suffix from page title
      return text.replace(/\s*[-–]\s*Claude\s*$/, '');
    }
    return '';
  }

  extractMessages() {
    // TODO: Implement with actual Claude Cloud DOM selectors.
    // These are placeholder selectors — update with real ones after inspecting the DOM.
    const messages = [];

    // Placeholder: look for message containers
    const messageEls = document.querySelectorAll('[data-testid="message"], .message-container, [class*="Message"]');

    messageEls.forEach((el, index) => {
      const role = this._detectRole(el);
      const content = this._extractContent(el);
      const timestamp = this._extractTimestamp(el, index);

      if (content) {
        messages.push({ role, content, timestamp });
      }
    });

    return messages;
  }

  extractDates() {
    // TODO: Extract actual dates from the page.
    // Placeholder: look for date elements in the conversation.
    const dateEls = document.querySelectorAll('[data-testid="message-timestamp"], time, [datetime]');
    let startedAt = 0;
    let endedAt = 0;

    dateEls.forEach(el => {
      const dateStr = el.getAttribute('datetime') || el.textContent;
      const ts = new Date(dateStr).getTime();
      if (!isNaN(ts) && ts > 0) {
        if (startedAt === 0 || ts < startedAt) startedAt = ts;
        if (ts > endedAt) endedAt = ts;
      }
    });

    return { startedAt, endedAt };
  }

  extractRepoUrl() {
    // TODO: Implement with actual Claude Cloud DOM selectors.
    // Claude Cloud may show the linked GitHub repo in the project settings,
    // sidebar, or conversation metadata. Look for links to github.com/gitlab.com/etc.
    const repoLink = document.querySelector(
      'a[href*="github.com/"], a[href*="gitlab.com/"], a[href*="bitbucket.org/"]'
    );
    if (repoLink) {
      return this._normalizeRepoUrl(repoLink.href);
    }

    // Fallback: check for repo name in project metadata or breadcrumbs.
    const metaEls = document.querySelectorAll(
      '[data-testid="project-repo"], [class*="repo"], [class*="repository"]'
    );
    for (const el of metaEls) {
      const text = (el.textContent || '').trim();
      // Match patterns like "owner/repo" that look like GitHub paths.
      const match = text.match(/^([a-zA-Z0-9_.-]+\/[a-zA-Z0-9_.-]+)$/);
      if (match) {
        return 'github.com/' + match[1];
      }
    }

    return null;
  }

  _normalizeRepoUrl(url) {
    try {
      const parsed = new URL(url);
      // Extract "host/owner/repo" from the URL, stripping .git suffix and extra paths.
      const pathParts = parsed.pathname.replace(/\.git$/, '').split('/').filter(Boolean);
      if (pathParts.length >= 2) {
        return parsed.hostname + '/' + pathParts[0] + '/' + pathParts[1];
      }
    } catch (e) {
      // Not a valid URL.
    }
    return null;
  }

  isPageReady() {
    if (document.readyState !== 'complete') return false;
    // TODO: Add a check for Claude-specific content loading indicators.
    // For now, check if there's at least one message-like element.
    const hasMessages = document.querySelectorAll('[data-testid="message"], .message-container, [class*="Message"]').length > 0;
    return hasMessages;
  }

  _detectRole(el) {
    // TODO: Implement actual role detection based on Claude Cloud DOM structure.
    // Placeholder heuristics:
    const classes = el.className || '';
    const testId = el.getAttribute('data-testid') || '';

    if (classes.includes('human') || classes.includes('user') || testId.includes('user') || testId.includes('human')) {
      return 'user';
    }
    return 'agent';
  }

  _extractContent(el) {
    // TODO: Implement actual content extraction.
    // Placeholder: get text content, stripping UI chrome.
    const contentEl = el.querySelector('[data-testid="message-content"], .message-content, .prose') || el;
    return (contentEl.textContent || '').trim();
  }

  _extractTimestamp(el, index) {
    // TODO: Implement actual timestamp extraction.
    // Placeholder: look for time elements or use index-based ordering.
    const timeEl = el.querySelector('time, [datetime]');
    if (timeEl) {
      const ts = new Date(timeEl.getAttribute('datetime') || timeEl.textContent).getTime();
      if (!isNaN(ts) && ts > 0) return ts;
    }
    // Fallback: use current time minus offset based on position.
    return Date.now() - (1000 * (1000 - index));
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { ClaudeCloudAgent };
}
