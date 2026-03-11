UPDATE messages
SET message_type = 'diff'
WHERE raw_json = '{"source":"derived_diff"}'
  AND message_type <> 'diff';
