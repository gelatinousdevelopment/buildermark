ALTER TABLE messages ADD COLUMN message_type TEXT NOT NULL DEFAULT 'log';

UPDATE messages
SET message_type = 'prompt'
WHERE role = 'user'
  AND TRIM(content) NOT LIKE '/%'
  AND TRIM(content) NOT LIKE '$bb%';

UPDATE conversations
SET user_prompt_count = (
  SELECT COUNT(*)
  FROM messages
  WHERE messages.conversation_id = conversations.id
    AND messages.message_type = 'prompt'
);
