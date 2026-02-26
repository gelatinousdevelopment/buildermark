CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    path TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    agent TEXT NOT NULL DEFAULT 'claude'
);

CREATE TABLE IF NOT EXISTS turns (
    id TEXT PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    project_id TEXT NOT NULL REFERENCES projects(id),
    conversation_id TEXT NOT NULL REFERENCES conversations(id),
    role TEXT NOT NULL CHECK(role IN ('agent', 'user')),
    content TEXT NOT NULL DEFAULT '',
    raw_json TEXT NOT NULL DEFAULT '',
    UNIQUE(conversation_id, timestamp)
);
