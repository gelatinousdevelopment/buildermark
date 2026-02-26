UPDATE messages
SET content = json_extract(raw_json, '$.summary')
WHERE content = '[summary]'
  AND json_extract(raw_json, '$.type') = 'summary'
  AND COALESCE(TRIM(json_extract(raw_json, '$.summary')), '') <> '';
