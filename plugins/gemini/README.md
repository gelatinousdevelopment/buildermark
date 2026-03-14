# rate-buildermark - Gemini CLI Plugin

Rate conversations on a 0-5 scale. Ratings are sent to the Buildermark Local server and viewable on the dashboard at http://localhost:55022.

## Install

Gemini CLI discovers custom commands from `~/.gemini/commands/` (user global) or `<project>/.gemini/commands/` (repo level).

**Option 1: Symlink (recommended)**

```bash
mkdir -p ~/.gemini/commands
ln -s ~/github/buildermark/plugins/gemini/commands/rate-buildermark.toml ~/.gemini/commands/rate-buildermark.toml
```

**Option 2: Copy**

```bash
mkdir -p ~/.gemini/commands
cp /path/to/buildermark/plugins/gemini/commands/rate-buildermark.toml ~/.gemini/commands/rate-buildermark.toml
```

## Usage

```text
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
