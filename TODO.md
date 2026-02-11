# TODO

- [ ] add a column to the projects table for "label" and default to the last component of the path
- [ ] is there an identifier in the .git folder of each project that would match across multiple users on a team? something like the first commit hash or something? Figure out what that should be and save that in a new column in the database ("git_id") when adding a project to the database (or if there is no .git folder yet, then check again for it later when a scan runs again)

## Later

- [ ] Separate @web/cloud project or maybe a subfolder in @web/server/cloud for stuff specific to our cloud implementation?
- [ ] Should we keep the personal tracking stuff totally clean and separate, then have the teams stuff in a separate folder?
- [ ] Split model feedback to prompt_suggestions and model_failures?
