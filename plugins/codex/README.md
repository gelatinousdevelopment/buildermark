# bbrate — Codex CLI Plugin

Rate conversations on a 0–5 scale. Ratings are sent to the Buildermark Local server and viewable on the dashboard at http://localhost:7022.

## Install

Codex CLI discovers skills from `~/.codex/skills/` (user global) or `.agents/skills/` (repo level).

**Option 1: Symlink (recommended)**

```bash
mkdir -p ~/.codex/skills
ln -s /path/to/buildermark/plugins/codex/skills/bbrate ~/.codex/skills/bbrate
```

**Option 2: Copy**

```bash
mkdir -p ~/.codex/skills
cp -r /path/to/buildermark/plugins/codex/skills/bbrate ~/.codex/skills/bbrate
```

## Usage

```
$bbrate
$bbrate 4 Great help with refactoring
$bbrate 5
$bbrate 2 Got stuck on the wrong approach
```

If you omit the rating, the model will infer a 0-5 rating from the conversation.

## Prerequisites

The Buildermark Local server must be running:

```bash
cd web/server && go run .
```
