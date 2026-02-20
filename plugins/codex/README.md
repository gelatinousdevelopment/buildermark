# zrate — Codex CLI Plugin

Rate conversations on a 0–5 scale. Ratings are sent to the local zrate server and viewable on the dashboard at http://localhost:7022.

## Install

Codex CLI discovers skills from `~/.codex/skills/` (user global) or `.agents/skills/` (repo level).

**Option 1: Symlink (recommended)**

```bash
mkdir -p ~/.codex/skills
ln -s /path/to/zrate/plugins/codex/skills/zrate ~/.codex/skills/zrate
```

**Option 2: Copy**

```bash
mkdir -p ~/.codex/skills
cp -r /path/to/zrate/plugins/codex/skills/zrate ~/.codex/skills/zrate
```

## Usage

```
$zrate
$zrate 4 Great help with refactoring
$zrate 5
$zrate 2 Got stuck on the wrong approach
```

If you omit the rating, the model will infer a 0-5 rating from the conversation.

## Prerequisites

The zrate server must be running:

```bash
cd web/server && go run .
```
