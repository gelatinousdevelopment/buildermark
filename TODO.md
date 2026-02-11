# TODO

- [x] The `history.jsonl` tracking doesn't seem to import the first user prompt/turn of each conversation. Investigate and fix. You can analyze the recent lines in my current `~/.claude/history.jsonl` file to compare against what's in the turns table in the `.data/zrate.db` sqlite database.
- [x] Fix the sorting of turns from the API... they should be sorted by most recent timestamp.
- [x] Some ratings are still not showing on the dashboard and, consequently, the conversations are not sorted properly. For instance, the "21f962cd-190a-4f5a-9b41-856587181c35" conversation should be first and show the one rating that it has (shows on the detail page). Fix it.
- [x] In the @web/server/internal/agent/claude implementation, figure out how to find claude's title for each session/conversation, then add that to the conversations table in the database (also add a string column for it)
- [x] Fix bug: `b8c1ca68-d072-41dc-b278-1576a25d081e` was incorrectly saved as a unique conversation and project, but it should have been on the `21f962cd-190a-4f5a-9b41-856587181c35` conversation. Figure out why this happened and fix it so this kind of bug doesn't happen again.
- [x] Reorganize the conversations detail page in @web/frontend to interleave the ratings in with the turns instead of having them separate. Replace the user turns that start with /zrate with the matching rating object, of course. Also, sort this list by oldest first.

## Later

- [ ] Separate @web/cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Split model feedback to prompt_suggestions and model_failures
