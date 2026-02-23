# bb — Claude Code Plugin

Rate conversations on a 0–5 scale. Ratings are sent to the local zrate server and viewable on the dashboard at http://localhost:7022.

## Install

From the repo root, launch Claude Code with the plugin loaded:

```bash
claude --plugin-dir ./plugins/claudecode
```

The `/bb` skill will be available as `/bb:rate` (or just `/bb` if unambiguous).

## Usage

```
/bb
/bb 4 Great help with refactoring
/bb 5
/bb 2 Got stuck on the wrong approach
```

If you omit the rating, the model will infer a 0-5 rating from the conversation.

## Prerequisites

The zrate server must be running:

```bash
cd web/server && go run .
```
