-- Normalize all datetime-like columns to unix milliseconds (INTEGER).

PRAGMA foreign_keys=off;

-- ratings.created_at: DATETIME -> INTEGER (unix ms)
ALTER TABLE ratings RENAME TO ratings_old;
CREATE TABLE ratings (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    rating INTEGER NOT NULL CHECK(rating >= 0 AND rating <= 5),
    note TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT 0,
    analysis TEXT NOT NULL DEFAULT '',
    temp_conversation_id TEXT NOT NULL DEFAULT ''
);
INSERT INTO ratings (id, conversation_id, rating, note, created_at, analysis, temp_conversation_id)
SELECT
    id,
    conversation_id,
    rating,
    note,
    CASE
        WHEN created_at IS NULL THEN 0
        WHEN typeof(created_at) IN ('integer', 'real') THEN CAST(created_at AS INTEGER)
        WHEN CAST(created_at AS TEXT) GLOB '[0-9]*' AND CAST(created_at AS TEXT) NOT LIKE '%-%' AND CAST(created_at AS TEXT) NOT LIKE '%:%' THEN CAST(created_at AS INTEGER)
        ELSE CAST((julianday(created_at) - 2440587.5) * 86400000 AS INTEGER)
    END,
    analysis,
    temp_conversation_id
FROM ratings_old;
DROP TABLE ratings_old;
CREATE INDEX IF NOT EXISTS idx_ratings_conversation_id ON ratings(conversation_id);
CREATE INDEX IF NOT EXISTS idx_ratings_temp_conversation_id ON ratings(temp_conversation_id);

-- commits.created_at: DATETIME -> INTEGER (unix ms)
ALTER TABLE commits RENAME TO commits_old;
CREATE TABLE commits (
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
    UNIQUE(project_id, commit_hash)
);
INSERT INTO commits (
    id, project_id, commit_hash, subject, user_name, user_email,
    authored_at, diff_content, lines_total, lines_from_agent,
    created_at, branch_name, coverage_version, lines_added, lines_removed
)
SELECT
    id, project_id, commit_hash, subject, user_name, user_email,
    authored_at, diff_content, lines_total, lines_from_agent,
    CASE
        WHEN created_at IS NULL THEN 0
        WHEN typeof(created_at) IN ('integer', 'real') THEN CAST(created_at AS INTEGER)
        WHEN CAST(created_at AS TEXT) GLOB '[0-9]*' AND CAST(created_at AS TEXT) NOT LIKE '%-%' AND CAST(created_at AS TEXT) NOT LIKE '%:%' THEN CAST(created_at AS INTEGER)
        ELSE CAST((julianday(created_at) - 2440587.5) * 86400000 AS INTEGER)
    END,
    branch_name, coverage_version, lines_added, lines_removed
FROM commits_old;
DROP TABLE commits_old;
CREATE INDEX IF NOT EXISTS idx_commits_authored_at ON commits(authored_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_commits_project_branch_hash
    ON commits(project_id, branch_name, commit_hash);
CREATE INDEX IF NOT EXISTS idx_commits_project_branch_authored
    ON commits(project_id, branch_name, authored_at DESC);

-- schema_version.applied_at: DATETIME -> INTEGER (unix ms)
ALTER TABLE schema_version RENAME TO schema_version_old;
CREATE TABLE schema_version (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL DEFAULT 0
);
INSERT INTO schema_version (version, applied_at)
SELECT
    version,
    CASE
        WHEN applied_at IS NULL THEN 0
        WHEN typeof(applied_at) IN ('integer', 'real') THEN CAST(applied_at AS INTEGER)
        WHEN CAST(applied_at AS TEXT) GLOB '[0-9]*' AND CAST(applied_at AS TEXT) NOT LIKE '%-%' AND CAST(applied_at AS TEXT) NOT LIKE '%:%' THEN CAST(applied_at AS INTEGER)
        ELSE CAST((julianday(applied_at) - 2440587.5) * 86400000 AS INTEGER)
    END
FROM schema_version_old;
DROP TABLE schema_version_old;

PRAGMA foreign_keys=on;
