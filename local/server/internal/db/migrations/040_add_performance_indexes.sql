-- Drop redundant indexes
DROP INDEX IF EXISTS idx_commits_project_id;
DROP INDEX IF EXISTS idx_commits_authored_at;
DROP INDEX IF EXISTS idx_messages_project_role_ts_id;

-- conversations table
CREATE INDEX IF NOT EXISTS idx_conversations_project_id_hidden ON conversations(project_id, hidden);
CREATE INDEX IF NOT EXISTS idx_conversations_parent_conversation_id ON conversations(parent_conversation_id);
CREATE INDEX IF NOT EXISTS idx_conversations_agent_title ON conversations(agent, title);

-- messages table
CREATE INDEX IF NOT EXISTS idx_messages_conversation_id_timestamp ON messages(conversation_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_id_message_type ON messages(conversation_id, message_type);
CREATE INDEX IF NOT EXISTS idx_messages_project_id ON messages(project_id);

-- ratings table
CREATE INDEX IF NOT EXISTS idx_ratings_created_at ON ratings(created_at);

-- projects table
CREATE INDEX IF NOT EXISTS idx_projects_path ON projects(path);
