UPDATE messages
SET content = (
    SELECT GROUP_CONCAT(text_value, '\n')
    FROM (
        SELECT TRIM(json_extract(j.value, '$.text')) AS text_value
        FROM json_each(messages.raw_json, '$.payload.summary') AS j
        WHERE COALESCE(TRIM(json_extract(j.value, '$.text')), '') <> ''
    )
)
WHERE json_extract(raw_json, '$.type') = 'response_item'
  AND json_extract(raw_json, '$.payload.type') = 'reasoning'
  AND (content LIKE '[response_item:reasoning]%' OR TRIM(content) = '')
  AND EXISTS (
      SELECT 1
      FROM json_each(messages.raw_json, '$.payload.summary') AS j
      WHERE COALESCE(TRIM(json_extract(j.value, '$.text')), '') <> ''
  );
