CREATE TABLE commits_new (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    commit_hash TEXT NOT NULL,
    subject TEXT NOT NULL DEFAULT '',
    user_name TEXT NOT NULL DEFAULT '',
    user_email TEXT NOT NULL DEFAULT '',
    authored_at INTEGER NOT NULL DEFAULT 0,
    diff_content TEXT NOT NULL DEFAULT '',
    lines_total INTEGER NOT NULL DEFAULT 0,
    lines_from_agent INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT 0,
    branch_name TEXT NOT NULL DEFAULT '',
    coverage_version INTEGER NOT NULL DEFAULT 0,
    lines_added INTEGER NOT NULL DEFAULT 0,
    lines_removed INTEGER NOT NULL DEFAULT 0,
    override_line_percent REAL DEFAULT NULL,
    needs_parent INTEGER NOT NULL DEFAULT 0,
    UNIQUE(project_id, commit_hash)
);

INSERT INTO commits_new (
    id, project_id, commit_hash, subject, user_name, user_email,
    authored_at, diff_content, lines_total, lines_from_agent,
    created_at, branch_name, coverage_version, lines_added, lines_removed,
    override_line_percent, needs_parent
)
SELECT
    c.id, c.project_id, c.commit_hash, c.subject, c.user_name, c.user_email,
    c.authored_at, c.diff_content, c.lines_total, c.lines_from_agent,
    c.created_at, c.branch_name, c.coverage_version, c.lines_added, c.lines_removed,
    c.override_line_percent, c.needs_parent
FROM commits c
JOIN projects p ON p.id = c.project_id;

CREATE TABLE commit_agent_coverage_new (
    id TEXT PRIMARY KEY,
    commit_id TEXT NOT NULL,
    agent TEXT NOT NULL DEFAULT '',
    lines_from_agent INTEGER NOT NULL DEFAULT 0,
    UNIQUE(commit_id, agent)
);

INSERT INTO commit_agent_coverage_new (id, commit_id, agent, lines_from_agent)
SELECT cac.id, cac.commit_id, cac.agent, cac.lines_from_agent
FROM commit_agent_coverage cac
JOIN commits_new c ON c.id = cac.commit_id;

DELETE FROM commit_conversation_links
WHERE commit_id NOT IN (SELECT id FROM commits_new);

DROP TABLE commit_agent_coverage;
DROP TABLE commits;

ALTER TABLE commits_new RENAME TO commits;
ALTER TABLE commit_agent_coverage_new RENAME TO commit_agent_coverage;

CREATE INDEX IF NOT EXISTS idx_commits_authored_at ON commits(authored_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_commits_project_branch_hash
    ON commits(project_id, branch_name, commit_hash);
CREATE INDEX IF NOT EXISTS idx_commits_project_branch_authored
    ON commits(project_id, branch_name, authored_at DESC);
CREATE INDEX IF NOT EXISTS idx_cac_commit_id ON commit_agent_coverage(commit_id);
