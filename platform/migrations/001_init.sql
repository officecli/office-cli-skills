CREATE TABLE IF NOT EXISTS api_keys (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  key_hash VARCHAR(128) NOT NULL,
  key_prefix VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  plan_name VARCHAR(128) NOT NULL,
  plan_code VARCHAR(64) NULL,
  expires_at DATETIME NULL,
  note TEXT NULL,
  last_used_at DATETIME NULL,
  quota_total INT NULL,
  quota_used INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_api_keys_key_hash (key_hash),
  KEY idx_api_keys_status (status)
);

CREATE TABLE IF NOT EXISTS free_quotas (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  fingerprint_hash VARCHAR(128) NOT NULL,
  free_limit INT NOT NULL,
  free_used INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_free_quotas_fingerprint_hash (fingerprint_hash)
);

CREATE TABLE IF NOT EXISTS usage_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  request_id VARCHAR(128) NULL,
  fingerprint_hash VARCHAR(128) NOT NULL,
  mode VARCHAR(16) NOT NULL,
  action VARCHAR(16) NOT NULL,
  api_key_id BIGINT UNSIGNED NULL,
  result VARCHAR(16) NOT NULL,
  reason_code VARCHAR(64) NULL,
  cli_version VARCHAR(64) NULL,
  document_type VARCHAR(32) NULL,
  billed_units INT NOT NULL DEFAULT 0,
  unit_type VARCHAR(32) NOT NULL DEFAULT 'document',
  charged TINYINT(1) NOT NULL DEFAULT 0,
  client_ip VARCHAR(64) NULL,
  forwarded_for VARCHAR(512) NULL,
  user_agent VARCHAR(512) NULL,
  request_host VARCHAR(191) NULL,
  request_path VARCHAR(191) NULL,
  request_method VARCHAR(16) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_usage_events_request_id (request_id),
  KEY idx_usage_events_fingerprint (fingerprint_hash),
  KEY idx_usage_events_mode (mode),
  KEY idx_usage_events_result (result),
  KEY idx_usage_events_reason_code (reason_code),
  KEY idx_usage_events_api_key_id (api_key_id),
  KEY idx_usage_events_client_ip (client_ip)
);

CREATE TABLE IF NOT EXISTS admin_audit_logs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  action VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id VARCHAR(128) NOT NULL,
  payload_json JSON NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_admin_audit_logs_action (action),
  KEY idx_admin_audit_logs_target_type (target_type),
  KEY idx_admin_audit_logs_target_id (target_id)
);
