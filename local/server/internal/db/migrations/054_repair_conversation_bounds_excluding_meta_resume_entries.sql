-- Recompute conversation started_at/ended_at in post-migration code so
-- meta resume prompts (for example Claude's "Continue from where you left off.")
-- do not affect conversation ordering.
SELECT 1;
