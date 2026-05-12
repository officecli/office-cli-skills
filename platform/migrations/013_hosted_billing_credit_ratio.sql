SET @add_hosted_pricing_settings_credits_per_usd := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'hosted_pricing_settings' AND COLUMN_NAME = 'credits_per_usd'
  ),
  'SELECT 1',
  'ALTER TABLE hosted_pricing_settings ADD COLUMN credits_per_usd INT NOT NULL DEFAULT 100 AFTER currency'
);
PREPARE stmt FROM @add_hosted_pricing_settings_credits_per_usd;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE hosted_pricing_settings
SET credits_per_usd = 100
WHERE credits_per_usd <= 0;
