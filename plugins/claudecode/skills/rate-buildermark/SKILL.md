---
name: rate-buildermark
description: Rate this Claude Code conversation (0-5 scale)
argument-hint: "[0-5] [note]"
allowed-tools: ["Bash"]
---

The user wants to rate this conversation.

Parse `$ARGUMENTS`:
- If the first word is a rating (0–5), use it as the rating and treat everything after as an optional note.
- If no rating is provided (including no args or note-only args), infer the rating (0–5) from conversation quality and treat all provided args as an optional note.

Before submitting, review the conversation in light of the rating and optional note. Produce two sections:

**Prompt Suggestions** — short bullet points (max 3) on how the user's prompt could have been clearer or more effective.

**Model Failures** — short bullet points (max 3) on what the model did wrong or could have done better.

Guidelines:
- Weigh the rating (0–5) and optional note to calibrate your response
- If no note is present, the rating alone implies user sentiment — infer what went wrong from the conversation context
- If rating < 5 and no note: explain what the model should have done better
- If rating = 5: likely no suggestions and no failures, unless you genuinely identify something worth noting
- 0, 1, or 2 bullets per section is perfectly acceptable — do not force 3
- A section with no bullets should say "None."
- Keep the tone technical and dry, no personality, never snarky or arrogant

Then run the submission script, passing your analysis text in the `ANALYSIS` environment variable:

```bash
ANALYSIS="your analysis text here" bash "$(git rev-parse --show-toplevel)/plugins/claudecode/skills/rate-buildermark/scripts/submit-rating.sh" <rating> [note...]
```

If the output starts with "ok", confirm to the user: **Rated N/5** (include the note if one was given), print a clickable conversation link using the `conversation_url` value from script output, then show your analysis under `**Prompt Suggestions:**` and `**Model Failures:**` headings.

If the output starts with "error", relay the message to the user. If it's a connection error, suggest starting the server with `cd web/server && go run .`
