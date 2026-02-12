# TODO

- [x] Save all messages/logs from all tracked projects for all agent-specific implementations (claudecode, codex, and gemini). Look through the `~/.claude`, `~/.codex`, and `~/.gemini` folders to be sure that we capture all of the messages/logs for each project and track by conversation. We're already capturing all user prompts, but we want to capture all of the data now. However, on the conversation detail page in the @web/frontend project, show the other non-user-prompt messages as boxes, but only a single line for each (perhaps with a label of the type or first part of the output or something), but they should be clickable to expand. Format the conversations detail page messages as markdown.
- [ ] Fix bug: the user prompts are not being captured and saved (or at least are not displayed) for codex conversations. See this example conversation id: `6db1e563-2391-4db4-9fbe-60c7f14c4dc5`. The first user message is actually just a codex internal system prompt, not the user's actual prompt, which started with "Save all messages/logs from all tracked projects for all agent-specific implementations..."
- [ ] Save the model along with each conversation
- [ ] Add a plugin and server agent implementation for opencode

## Later

- [ ] Make a separate cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
