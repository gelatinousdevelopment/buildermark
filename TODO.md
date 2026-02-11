# TODO

- [x] add a column to the projects table for "label" and default to the last component of the path
- [x] is there an identifier in the .git folder of each project that would match across multiple users on a team? something like the first commit hash or something? Figure out what that should be and save that in a new column in the database ("git_id") when adding a project to the database (or if there is no .git folder yet, then check again for it later when a scan runs again)
- [x] Add UI to rate conversations, inserting the rating after any user turn (prompt) in the conversations detail page @web/frontend/src/routes/dashboard/conversations/[id] file. You should be able to reuse the existing POST rating API endpoint, although you won't have the model feedback part, of course.
- [ ] Make sure you don't insert multiple entries in turns for the same thing, which is happening sometimes. For instance, in project "6db1e563-2391-4db4-9fbe-60c7f14c4dc5", the two user entries are the same text, but just a slightly different timestamp... figure out why that happened and implement a fix to prevent this bug in the future. Do it in a clean and best-practice way.
- [ ] Add plugin: codex
- [ ] Add plugin: gemini
- [ ] Add plugin: opencode

## Later

- [ ] Separate @web/cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Split model feedback to prompt_suggestions and model_failures?
