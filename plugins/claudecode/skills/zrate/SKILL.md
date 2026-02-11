---
name: zrate
description: Rate this Claude Code conversation (0-5 scale)
argument-hint: <0-5> [note]
allowed-tools: ["Bash"]
---

The user wants to rate this conversation.

Parse `$ARGUMENTS`: the first word is the rating (0–5), everything after is an optional note.

If no arguments were provided, ask the user: "How would you rate this conversation? (0-5, with an optional note)"

Otherwise, before submitting, analyze the conversation in light of the rating and note. The user may be rating the prompt, the model's output, or both — the note will often clarify. Write a short analysis (no more than 2 sentences or a short bulleted list) that is technical and dry with no personality. The analysis may include:

- What went well or poorly in the interaction
- Whether the original prompt could have been clearer or more specific
- Whether the model should have asked clarifying questions, chosen a different approach, or known better given available context

Never be snarky or arrogant.

Then run the submission script, passing your analysis text in the `ANALYSIS` environment variable:

```bash
ANALYSIS="your analysis text here" bash plugins/claudecode/skills/zrate/scripts/submit-rating.sh $ARGUMENTS
```

If the output starts with "ok", confirm to the user: **Rated N/5** (include the note if one was given), then show your analysis under a `**Analysis:**` heading.

If the output starts with "error", relay the message to the user. If it's a connection error, suggest starting the server with `cd web/server && go run .`
