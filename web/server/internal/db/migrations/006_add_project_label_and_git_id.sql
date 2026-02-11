ALTER TABLE projects ADD COLUMN label TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN git_id TEXT NOT NULL DEFAULT '';

-- Backfill label from last path component for existing rows.
UPDATE projects SET label = SUBSTR(path, LENGTH(RTRIM(path, REPLACE(path, '/', ''))) + 1)
WHERE label = '';
