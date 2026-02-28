ALTER TABLE conversations ADD COLUMN user_prompt_count INTEGER NOT NULL DEFAULT 0;

UPDATE conversations SET user_prompt_count = (
  SELECT COUNT(*) FROM messages
  WHERE messages.conversation_id = conversations.id AND messages.role = 'user'
);
