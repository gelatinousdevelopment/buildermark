# Buildermark Browser Extensions

Browser extensions that automatically import conversations from coding agent web interfaces (Claude Cloud, Codex Cloud) into your local Buildermark instance.

## Prerequisites

- Buildermark server running locally on port 7022
- ImageMagick (for manual icon generation)

## Building

Run the build script from the repo root:

```bash
./scripts/build-browser-extensions.sh
```

This copies shared code into each extension directory.

To regenerate extension icons manually:

```bash
./apps/browser_extensions/generate-icons.sh
```

## Testing

### Chrome

1. Open `chrome://extensions/`
2. Enable "Developer mode" (toggle in the top right)
3. Click "Load unpacked"
4. Select the `apps/browser_extensions/chrome/` directory
5. Navigate to a Claude Cloud conversation (e.g., `claude.ai/chat/<id>`)
6. The extension only runs on matching domains — no broad page access is required

### Firefox

1. Open `about:debugging#/runtime/this-firefox`
2. Click "Load Temporary Add-on..."
3. Select `apps/browser_extensions/firefox/manifest.json`
4. Navigate to a Claude Cloud or Codex Cloud conversation
5. Note: Temporary add-ons are removed when Firefox restarts

### Safari

Safari web extensions require an Xcode wrapper to load. For development testing:

1. Open Safari > Settings > Advanced, enable "Show features for web developers"
2. Enable Safari > Settings > Developer > "Allow unsigned extensions"
3. Use Xcode's "Safari Web Extension" converter to create a project from the extension:
   ```bash
   xcrun safari-web-extension-converter apps/browser_extensions/safari/ \
     --project-location /tmp/buildermark-safari-ext \
     --app-name "Buildermark Importer"
   ```
4. Open the generated Xcode project, build and run it
5. Enable the extension in Safari > Settings > Extensions
6. Navigate to a conversation page to test

## How It Works

1. Content scripts run only on specific domains (claude.ai, chatgpt.com/codex, codex.openai.com)
2. When a conversation URL is detected, the extension checks the Buildermark API to see if it's already been imported
3. If not imported, it waits for the page to load, extracts messages, and sends them to the local API
4. A status overlay appears in the top-right corner during import
5. The extension badge updates to show import status (checkmark = imported, ! = error)

## Permissions

The extensions request minimal permissions:

- **Content scripts** are scoped to specific domain patterns only (claude.ai, chatgpt.com/codex, codex.openai.com)
- **Host permissions** are limited to `localhost:7022` for the Buildermark API
- No `activeTab`, `tabs`, or broad host access is requested

## Project Matching

The extensions attempt to match web conversations to existing Buildermark projects by extracting repository URLs from the page (e.g., GitHub links) and matching them against the `remote` field in the projects table. If no match is found, conversations are stored under a "Web Imports" project.

## Adding a New Agent

1. Create a new file in `shared/agents/` extending `BaseAgent`
2. Implement `name`, `agentId`, `urlPattern`, and the extraction methods
3. Add the URL patterns to `content_scripts.matches` in each browser's `manifest.json`
4. Add the new JS file to `content_scripts.js` in each manifest
5. Register the agent in each browser's `content.js` with `registerAgent(new YourAgent())`
6. Run `./scripts/build-browser-extensions.sh` to copy shared code
