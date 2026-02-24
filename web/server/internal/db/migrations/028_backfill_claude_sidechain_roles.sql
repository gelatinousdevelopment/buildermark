UPDATE messages
SET role = 'agent'
WHERE role = 'user'
  AND json_valid(raw_json)
  AND json_extract(raw_json, '$.type') = 'user'
  AND COALESCE(json_extract(raw_json, '$.isSidechain'), 0) = 1
  AND LOWER(COALESCE(json_extract(raw_json, '$.userType'), '')) = 'external'
  AND COALESCE(TRIM(json_extract(raw_json, '$.agentId')), '') <> ''
  AND conversation_id IN (
      SELECT id
      FROM conversations
      WHERE agent = 'claude'
  );
