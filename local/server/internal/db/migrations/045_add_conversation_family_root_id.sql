-- Add precomputed family_root_id to avoid recursive CTE on every query.
-- family_root_id is the ultimate root conversation of a family chain.
-- For conversations with no parent, it equals their own id.
ALTER TABLE conversations ADD COLUMN family_root_id TEXT NOT NULL DEFAULT '';

-- Backfill: first set all conversations to point to themselves.
UPDATE conversations SET family_root_id = id;

-- Now walk parent chains to find the true root.
-- We iterate a fixed number of times (max depth 32) to propagate roots upward.
-- Each pass sets family_root_id to the parent's family_root_id for any conversation
-- whose parent has a different (higher) root.
UPDATE conversations SET family_root_id = (
    SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id
) WHERE parent_conversation_id <> ''
  AND family_root_id <> (SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id);

UPDATE conversations SET family_root_id = (
    SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id
) WHERE parent_conversation_id <> ''
  AND family_root_id <> (SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id);

UPDATE conversations SET family_root_id = (
    SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id
) WHERE parent_conversation_id <> ''
  AND family_root_id <> (SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id);

UPDATE conversations SET family_root_id = (
    SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id
) WHERE parent_conversation_id <> ''
  AND family_root_id <> (SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id);

UPDATE conversations SET family_root_id = (
    SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id
) WHERE parent_conversation_id <> ''
  AND family_root_id <> (SELECT p.family_root_id FROM conversations p WHERE p.id = conversations.parent_conversation_id);

-- Index for the project detail page query.
CREATE INDEX IF NOT EXISTS idx_conversations_family_root ON conversations(project_id, family_root_id, hidden);
