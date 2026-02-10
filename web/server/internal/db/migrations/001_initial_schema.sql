CREATE TABLE IF NOT EXISTS ratings (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    rating INTEGER NOT NULL CHECK(rating >= 0 AND rating <= 5),
    note TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ratings_conversation_id ON ratings(conversation_id);
