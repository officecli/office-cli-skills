CREATE TABLE IF NOT EXISTS hosted_pricing_settings (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  markup_bps INT NOT NULL DEFAULT 3000,
  currency VARCHAR(8) NOT NULL DEFAULT 'usd',
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
);

INSERT IGNORE INTO hosted_pricing_settings (id, markup_bps, currency)
VALUES (1, 3000, 'usd');

CREATE TABLE IF NOT EXISTS hosted_pricing_rules (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  document_profile VARCHAR(64) NOT NULL,
  provider VARCHAR(64) NOT NULL,
  model VARCHAR(128) NOT NULL,
  prompt_per_1k_cost_microusd BIGINT NOT NULL DEFAULT 0,
  output_per_1k_cost_microusd BIGINT NOT NULL DEFAULT 0,
  reasoning_per_1k_cost_microusd BIGINT NOT NULL DEFAULT 0,
  image_per_asset_cost_microusd BIGINT NOT NULL DEFAULT 0,
  reservation_credits INT NOT NULL DEFAULT 0,
  minimum_charge_credits INT NOT NULL DEFAULT 0,
  markup_bps INT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  KEY idx_hosted_pricing_rules_profile (document_profile),
  KEY idx_hosted_pricing_rules_enabled (enabled)
);

CREATE TABLE IF NOT EXISTS hosted_credit_packs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description VARCHAR(512) NOT NULL,
  currency VARCHAR(8) NOT NULL DEFAULT 'usd',
  amount_total BIGINT NOT NULL,
  credit_amount INT NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  KEY idx_hosted_credit_packs_enabled (enabled)
);

SET @add_usage_events_hosted_pricing_rule_id := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'hosted_pricing_rule_id'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN hosted_pricing_rule_id BIGINT NOT NULL DEFAULT 0 AFTER refund_credits'
);
PREPARE stmt FROM @add_usage_events_hosted_pricing_rule_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_markup_bps := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'markup_bps'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN markup_bps INT NOT NULL DEFAULT 0 AFTER hosted_pricing_rule_id'
);
PREPARE stmt FROM @add_usage_events_markup_bps;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_upstream_cost_microusd := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'upstream_cost_microusd'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN upstream_cost_microusd BIGINT NOT NULL DEFAULT 0 AFTER markup_bps'
);
PREPARE stmt FROM @add_usage_events_upstream_cost_microusd;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_uncapped_charge_credits := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'uncapped_charge_credits'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN uncapped_charge_credits INT NOT NULL DEFAULT 0 AFTER upstream_cost_microusd'
);
PREPARE stmt FROM @add_usage_events_uncapped_charge_credits;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_profit_microusd := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'profit_microusd'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN profit_microusd BIGINT NOT NULL DEFAULT 0 AFTER uncapped_charge_credits'
);
PREPARE stmt FROM @add_usage_events_profit_microusd;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_cap_applied := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'cap_applied'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN cap_applied TINYINT(1) NOT NULL DEFAULT 0 AFTER profit_microusd'
);
PREPARE stmt FROM @add_usage_events_cap_applied;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
