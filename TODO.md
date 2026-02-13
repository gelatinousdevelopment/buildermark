# TODO

- [x] Capture code diff outputs from all coding agents' logs and include them as a message entry in the right place in the conversation. This may be difficult, but try hard and only do it if it's reliable.
- [x] In the diff message rows, show a summary of number of files edited, number of lines added, and number of lines removed. Show this in the header of the message, so it's visible when collapsed and expanded.
- [x] Verify that the new tests for each of the @web/server/internal/agent implementations looking for diffs are well-formed to match real logs from my computer's folders `~/.claude`, `~/.codex`, and `~/.gemini`. Do not edit any code right now, just check and tell me what you find.

## Later

- [ ] Make a separate cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
- [ ] Add a plugin and server agent implementation for opencode
