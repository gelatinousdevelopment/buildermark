# rate-buildermark — Cursor IDE Plugin

Rate conversations on a 0–5 scale. Ratings are sent to the Buildermark Local server and viewable on the dashboard at http://localhost:55022.

## Install

Cursor discovers skills from `~/.cursor/skills/` (user global).

**Option 1: Symlink (recommended)**

```bash
mkdir -p ~/.cursor/skills
ln -s /path/to/buildermark/plugins/cursor/skills/rate-buildermark ~/.cursor/skills/rate-buildermark
```

**Option 2: Copy**

```bash
mkdir -p ~/.cursor/skills
cp -r /path/to/buildermark/plugins/cursor/skills/rate-buildermark ~/.cursor/skills/rate-buildermark
```

## Usage

```
/rate-buildermark
/rate-buildermark 4 Great help with refactoring
/rate-buildermark 5
/rate-buildermark 2 Got stuck on the wrong approach
```

If you omit the rating, the model will infer a 0-5 rating from the conversation.

## Prerequisites

The Buildermark Local server must be running:

```bash
cd web/server && go run .
```
