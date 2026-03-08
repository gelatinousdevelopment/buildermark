/**
 * Agent importer for Codex Cloud (OpenAI Codex web interface).
 */
class CodexCloudAgent extends BaseAgent {
  get name() {
    return 'Codex Cloud';
  }

  get agentId() {
    return 'codex_cloud';
  }

  get urlPattern() {
    // Matches URLs like https://chatgpt.com/codex/... or https://codex.openai.com/...
    // TODO: Update with actual Codex Cloud URL pattern once confirmed.
    return /(?:chatgpt\.com\/codex|codex\.openai\.com)\/(?:s\/)?([a-zA-Z0-9_-]+)/;
  }

  extractTitle() {
    // TODO: Implement with actual Codex Cloud DOM selectors.
    const titleEl = document.querySelector('[data-testid="conversation-title"], h1, .conversation-title, title');
    if (titleEl) {
      const text = titleEl.textContent.trim();
      return text.replace(/\s*[-–]\s*(?:Codex|ChatGPT)\s*$/, '');
    }
    return '';
  }

  extractMessages() {
    // TODO: Implement with actual Codex Cloud DOM selectors.
    const messages = [];

    const messageEls = document.querySelectorAll('[data-testid="message"], [data-message-id], .message, [class*="message"]');

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
    const dateEls = document.querySelectorAll('time, [datetime], [data-testid="message-timestamp"]');
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
    // TODO: Implement with actual Codex Cloud DOM selectors.
    // Codex tasks are typically linked to a GitHub repository. Look for repo links
    // or metadata in the task/conversation header area.
    const repoLink = document.querySelector(
      'a[href*="github.com/"], a[href*="gitlab.com/"], a[href*="bitbucket.org/"]'
    );
    if (repoLink) {
      return this._normalizeRepoUrl(repoLink.href);
    }

    // Fallback: look for repo name in task metadata.
    const metaEls = document.querySelectorAll(
      '[data-testid="task-repo"], [data-testid="repository"], [class*="repo"], [class*="repository"]'
    );
    for (const el of metaEls) {
      const text = (el.textContent || '').trim();
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
    // TODO: Add Codex-specific readiness check.
    const hasMessages = document.querySelectorAll('[data-testid="message"], [data-message-id], .message, [class*="message"]').length > 0;
    return hasMessages;
  }

  _detectRole(el) {
    // TODO: Implement actual role detection based on Codex Cloud DOM.
    const classes = el.className || '';
    const testId = el.getAttribute('data-testid') || '';
    const authorEl = el.querySelector('[data-message-author-role]');

    if (authorEl) {
      const authorRole = authorEl.getAttribute('data-message-author-role');
      if (authorRole === 'user') return 'user';
      return 'agent';
    }

    if (classes.includes('user') || testId.includes('user')) {
      return 'user';
    }
    return 'agent';
  }

  _extractContent(el) {
    // TODO: Implement actual content extraction.
    const contentEl = el.querySelector('.message-content, .markdown, .prose, [data-testid="message-content"]') || el;
    return (contentEl.textContent || '').trim();
  }

  _extractTimestamp(el, index) {
    const timeEl = el.querySelector('time, [datetime]');
    if (timeEl) {
      const ts = new Date(timeEl.getAttribute('datetime') || timeEl.textContent).getTime();
      if (!isNaN(ts) && ts > 0) return ts;
    }
    return Date.now() - (1000 * (1000 - index));
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { CodexCloudAgent };
}
