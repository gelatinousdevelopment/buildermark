# rate-buildermark — Claude Code Skill

Rate conversations on a 0–5 scale. Ratings are sent to the local Buildermark Local server and viewable on the dashboard at http://localhost:55022.

## Install

Install via the Buildermark plugins page, or manually copy the skill to `~/.claude/skills/rate-buildermark/`.

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
