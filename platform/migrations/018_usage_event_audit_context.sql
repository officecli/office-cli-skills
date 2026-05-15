SET @add_usage_events_client_ip := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'client_ip'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN client_ip VARCHAR(64) NULL AFTER cap_applied'
);
PREPARE stmt FROM @add_usage_events_client_ip;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_forwarded_for := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'forwarded_for'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN forwarded_for VARCHAR(512) NULL AFTER client_ip'
);
PREPARE stmt FROM @add_usage_events_forwarded_for;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_user_agent := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'user_agent'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN user_agent VARCHAR(512) NULL AFTER forwarded_for'
);
PREPARE stmt FROM @add_usage_events_user_agent;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_request_host := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'request_host'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN request_host VARCHAR(191) NULL AFTER user_agent'
);
PREPARE stmt FROM @add_usage_events_request_host;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_request_path := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'request_path'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN request_path VARCHAR(191) NULL AFTER request_host'
);
PREPARE stmt FROM @add_usage_events_request_path;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_request_method := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'request_method'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN request_method VARCHAR(16) NULL AFTER request_path'
);
PREPARE stmt FROM @add_usage_events_request_method;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_client_ip_index := IF(
  EXISTS(
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND INDEX_NAME = 'idx_usage_events_client_ip'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD KEY idx_usage_events_client_ip (client_ip)'
);
PREPARE stmt FROM @add_usage_events_client_ip_index;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
