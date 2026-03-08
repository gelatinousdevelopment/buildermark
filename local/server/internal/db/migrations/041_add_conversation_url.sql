ALTER TABLE conversations ADD COLUMN url TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_conversations_url ON conversations(url);
