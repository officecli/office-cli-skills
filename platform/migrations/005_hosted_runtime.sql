SET @add_api_keys_allowed_modes := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'api_keys' AND COLUMN_NAME = 'allowed_modes'
  ),
  'SELECT 1',
  'ALTER TABLE api_keys ADD COLUMN allowed_modes VARCHAR(32) NOT NULL DEFAULT "external_only" AFTER plan_code'
);
PREPARE stmt FROM @add_api_keys_allowed_modes;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_api_keys_hosted_enabled := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'api_keys' AND COLUMN_NAME = 'hosted_enabled'
  ),
  'SELECT 1',
  'ALTER TABLE api_keys ADD COLUMN hosted_enabled TINYINT(1) NOT NULL DEFAULT 0 AFTER allowed_modes'
);
PREPARE stmt FROM @add_api_keys_hosted_enabled;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_api_keys_default_runtime_mode := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'api_keys' AND COLUMN_NAME = 'default_runtime_mode'
  ),
  'SELECT 1',
  'ALTER TABLE api_keys ADD COLUMN default_runtime_mode VARCHAR(16) NULL AFTER hosted_enabled'
);
PREPARE stmt FROM @add_api_keys_default_runtime_mode;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_api_keys_credit_balance := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'api_keys' AND COLUMN_NAME = 'credit_balance'
  ),
  'SELECT 1',
  'ALTER TABLE api_keys ADD COLUMN credit_balance INT NOT NULL DEFAULT 0 AFTER quota_used'
);
PREPARE stmt FROM @add_api_keys_credit_balance;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_api_keys_credit_reserved := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'api_keys' AND COLUMN_NAME = 'credit_reserved'
  ),
  'SELECT 1',
  'ALTER TABLE api_keys ADD COLUMN credit_reserved INT NOT NULL DEFAULT 0 AFTER credit_balance'
);
PREPARE stmt FROM @add_api_keys_credit_reserved;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_runtime_mode := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'runtime_mode'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN runtime_mode VARCHAR(16) NULL AFTER document_type'
);
PREPARE stmt FROM @add_usage_events_runtime_mode;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_provider := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'provider'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN provider VARCHAR(64) NULL AFTER charged'
);
PREPARE stmt FROM @add_usage_events_provider;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_model_name := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'model_name'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN model_name VARCHAR(128) NULL AFTER provider'
);
PREPARE stmt FROM @add_usage_events_model_name;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_prompt_tokens := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'prompt_tokens'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN prompt_tokens INT NOT NULL DEFAULT 0 AFTER model_name'
);
PREPARE stmt FROM @add_usage_events_prompt_tokens;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_completion_tokens := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'completion_tokens'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN completion_tokens INT NOT NULL DEFAULT 0 AFTER prompt_tokens'
);
PREPARE stmt FROM @add_usage_events_completion_tokens;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_reasoning_tokens := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'reasoning_tokens'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN reasoning_tokens INT NOT NULL DEFAULT 0 AFTER completion_tokens'
);
PREPARE stmt FROM @add_usage_events_reasoning_tokens;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_image_count := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'image_count'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN image_count INT NOT NULL DEFAULT 0 AFTER reasoning_tokens'
);
PREPARE stmt FROM @add_usage_events_image_count;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_reserved_credits := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'reserved_credits'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN reserved_credits INT NOT NULL DEFAULT 0 AFTER image_count'
);
PREPARE stmt FROM @add_usage_events_reserved_credits;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_settled_credits := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'settled_credits'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN settled_credits INT NOT NULL DEFAULT 0 AFTER reserved_credits'
);
PREPARE stmt FROM @add_usage_events_settled_credits;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_refund_credits := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'refund_credits'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN refund_credits INT NOT NULL DEFAULT 0 AFTER settled_credits'
);
PREPARE stmt FROM @add_usage_events_refund_credits;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_orders_pack_kind := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'orders' AND COLUMN_NAME = 'pack_kind'
  ),
  'SELECT 1',
  'ALTER TABLE orders ADD COLUMN pack_kind VARCHAR(32) NOT NULL DEFAULT "external_generation" AFTER pack_name'
);
PREPARE stmt FROM @add_orders_pack_kind;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_orders_credit_amount := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'orders' AND COLUMN_NAME = 'credit_amount'
  ),
  'SELECT 1',
  'ALTER TABLE orders ADD COLUMN credit_amount INT NOT NULL DEFAULT 0 AFTER quota_amount'
);
PREPARE stmt FROM @add_orders_credit_amount;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
