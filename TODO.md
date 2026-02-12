# TODO

- [x] Change the prompt to the model in the plugin to request Prompt Suggestions (short bullet points, max 3) and Model Failures (short bullet points, max 3) instead of general analysis. The model should take both the user's rating (if set) and the user note (if set). If no note is present, then the rating should imply that it's how the user feels that the model performed, so if it's less than 5 stars, the model should explain what it should have done better. If the user rated it 5 stars, then there are likely no prompt suggestions and likely no model failures (unless the model feels strongly that something could be improved)... my point is that no bullet points or only one or two bullet points from the model is acceptable.
- [ ] Add a plugin for codex cli from openai that works similarly to the claudecode plugin. Analyze the claudecode plugin carefully and look up the documentation for codex cli plugins.
- [ ] Add a plugin for codex cli from openai that works similarly to the gemini plugin. Analyze the claudecode plugin carefully and look up the documentation for gemini cli plugins.
- [ ] Add plugin: opencode
- [ ] Save the model along with each conversation

## Later

- [ ] Should the note be more for the user's records or telling the model what went wrong? maybe it's for telling 
- [ ] Separate @web/cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
