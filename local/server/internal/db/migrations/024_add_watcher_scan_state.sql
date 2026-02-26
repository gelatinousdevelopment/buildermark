CREATE TABLE IF NOT EXISTS watcher_scan_state (
    agent TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    source_key TEXT NOT NULL,
    file_size INTEGER NOT NULL DEFAULT 0,
    file_mtime_ms INTEGER NOT NULL DEFAULT 0,
    file_offset INTEGER NOT NULL DEFAULT 0,
    state_json TEXT NOT NULL DEFAULT '',
    updated_at_ms INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(agent, source_kind, source_key)
);

CREATE INDEX IF NOT EXISTS idx_watcher_scan_state_agent_kind
    ON watcher_scan_state(agent, source_kind);
