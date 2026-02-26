CREATE TABLE IF NOT EXISTS commit_agent_coverage (
    id TEXT PRIMARY KEY,
    commit_id TEXT NOT NULL,
    agent TEXT NOT NULL DEFAULT '',
    lines_from_agent INTEGER NOT NULL DEFAULT 0,
    chars_from_agent INTEGER NOT NULL DEFAULT 0,
    UNIQUE(commit_id, agent)
);
CREATE INDEX IF NOT EXISTS idx_cac_commit_id ON commit_agent_coverage(commit_id);
