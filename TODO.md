# TODO

- [x] Rename the "turns" table in the database to be "messages" rename anything that we call "turn" or "turns" across the codebase to match the "message" language
- [x] Add a plugin for codex cli from openai that works similarly to the claudecode plugin. Analyze the claudecode plugin carefully and look up the documentation for codex cli plugins.
- [x] Add a plugin and server agent implementation for gemini cli from google that works similarly to the claudecode and codex plugins. Analyze the claudecode and codex plugins carefully and look up the documentation for gemini cli plugins. Also analyze the `~/.gemini` folder on my computer to look at the logs and figure out how to connect them from the local server side after a rating is posted (note that claudecode and codex do this differently because of the vendor-specific folder and file structures).
- [ ] Save all messages/logs from all tracked projects for all agent-specific implementations (claudecode, codex, and gemini). Look through the `~/.claude`, `~/.codex`, and `~/.gemini` folders to be sure that we capture all of the messages/logs for each project and track by conversation. We're already capturing all user prompts, but we want to capture all of the data now. However, on the conversation detail page in the @web/frontend project, show the other non-user-prompt messages as boxes, but only a single line for each (perhaps with a label of the type or first part of the output or something), but they should be clickable to expand. Format the conversations detail page messages as markdown.
- [ ] Add a plugin and server agent implementation for opencode
- [ ] Save the model along with each conversation
- [ ] Is writing sql inline in go code the best practice way to do this in the go community? I want to make sure that this go code is really clean, secure, and won't get too much criticism from hacker news commenters.
- [ ]  All non-user and non-rating messages should be collapsed by default to only a single line of text for each (perhaps with a label of the type or first part of the output or something), but they should be clickable and expand to show the full message on click.

## Later

- [ ] Should the note be more for the user's records or telling the model what went wrong? maybe it's for telling 
- [ ] Separate @web/cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
