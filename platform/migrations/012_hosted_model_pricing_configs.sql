CREATE TABLE IF NOT EXISTS hosted_model_pricing_configs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `key` VARCHAR(64) NOT NULL UNIQUE,
  kind VARCHAR(32) NOT NULL,
  provider VARCHAR(64) NOT NULL,
  model VARCHAR(128) NOT NULL,
  prompt_per_1m_cost_microusd BIGINT NOT NULL DEFAULT 0,
  output_per_1m_cost_microusd BIGINT NOT NULL DEFAULT 0,
  reasoning_per_1m_cost_microusd BIGINT NOT NULL DEFAULT 0,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  KEY idx_hosted_model_pricing_configs_kind (kind),
  KEY idx_hosted_model_pricing_configs_enabled (enabled)
);

INSERT IGNORE INTO hosted_model_pricing_configs (`key`, kind, provider, model, enabled)
VALUES
  ('text_default', 'text', 'openai', 'gpt-4.1', 1),
  ('image_default', 'image', 'openai', 'gpt-image-2', 1);

SET @add_hosted_pricing_rules_text_model_key := IF(
  NOT EXISTS (
    SELECT 1 FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'hosted_pricing_rules' AND COLUMN_NAME = 'text_model_key'
  ),
  'ALTER TABLE hosted_pricing_rules ADD COLUMN text_model_key VARCHAR(64) NOT NULL DEFAULT '''' AFTER model',
  'SELECT 1'
);
PREPARE stmt FROM @add_hosted_pricing_rules_text_model_key;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_hosted_pricing_rules_image_model_key := IF(
  NOT EXISTS (
    SELECT 1 FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'hosted_pricing_rules' AND COLUMN_NAME = 'image_model_key'
  ),
  'ALTER TABLE hosted_pricing_rules ADD COLUMN image_model_key VARCHAR(64) NOT NULL DEFAULT '''' AFTER text_model_key',
  'SELECT 1'
);
PREPARE stmt FROM @add_hosted_pricing_rules_image_model_key;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE hosted_pricing_rules
SET text_model_key = 'text_default'
WHERE document_profile IN ('docx-xlsx', 'pptx-no-image', 'pptx-with-image') AND text_model_key = '';

UPDATE hosted_pricing_rules
SET image_model_key = 'image_default'
WHERE document_profile IN ('pptx-with-image', 'img') AND image_model_key = '';

CREATE INDEX idx_hosted_pricing_rules_text_model_key ON hosted_pricing_rules (text_model_key);
CREATE INDEX idx_hosted_pricing_rules_image_model_key ON hosted_pricing_rules (image_model_key);
