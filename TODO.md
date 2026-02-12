# TODO

- [x] Rename the "turns" table in the database to be "messages" rename anything that we call "turn" or "turns" across the codebase to match the "message" language
- [x] Add a plugin for codex cli from openai that works similarly to the claudecode plugin. Analyze the claudecode plugin carefully and look up the documentation for codex cli plugins.
- [ ] Add a plugin and server agent implementation for gemini cli from google that works similarly to the claudecode plugin. Analyze the claudecode plugin carefully and look up the documentation for gemini cli plugins.
- [ ] Add plugin and server agent implementation for opencode cli that works similarly to the claudecode plugin. Analyze the claudecode plugin carefully and look up the documentation for opencode cli plugins.
- [ ] Save the model along with each conversation

## Later

- [ ] Should the note be more for the user's records or telling the model what went wrong? maybe it's for telling 
- [ ] Separate @web/cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
