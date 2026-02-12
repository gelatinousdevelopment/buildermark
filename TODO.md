# TODO

- [x] Save the model along with each conversation. Add a column to the conversation table for this and figure out how to extract the model name from each coding agent's tracking implementation in the @web/server code.
- [x] It appears that we should actually be saving the model name per-message, not per-conversation. Please change that and be thorough. I noticed that the codex model names are being captured, but the gemini and claude ones are not detected (at least in the historical scan that runs on startup). Fix those too after changing the model name to be per-message.
- [ ] Capture code diff outputs from all coding agents' logs and include them as a message entry in the right place in the conversation

## Later

- [ ] Make a separate cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
- [ ] Add a plugin and server agent implementation for opencode
