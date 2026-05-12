UPDATE hosted_pricing_rules
SET
  document_profile = 'text',
  text_model_key = COALESCE(NULLIF(text_model_key, ''), 'text_default'),
  image_model_key = ''
WHERE document_profile IN ('docx-xlsx', 'pptx-no-image');

UPDATE hosted_pricing_rules
SET
  document_profile = 'text',
  text_model_key = COALESCE(NULLIF(text_model_key, ''), 'text_default'),
  image_model_key = ''
WHERE document_profile = 'pptx-with-image'
  AND NOT EXISTS (
    SELECT 1 FROM (
      SELECT id FROM hosted_pricing_rules WHERE document_profile = 'text'
    ) AS existing_text
  )
ORDER BY id
LIMIT 1;

DELETE FROM hosted_pricing_rules
WHERE document_profile = 'pptx-with-image';

UPDATE hosted_pricing_rules
SET
  document_profile = 'image',
  text_model_key = '',
  image_model_key = COALESCE(NULLIF(image_model_key, ''), 'image_default')
WHERE document_profile = 'img';

DELETE r
FROM hosted_pricing_rules r
JOIN (
  SELECT id
  FROM (
    SELECT
      id,
      ROW_NUMBER() OVER (PARTITION BY document_profile ORDER BY enabled DESC, id ASC) AS rn
    FROM hosted_pricing_rules
    WHERE document_profile IN ('text', 'image')
  ) AS ranked_profiles
  WHERE rn > 1
) AS duplicate_profiles
  ON duplicate_profiles.id = r.id;
