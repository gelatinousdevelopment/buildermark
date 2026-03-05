CREATE TABLE IF NOT EXISTS commit_conversation_links (
    commit_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    UNIQUE(commit_id, conversation_id)
);
CREATE INDEX IF NOT EXISTS idx_ccl_commit_id ON commit_conversation_links(commit_id);
CREATE INDEX IF NOT EXISTS idx_ccl_conversation_id ON commit_conversation_links(conversation_id);
