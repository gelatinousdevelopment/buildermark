ALTER TABLE conversations ADD COLUMN started_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE conversations ADD COLUMN ended_at INTEGER NOT NULL DEFAULT 0;

UPDATE conversations
SET
    started_at = COALESCE((SELECT MIN(timestamp) FROM messages WHERE conversation_id = conversations.id), 0),
    ended_at = COALESCE((SELECT MAX(timestamp) FROM messages WHERE conversation_id = conversations.id), 0);
