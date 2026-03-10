/**
 * Core import orchestrator used by all browser extensions.
 * Coordinates agent detection, API checks, data extraction, and UI overlay.
 */

// Registry of all available agents.
const AGENTS = [];

// Current page import state, queryable by the popup.
let _buildermarkPageState = "waiting";

function _setPageState(state) {
  console.log("setPageState", state);
  _buildermarkPageState = state;
  try {
    chrome.runtime.sendMessage({ type: "pageStateChanged", state });
  } catch {
    // Popup may not be open — ignore.
  }
}

function registerAgent(agent) {
  AGENTS.push(agent);
}

function formatImportErrorMessage(err) {
  const message = typeof err?.message === "string" ? err.message.trim() : "";
  if (!message) {
    return "Buildermark: Import failed.";
  }

  const normalized = message.replace(/\s+/g, " ");
  const trimmed = normalized.replace(/[.!\s]+$/, "");
  const displayMessage = trimmed.length > 160 ? `${trimmed.slice(0, 157)}...` : trimmed;
  return `Buildermark: Import failed: ${displayMessage}.`;
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
 * Wait for the page to be ready according to the agent's readiness check.
 * @param {BaseAgent} agent
 * @param {number} timeoutMs
 * @returns {Promise<boolean>}
 */
function waitForPageReady(agent, timeoutMs = 15000) {
  return new Promise((resolve) => {
    console.log("isPageReady", agent.isPageReady());
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
  if (!match) {
    _setPageState("ignored");
    setBadge("ignored");
    return;
  }

  const { agent, conversationId } = match;
  _setPageState("waiting");
  setBadge("waiting");

  try {
    // Check if already imported.
    const checkResult = await BuildermarkAPI.checkUrl(url);
    if (checkResult.imported) {
      _setPageState("already");
      if (setBadge) setBadge("already");
      return;
    }
  } catch (err) {
    // Server might not be running — silently skip.
    _setPageState("server_unavailable");
    console.log("[Buildermark] Server not reachable, skipping import check:", err.message);
    return;
  }

  // Wait for page content to load.
  const ready = await waitForPageReady(agent);
  if (!ready) {
    console.log("[Buildermark] Page did not become ready in time, skipping import.");
    return;
  }

  _setPageState("importing");
  if (setBadge) setBadge("importing");

  try {
    const title = agent.extractTitle();
    const messages = agent.extractMessages();
    const dates = agent.extractDates();
    const repoUrl = agent.extractRepoUrl();

    if (messages.length === 0) {
      overlay.show("Buildermark: No messages found to import.", "#ff6b6b");
      _setPageState("error");
      if (setBadge) setBadge("error");
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
      overlay.show("Buildermark: Already imported.", "#4ecdc4");
      _setPageState("already");
    } else {
      overlay.show(`Buildermark: Imported ${result.messageCount} messages.`, "#4ecdc4");
      _setPageState("done");
    }
    if (setBadge) setBadge("done");
    setTimeout(() => overlay.hide(), 5000);
  } catch (err) {
    console.error("[Buildermark] Import failed:", err);
    overlay.show(formatImportErrorMessage(err), "#ff6b6b");
    _setPageState("error");
    if (setBadge) setBadge("error");
    setTimeout(() => overlay.hide(), 5000);
  }
}

// Listen for state queries from the popup.
if (typeof chrome !== "undefined" && chrome.runtime && chrome.runtime.onMessage) {
  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message.type === "getPageState") {
      sendResponse({ state: _buildermarkPageState });
    }
  });
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { registerAgent, findMatchingAgent, runImport, AGENTS };
}
