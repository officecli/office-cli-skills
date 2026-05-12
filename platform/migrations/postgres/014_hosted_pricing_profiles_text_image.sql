UPDATE hosted_pricing_rules
SET
  document_profile = 'text',
  text_model_key = COALESCE(NULLIF(text_model_key, ''), 'text_default'),
  image_model_key = ''
WHERE document_profile IN ('docx-xlsx', 'pptx-no-image');

WITH fallback_pptx_text AS (
  SELECT id
  FROM hosted_pricing_rules
  WHERE document_profile = 'pptx-with-image'
    AND NOT EXISTS (SELECT 1 FROM hosted_pricing_rules WHERE document_profile = 'text')
  ORDER BY id
  LIMIT 1
)
UPDATE hosted_pricing_rules
SET
  document_profile = 'text',
  text_model_key = COALESCE(NULLIF(text_model_key, ''), 'text_default'),
  image_model_key = ''
WHERE id IN (SELECT id FROM fallback_pptx_text);

DELETE FROM hosted_pricing_rules
WHERE document_profile = 'pptx-with-image';

UPDATE hosted_pricing_rules
SET
  document_profile = 'image',
  text_model_key = '',
  image_model_key = COALESCE(NULLIF(image_model_key, ''), 'image_default')
WHERE document_profile = 'img';

WITH ranked_profiles AS (
  SELECT
    id,
    ROW_NUMBER() OVER (PARTITION BY document_profile ORDER BY enabled DESC, id ASC) AS rn
  FROM hosted_pricing_rules
  WHERE document_profile IN ('text', 'image')
)
DELETE FROM hosted_pricing_rules r
USING ranked_profiles
WHERE r.id = ranked_profiles.id
  AND ranked_profiles.rn > 1;
