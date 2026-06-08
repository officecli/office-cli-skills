SET @add_owner_user_id := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'api_keys' AND COLUMN_NAME = 'owner_user_id'
  ),
  'SELECT 1',
  'ALTER TABLE api_keys ADD COLUMN owner_user_id BIGINT UNSIGNED NULL AFTER id'
);
PREPARE stmt FROM @add_owner_user_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  google_sub VARCHAR(191) NOT NULL,
  email VARCHAR(191) NOT NULL,
  name VARCHAR(191) NOT NULL,
  avatar_url VARCHAR(512) NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  paid_entitlement TINYINT(1) NOT NULL DEFAULT 0,
  paid_entitlement_updated_at DATETIME NULL,
  paid_entitlement_source VARCHAR(32) NOT NULL DEFAULT 'none',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_users_google_sub (google_sub),
  UNIQUE KEY uk_users_email (email),
  KEY idx_users_status (status)
);

CREATE TABLE IF NOT EXISTS stripe_customers (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  stripe_customer_id VARCHAR(191) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_stripe_customers_user_id (user_id),
  UNIQUE KEY uk_stripe_customers_customer_id (stripe_customer_id)
);

CREATE TABLE IF NOT EXISTS orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  stripe_checkout_session_id VARCHAR(191) NULL,
  stripe_payment_intent_id VARCHAR(191) NULL,
  stripe_customer_id VARCHAR(191) NULL,
  status VARCHAR(16) NOT NULL,
  currency VARCHAR(16) NOT NULL,
  amount_total BIGINT NOT NULL,
  pack_code VARCHAR(64) NOT NULL,
  pack_name VARCHAR(128) NOT NULL,
  quota_amount INT NOT NULL,
  target_api_key_id BIGINT UNSIGNED NULL,
  note TEXT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_orders_checkout_session (stripe_checkout_session_id),
  KEY idx_orders_user_id (user_id),
  KEY idx_orders_status (status),
  KEY idx_orders_target_api_key_id (target_api_key_id)
);

CREATE TABLE IF NOT EXISTS billing_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  order_id BIGINT UNSIGNED NULL,
  event_id VARCHAR(191) NOT NULL,
  event_type VARCHAR(128) NOT NULL,
  status VARCHAR(16) NOT NULL,
  payload_json JSON NOT NULL,
  error_message TEXT NULL,
  processed_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_billing_events_event_id (event_id),
  KEY idx_billing_events_order_id (order_id),
  KEY idx_billing_events_event_type (event_type),
  KEY idx_billing_events_status (status)
);
