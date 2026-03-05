-- No schema changes. This migration exists to trigger post-migration
-- message-type backfill, which refreshes question/answer markdown formatting.
SELECT 1;
