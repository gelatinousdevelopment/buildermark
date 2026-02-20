ALTER TABLE ratings ADD COLUMN temp_conversation_id TEXT NOT NULL DEFAULT '';

UPDATE ratings
SET temp_conversation_id = conversation_id
WHERE temp_conversation_id = '';

CREATE INDEX IF NOT EXISTS idx_ratings_temp_conversation_id ON ratings(temp_conversation_id);
