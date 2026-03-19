-- Recalculate ended_at to only reflect prompt/answer messages, so that agent
-- messages, slash commands, and rating-related ingestion don't affect
-- conversation sort order.
UPDATE conversations
SET ended_at = COALESCE(
    (SELECT MAX(m.timestamp)
     FROM messages m
     WHERE m.conversation_id = conversations.id
       AND m.message_type IN ('prompt', 'answer')),
    ended_at
);
