ALTER TABLE projects ADD COLUMN default_branch TEXT NOT NULL DEFAULT '';
ALTER TABLE commits ADD COLUMN branch_name TEXT NOT NULL DEFAULT '';

UPDATE commits SET branch_name = 'main' WHERE branch_name = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_commits_project_branch_hash
    ON commits(project_id, branch_name, commit_hash);

DROP INDEX IF EXISTS idx_commits_project_id;
CREATE INDEX IF NOT EXISTS idx_commits_project_branch_authored
    ON commits(project_id, branch_name, authored_at DESC);
