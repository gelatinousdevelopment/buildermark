# TODO

- [x] In the DiffMessageCard.svelte, make the file name overflow with ellipsis truncation instead of making the whole row wider than the container when the text is long.
- [x] Add a date column to the conversations and a compact option. Use the non-compact version on the `/local/projects/[id]/conversations` route only, then compact everywhere else.
- [x] Change dates to be relative (with title tooltip for full date string) in both the Conversations.svelte and Commits.svelte components. Use short dates, when possible, but only have relative dates (like 1h ago) when less than 24 hours. When 24 hours or more, use short date syntax, like "Feb 3, 4:22pm" but make it localized to the browser's time, of course. Use the new built-in javascript API called Temporal (it's in all major browsers now).
- [x] On the conversations detail route, I've added a column on the right to display the details of a message, both for diff messages and messages from the agent/model. Implement this so that either a DiffMessageCard or a LogMessageCard gets displayed in that right pane. Note that this is in addition to it being displayed inline. However, we must use css to hide each one of them depending on the viewport width... I've already implemented this for the right column where the css says `@media (max-width: 1023px) {`, but we also need to hide the inline diff and agent message content when in the skinny condition (less than 1023). I should also be able to deselect a message by clicking anywhere on the background of the left column (that's not already a button/link).

- [ ] Summary agent percentage bar should actually be a stacked chart over time (day resolution), since an overall percentage isn't very useful due to old commits.
- [ ] Support cloud AI pull requests
- [ ] Improve design of conversation header
- [ ] In split view of Agent Conversations and Git Commits, highlight related items in the opposite column on hover
- [ ] Show branch name for each commit in the list on Projects page
- [ ] Don't use the AgentTag.svelte in the messages on the conversations detail page

## Later

- [ ] In conversations detail page, maybe show the diff detail in a right side pane, if the window is wide enough?
- [ ] Manually correct a commit's agent percentage
- [ ] Make a separate cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
- [ ] Add a plugin and server agent implementation for opencode
- [ ] Add a plugin and server agent implementation for VS Code and Cursor
