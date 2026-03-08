/**
 * Core import orchestrator used by all browser extensions.
 * Coordinates agent detection, API checks, data extraction, and UI overlay.
 */

// Registry of all available agents.
const AGENTS = [];

function registerAgent(agent) {
  AGENTS.push(agent);
}

/**
 * Find the matching agent for the current URL.
 * @param {string} url
 * @returns {{ agent: BaseAgent, conversationId: string } | null}
 */
function findMatchingAgent(url) {
  for (const agent of AGENTS) {
    const result = agent.matchUrl(url);
    if (result.match && result.conversationId) {
      return { agent, conversationId: result.conversationId };
    }
  }
  return null;
}

/**
 * Create and manage the status overlay in the top-right corner.
 */
function createOverlay() {
  const existing = document.getElementById('buildermark-import-overlay');
  if (existing) existing.remove();

  const overlay = document.createElement('div');
  overlay.id = 'buildermark-import-overlay';
  overlay.style.cssText = `
    position: fixed;
    top: 16px;
    right: 16px;
    z-index: 2147483647;
    background: #1a1a2e;
    color: #e0e0e0;
    padding: 12px 20px;
    border-radius: 8px;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    font-size: 14px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    border: 1px solid #333;
    transition: opacity 0.3s ease;
    pointer-events: none;
  `;
  document.body.appendChild(overlay);

  return {
    show(text, color) {
      overlay.textContent = text;
      if (color) {
        overlay.style.borderColor = color;
      }
      overlay.style.opacity = '1';
      overlay.style.display = 'block';
    },
    hide() {
      overlay.style.opacity = '0';
      setTimeout(() => {
        overlay.style.display = 'none';
      }, 300);
    },
    remove() {
      overlay.remove();
    },
  };
}

/**
 * Wait for the page to be ready according to the agent's readiness check.
 * @param {BaseAgent} agent
 * @param {number} timeoutMs
 * @returns {Promise<boolean>}
 */
function waitForPageReady(agent, timeoutMs = 15000) {
  return new Promise(resolve => {
    if (agent.isPageReady()) {
      resolve(true);
      return;
    }

    const start = Date.now();
    const interval = setInterval(() => {
      if (agent.isPageReady()) {
        clearInterval(interval);
        resolve(true);
      } else if (Date.now() - start > timeoutMs) {
        clearInterval(interval);
        resolve(false);
      }
    }, 500);
  });
}

/**
 * Run the import flow for the current page.
 * Called by the content script when a matching URL is detected.
 *
 * @param {Function} setBadge - Function to update the extension badge/icon.
 *   Called with 'importing', 'done', 'already', or 'error'.
 */
async function runImport(setBadge) {
  const url = window.location.href;
  const match = findMatchingAgent(url);
  if (!match) return;

  const { agent, conversationId } = match;

  try {
    // Check if already imported.
    const checkResult = await BuildermarkAPI.checkUrl(url);
    if (checkResult.imported) {
      if (setBadge) setBadge('already');
      return;
    }
  } catch (err) {
    // Server might not be running — silently skip.
    console.log('[Buildermark] Server not reachable, skipping import check:', err.message);
    return;
  }

  // Wait for page content to load.
  const ready = await waitForPageReady(agent);
  if (!ready) {
    console.log('[Buildermark] Page did not become ready in time, skipping import.');
    return;
  }

  const overlay = createOverlay();
  overlay.show('Buildermark: Importing conversation...', '#4a9eff');
  if (setBadge) setBadge('importing');

  try {
    const title = agent.extractTitle();
    const messages = agent.extractMessages();
    const dates = agent.extractDates();
    const repoUrl = agent.extractRepoUrl();

    if (messages.length === 0) {
      overlay.show('Buildermark: No messages found to import.', '#ff6b6b');
      if (setBadge) setBadge('error');
      setTimeout(() => overlay.hide(), 5000);
      return;
    }

    const params = {
      url,
      agent: agent.agentId,
      title,
      startedAt: dates.startedAt,
      endedAt: dates.endedAt,
      messages,
    };
    if (repoUrl) {
      params.repoUrl = repoUrl;
    }

    const result = await BuildermarkAPI.importConversation(params);

    if (result.alreadyExisted) {
      overlay.show('Buildermark: Already imported.', '#4ecdc4');
    } else {
      overlay.show(`Buildermark: Imported ${result.messageCount} messages.`, '#4ecdc4');
    }
    if (setBadge) setBadge('done');
    setTimeout(() => overlay.hide(), 5000);
  } catch (err) {
    console.error('[Buildermark] Import failed:', err);
    overlay.show('Buildermark: Import failed.', '#ff6b6b');
    if (setBadge) setBadge('error');
    setTimeout(() => overlay.hide(), 5000);
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { registerAgent, findMatchingAgent, runImport, AGENTS };
}
