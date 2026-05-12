CREATE TABLE IF NOT EXISTS hosted_model_pricing_configs (
  id BIGSERIAL PRIMARY KEY,
  key VARCHAR(64) NOT NULL UNIQUE,
  kind VARCHAR(32) NOT NULL,
  provider VARCHAR(64) NOT NULL,
  model VARCHAR(128) NOT NULL,
  prompt_per_1m_cost_microusd BIGINT NOT NULL DEFAULT 0,
  output_per_1m_cost_microusd BIGINT NOT NULL DEFAULT 0,
  reasoning_per_1m_cost_microusd BIGINT NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_hosted_model_pricing_configs_kind ON hosted_model_pricing_configs(kind);
CREATE INDEX IF NOT EXISTS idx_hosted_model_pricing_configs_enabled ON hosted_model_pricing_configs(enabled);

INSERT INTO hosted_model_pricing_configs (key, kind, provider, model, enabled)
VALUES
  ('text_default', 'text', 'openai', 'gpt-4.1', TRUE),
  ('image_default', 'image', 'openai', 'gpt-image-2', TRUE)
ON CONFLICT (key) DO NOTHING;

ALTER TABLE hosted_pricing_rules ADD COLUMN IF NOT EXISTS text_model_key VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE hosted_pricing_rules ADD COLUMN IF NOT EXISTS image_model_key VARCHAR(64) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_hosted_pricing_rules_text_model_key ON hosted_pricing_rules(text_model_key);
CREATE INDEX IF NOT EXISTS idx_hosted_pricing_rules_image_model_key ON hosted_pricing_rules(image_model_key);

UPDATE hosted_pricing_rules
SET text_model_key = 'text_default'
WHERE document_profile IN ('docx-xlsx', 'pptx-no-image', 'pptx-with-image') AND text_model_key = '';

UPDATE hosted_pricing_rules
SET image_model_key = 'image_default'
WHERE document_profile IN ('pptx-with-image', 'img') AND image_model_key = '';

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_hosted_model_pricing_configs_updated_at') THEN
    CREATE TRIGGER set_hosted_model_pricing_configs_updated_at BEFORE UPDATE ON hosted_model_pricing_configs FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
END;
$$;
