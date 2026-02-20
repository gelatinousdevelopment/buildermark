CREATE TABLE IF NOT EXISTS commit_sync_state (
    project_id TEXT NOT NULL REFERENCES projects(id),
    branch_name TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'idle',
    latest_known_head_hash TEXT NOT NULL DEFAULT '',
    last_processed_head_hash TEXT NOT NULL DEFAULT '',
    estimated_total_commits INTEGER NOT NULL DEFAULT 0,
    last_started_at_ms INTEGER NOT NULL DEFAULT 0,
    last_finished_at_ms INTEGER NOT NULL DEFAULT 0,
    last_duration_ms INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(project_id, branch_name)
);

CREATE INDEX IF NOT EXISTS idx_messages_project_role_ts_id
    ON messages(project_id, role, timestamp, id);
