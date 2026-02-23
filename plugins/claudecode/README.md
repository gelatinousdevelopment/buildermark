# bbrate — Claude Code Plugin

Rate conversations on a 0–5 scale. Ratings are sent to the local Buildermark Local server and viewable on the dashboard at http://localhost:7022.

## Install

From the repo root, launch Claude Code with the plugin loaded:

```bash
claude --plugin-dir ./plugins/claudecode
```

The `/bbrate` skill will be available as `/bbrate:rate` (or just `/bbrate` if unambiguous).

## Usage

```
/bbrate
/bbrate 4 Great help with refactoring
/bbrate 5
/bbrate 2 Got stuck on the wrong approach
```

If you omit the rating, the model will infer a 0-5 rating from the conversation.

## Prerequisites

The Buildermark Local server must be running:

```bash
cd web/server && go run .
```
