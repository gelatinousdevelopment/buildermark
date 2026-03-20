CREATE TABLE IF NOT EXISTS import_logs (
    id TEXT PRIMARY KEY,
    type TEXT,
    source TEXT,
    source_id TEXT NOT NULL DEFAULT '',
    timestamp INTEGER NOT NULL DEFAULT 0,
    content TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_import_logs_timestamp ON import_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_import_logs_lookup ON import_logs(type, source, source_id, timestamp);
