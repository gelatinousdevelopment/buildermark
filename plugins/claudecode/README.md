# zrate — Claude Code Plugin

Rate conversations on a 0–5 scale. Ratings are sent to the local zrate server and viewable on the dashboard at http://localhost:7022.

## Install

From the repo root, launch Claude Code with the plugin loaded:

```bash
claude --plugin-dir ./plugins/claudecode
```

The `/zrate` skill will be available as `/zrate:zrate` (or just `/zrate` if unambiguous).

## Usage

```
/zrate 4 Great help with refactoring
/zrate 5
/zrate 2 Got stuck on the wrong approach
```

## Prerequisites

The zrate server must be running:

```bash
cd web/server && go run .
```
