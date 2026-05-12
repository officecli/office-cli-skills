CREATE TABLE IF NOT EXISTS hosted_credit_grants (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  api_key_id BIGINT UNSIGNED NOT NULL,
  source_type VARCHAR(64) NOT NULL,
  idempotency_key VARCHAR(191) NOT NULL UNIQUE,
  credit_amount INT NOT NULL,
  reason VARCHAR(191) NOT NULL,
  metadata_json JSON NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_hosted_credit_grants_user_id (user_id),
  KEY idx_hosted_credit_grants_api_key_id (api_key_id),
  KEY idx_hosted_credit_grants_source_type (source_type)
);
