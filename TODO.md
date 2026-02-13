# TODO

- [ ] in @web/frontend conversation detail page, convert model analysis markdown to html for displaying

## Later

- [ ] Make a separate cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
- [ ] Add a plugin and server agent implementation for opencode

## Architecture

- extensions
- local-server (go binary): collects, displays, and forwards local user's data (self-updating, settings in web UI)
- team-server (docker container): receives data from local servers, displays
- cloud server (subscription): team server + extra features
- local-frontend
- team-frontend
