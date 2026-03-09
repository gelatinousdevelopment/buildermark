CREATE TABLE IF NOT EXISTS commits (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    commit_hash     TEXT NOT NULL,
    subject         TEXT NOT NULL DEFAULT '',
    author_name     TEXT NOT NULL DEFAULT '',
    author_email    TEXT NOT NULL DEFAULT '',
    authored_at     INTEGER NOT NULL DEFAULT 0,
    diff_content    TEXT NOT NULL DEFAULT '',
    lines_total     INTEGER NOT NULL DEFAULT 0,
    lines_from_agent INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, commit_hash)
);

CREATE INDEX IF NOT EXISTS idx_commits_project_id ON commits(project_id);
CREATE INDEX IF NOT EXISTS idx_commits_authored_at ON commits(authored_at);
