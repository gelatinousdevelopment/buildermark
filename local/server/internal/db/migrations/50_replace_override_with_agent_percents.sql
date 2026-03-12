-- Replace single override_line_percent with per-agent JSON override_agent_percents.
-- Requires SQLite 3.35+ for ALTER TABLE DROP COLUMN.
ALTER TABLE commits ADD COLUMN override_agent_percents TEXT DEFAULT NULL;
ALTER TABLE commits DROP COLUMN override_line_percent;
