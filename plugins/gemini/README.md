# zrate - Gemini CLI Plugin

Rate conversations on a 0-5 scale. Ratings are sent to the local zrate server and viewable on the dashboard at http://localhost:7022.

## Install

Gemini CLI discovers custom commands from `~/.gemini/commands/` (user global) or `<project>/.gemini/commands/` (repo level).

**Option 1: Symlink (recommended)**

```bash
mkdir -p ~/.gemini/commands
ln -s ~/github/zrate/plugins/gemini/commands/zrate.toml ~/.gemini/commands/zrate.toml
```

**Option 2: Copy**

```bash
mkdir -p ~/.gemini/commands
cp /path/to/zrate/plugins/gemini/commands/zrate.toml ~/.gemini/commands/zrate.toml
```

## Usage

```text
/zrate 4 Great help with refactoring
/zrate 5
/zrate 2 Got stuck on the wrong approach
```

## Prerequisites

The zrate server must be running:

```bash
cd web/server && go run .
```
