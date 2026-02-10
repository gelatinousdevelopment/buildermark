---
name: zrate
description: Rate this Claude Code conversation (0-5 scale)
argument-hint: <0-5> [note]
allowed-tools: ["Bash"]
---

The user wants to rate this conversation.

Parse `$ARGUMENTS`: the first word is the rating (0–5), everything after is an optional note.

If no arguments were provided, ask the user: "How would you rate this conversation? (0-5, with an optional note)"

Otherwise, run the submission script:

```bash
bash plugins/claudecode/skills/zrate/scripts/submit-rating.sh $ARGUMENTS
```

If the output starts with "ok", briefly confirm to the user: **Rated N/5** (include the note if one was given).

If the output starts with "error", relay the message to the user. If it's a connection error, suggest starting the server with `cd web/server && go run .`
