# TODO

- [ ] Add a `/local/settings` route page
- [ ] Implement a list of projects in Settings, with a checkbox for each indicating if it's tracked (checked means tracked) or ignored. Remove the ignored ones from the `

- [ ] Support cloud AI pull requests
- [ ] Improve design of conversation header
/local` route homepage
- [ ] In split view of Agent Conversations and Git Commits, highlight related items in the opposite column on hover
- [ ] Show branch name for each commit in the list on Projects page
- [ ] Are we incorrectly detecting some diffs, like the last diff on [ba9dc20a-7886-4abf-9fec-6101551c8d03](http://localhost:5173/local/projects/oiSyQa5jX3iGHhcaykB-5/conversations/ba9dc20a-7886-4abf-9fec-6101551c8d03), which appears to be replacing the full file instead of doing a diff. Should we do a diff ourselves, by importing the `[file-history-snapshot]` that is logged just before it?

## Later

- [ ] In conversations detail page, maybe show the diff detail in a right side pane, if the window is wide enough?
- [ ] Manually correct a commit's agent percentage
- [ ] Make a separate cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
- [ ] Add a plugin and server agent implementation for opencode
- [ ] Add a plugin and server agent implementation for VS Code and Cursor
- [ ] Analyze local git repo for commits and pull requests?
