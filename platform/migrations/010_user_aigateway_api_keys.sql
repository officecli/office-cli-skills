CREATE TABLE IF NOT EXISTS user_aigateway_api_keys (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  key_ciphertext TEXT NOT NULL,
  key_prefix VARCHAR(32) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL,
  upstream_id VARCHAR(128) NOT NULL DEFAULT '',
  upstream_name VARCHAR(191) NOT NULL,
  last_error TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_user_aigateway_api_keys_user_id (user_id),
  KEY idx_user_aigateway_api_keys_status (status)
);
