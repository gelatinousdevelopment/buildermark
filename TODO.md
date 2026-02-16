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
- [x] From the conversation detail page, move the message content of each different message type into a separate svelte component, similar to how DiffMessageCard is a component, except we want the shell `<div class="message">`, `<div class="rating-card">`, `<div class="message log-group">`, etc tag in the +page.svelte file (including for the DiffMessageCard too, so move some of that outer styling up to the +page.svelte), so we can handle styling of that outer tag there. Move the onclick for toggling expansion up to this top level div as well, so we manage that in the +page.svelte file.
- [x] Remove the inline add rating widget from the list of messages, but keep the full Add rating box at the bottom. However, for the Add rating box at the bottom, show it only if there isn't already a rating after the last user message.
- [x] Move some of the message filtering to go, but keep the log-group calculation in svelte. Specifically, move: rating matching, message.content.trim() != '', message.content.trim() != '[user]', and !message.content.trim().startsWith('<command-message>').
- [x] Also hide user messages where the content is just `/clear` or `/new`
- [x] Also filter out user messages that include the text like `[Pasted text #1 ...]`, but do a regex to allow for the number to change and any text where I wrote `...`. This is because the next user message has the full text expanded, so we don't need this ones that say pasted text.
- [x] In the DiffMessageCard.svelte when there is only one file, hide the summary's DiffCount component
- [x] Add "Expand All" button next to "Matched Messages" to expand all of the diffs on the commit detail page
- [ ] Show the agent and model in the DiffMessageCard header
- [x] Add time length of model/agent messages to "6 logs from agent", like "6 logs from agent over 30 seconds"
- [x] The "Matched Messages" "Expand All" button should apply only to the Matched Messages list, not the stuff above it

- [ ] Are we incorrectly detecting some diffs, like the last diff on [ba9dc20a-7886-4abf-9fec-6101551c8d03](http://localhost:5173/local/projects/oiSyQa5jX3iGHhcaykB-5/conversations/ba9dc20a-7886-4abf-9fec-6101551c8d03), which appears to be replacing the full file instead of doing a diff. Should we do a diff ourselves, by importing the `[file-history-snapshot]` that is logged just before it?

- [ ] Improve design of conversation header
- [ ] Improve normalization of code (like code formatting) when comparing agent diffs to commit diffs
- [ ] Is the go process continuously monitoring conversation logs and importing? I think it's not, at least for claude.

- [ ] The mechanism in conversation.go to ignore user messages that start with "<command-name>" and other things doesn't seem to be working. For instance, message id `BecL-QoaOaq_V4F72FrRz` is appearing on http://localhost:5173/local/projects/oiSyQa5jX3iGHhcaykB-5/conversations/ba9dc20a-7886-4abf-9fec-6101551c8d03 when it shouldn't. Fix that filtering mechanism.

## Later

- [ ] Manually correct a commit's agent percentage
- [ ] Label lines in diff that were agent generated
- [ ] Maybe organize dashboard by Project > Commits (and Working Copy) > Conversations?
- [ ] Make a separate cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Re-run prompts (along with code state) against new models to find the best or cheapest (sample mostly the poor ratings?)
- [ ] Add a plugin and server agent implementation for opencode
- [ ] Add a plugin and server agent implementation for VS Code and Cursor
- [ ] Analyze local git repo for commits and pull requests?
