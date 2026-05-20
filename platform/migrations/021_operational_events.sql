CREATE TABLE IF NOT EXISTS operational_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  event_name VARCHAR(64) NOT NULL,
  surface VARCHAR(16) NOT NULL,
  visitor_id VARCHAR(96) NULL,
  fingerprint_hash VARCHAR(128) NULL,
  user_id BIGINT UNSIGNED NULL,
  api_key_id BIGINT UNSIGNED NULL,
  order_id BIGINT UNSIGNED NULL,
  invite VARCHAR(191) NULL,
  utm_source VARCHAR(191) NULL,
  utm_medium VARCHAR(191) NULL,
  utm_campaign VARCHAR(191) NULL,
  utm_term VARCHAR(191) NULL,
  utm_content VARCHAR(191) NULL,
  utm_id VARCHAR(191) NULL,
  page_path VARCHAR(512) NULL,
  referrer VARCHAR(512) NULL,
  user_agent VARCHAR(512) NULL,
  metadata_json JSON NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_operational_events_event_name_created_at (event_name, created_at),
  KEY idx_operational_events_surface_created_at (surface, created_at),
  KEY idx_operational_events_visitor_id_created_at (visitor_id, created_at),
  KEY idx_operational_events_fingerprint_hash_created_at (fingerprint_hash, created_at),
  KEY idx_operational_events_user_id_created_at (user_id, created_at)
);

SET @add_users_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND INDEX_NAME = 'idx_users_created_at'), 'SELECT 1', 'ALTER TABLE users ADD KEY idx_users_created_at (created_at)');
PREPARE stmt FROM @add_users_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_cli_login_challenges_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cli_login_challenges' AND INDEX_NAME = 'idx_cli_login_challenges_created_at'), 'SELECT 1', 'ALTER TABLE cli_login_challenges ADD KEY idx_cli_login_challenges_created_at (created_at)');
PREPARE stmt FROM @add_cli_login_challenges_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_cli_login_challenges_completed_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cli_login_challenges' AND INDEX_NAME = 'idx_cli_login_challenges_completed_at'), 'SELECT 1', 'ALTER TABLE cli_login_challenges ADD KEY idx_cli_login_challenges_completed_at (completed_at)');
PREPARE stmt FROM @add_cli_login_challenges_completed_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_cli_sessions_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cli_sessions' AND INDEX_NAME = 'idx_cli_sessions_created_at'), 'SELECT 1', 'ALTER TABLE cli_sessions ADD KEY idx_cli_sessions_created_at (created_at)');
PREPARE stmt FROM @add_cli_sessions_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_cli_sessions_user_id_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cli_sessions' AND INDEX_NAME = 'idx_cli_sessions_user_id_created_at'), 'SELECT 1', 'ALTER TABLE cli_sessions ADD KEY idx_cli_sessions_user_id_created_at (user_id, created_at)');
PREPARE stmt FROM @add_cli_sessions_user_id_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_usage_events_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND INDEX_NAME = 'idx_usage_events_created_at'), 'SELECT 1', 'ALTER TABLE usage_events ADD KEY idx_usage_events_created_at (created_at)');
PREPARE stmt FROM @add_usage_events_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_usage_events_fingerprint_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND INDEX_NAME = 'idx_usage_events_fingerprint_created_at'), 'SELECT 1', 'ALTER TABLE usage_events ADD KEY idx_usage_events_fingerprint_created_at (fingerprint_hash, created_at)');
PREPARE stmt FROM @add_usage_events_fingerprint_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_usage_events_user_id_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND INDEX_NAME = 'idx_usage_events_user_id_created_at'), 'SELECT 1', 'ALTER TABLE usage_events ADD KEY idx_usage_events_user_id_created_at (user_id, created_at)');
PREPARE stmt FROM @add_usage_events_user_id_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_usage_events_mode_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND INDEX_NAME = 'idx_usage_events_mode_created_at'), 'SELECT 1', 'ALTER TABLE usage_events ADD KEY idx_usage_events_mode_created_at (mode, created_at)');
PREPARE stmt FROM @add_usage_events_mode_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_usage_events_result_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND INDEX_NAME = 'idx_usage_events_result_created_at'), 'SELECT 1', 'ALTER TABLE usage_events ADD KEY idx_usage_events_result_created_at (result, created_at)');
PREPARE stmt FROM @add_usage_events_result_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_orders_created_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'orders' AND INDEX_NAME = 'idx_orders_created_at'), 'SELECT 1', 'ALTER TABLE orders ADD KEY idx_orders_created_at (created_at)');
PREPARE stmt FROM @add_orders_created_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_orders_updated_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'orders' AND INDEX_NAME = 'idx_orders_updated_at'), 'SELECT 1', 'ALTER TABLE orders ADD KEY idx_orders_updated_at (updated_at)');
PREPARE stmt FROM @add_orders_updated_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_orders_status_updated_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'orders' AND INDEX_NAME = 'idx_orders_status_updated_at'), 'SELECT 1', 'ALTER TABLE orders ADD KEY idx_orders_status_updated_at (status, updated_at)');
PREPARE stmt FROM @add_orders_status_updated_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @add_orders_user_id_status_updated_at_index := IF(EXISTS (SELECT 1 FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'orders' AND INDEX_NAME = 'idx_orders_user_id_status_updated_at'), 'SELECT 1', 'ALTER TABLE orders ADD KEY idx_orders_user_id_status_updated_at (user_id, status, updated_at)');
PREPARE stmt FROM @add_orders_user_id_status_updated_at_index; EXECUTE stmt; DEALLOCATE PREPARE stmt;
