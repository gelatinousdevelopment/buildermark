# TODO

- [ ] Support more than just main branch
- [ ] Support cloud AI pull requests
- [x] Show color bar for agent percentage vs manual (or per-agent colors?)
- [x] Make the "3 logs from model" no border and gray text
- [x] Fix font sizes of markdown in messages
- [x] Hide user messages when content starts with "<command-message>"
- [x] If the user content of a message is empty, then just hide the message
- [x] Make the message diffs files list counts right aligned
- [x] Give hover state to clickable messages
- [ ] Move the rating widget somewhere else
- [ ] Improve design of conversation header

## Later

- [ ] Label lines in diff that were agent generated
- [ ] Maybe organize dashboard by Project > Commits (and Working Copy) > Conversations?
- [ ] Make a separate cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
- [ ] Add a plugin and server agent implementation for opencode
- [ ] Add a plugin and server agent implementation for VS Code and Cursor
- [ ] Analyze local git repo for commits and pull requests?
