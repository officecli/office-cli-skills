CREATE TABLE IF NOT EXISTS user_hosted_credit_accounts (
  user_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  credit_balance INT NOT NULL DEFAULT 0,
  credit_reserved INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_hosted_credit_ledger (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  source_type VARCHAR(64) NOT NULL,
  idempotency_key VARCHAR(191) NOT NULL UNIQUE,
  credit_delta INT NOT NULL DEFAULT 0,
  reserved_delta INT NOT NULL DEFAULT 0,
  usage_event_id BIGINT UNSIGNED NULL,
  order_id BIGINT UNSIGNED NULL,
  reason VARCHAR(191) NOT NULL,
  metadata_json JSON NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_user_hosted_credit_ledger_user_id (user_id),
  KEY idx_user_hosted_credit_ledger_source_type (source_type),
  KEY idx_user_hosted_credit_ledger_usage_event_id (usage_event_id),
  KEY idx_user_hosted_credit_ledger_order_id (order_id)
);

CREATE TABLE IF NOT EXISTS cli_login_challenges (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  challenge_id VARCHAR(128) NOT NULL UNIQUE,
  flow VARCHAR(16) NOT NULL DEFAULT 'callback',
  code_challenge VARCHAR(191) NOT NULL,
  code_challenge_method VARCHAR(16) NOT NULL,
  redirect_uri VARCHAR(512) NOT NULL,
  state VARCHAR(191) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  user_id BIGINT UNSIGNED NULL,
  user_code_hash VARCHAR(128) NULL UNIQUE,
  exchange_code_hash VARCHAR(128) NULL UNIQUE,
  expires_at TIMESTAMP NOT NULL,
  completed_at TIMESTAMP NULL,
  consumed_at TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_cli_login_challenges_flow (flow),
  KEY idx_cli_login_challenges_user_id (user_id),
  KEY idx_cli_login_challenges_expires_at (expires_at)
);

CREATE TABLE IF NOT EXISTS cli_sessions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  token_hash VARCHAR(128) NOT NULL UNIQUE,
  token_prefix VARCHAR(32) NOT NULL,
  name VARCHAR(191) NOT NULL,
  revoked_at TIMESTAMP NULL,
  last_used_at TIMESTAMP NULL,
  expires_at TIMESTAMP NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_cli_sessions_user_id (user_id),
  KEY idx_cli_sessions_expires_at (expires_at)
);

INSERT INTO user_hosted_credit_accounts (user_id, credit_balance, credit_reserved, created_at, updated_at)
SELECT owner_user_id, SUM(GREATEST(credit_balance - credit_reserved, 0)), 0, NOW(), NOW()
FROM api_keys
WHERE owner_user_id IS NOT NULL
  AND GREATEST(credit_balance - credit_reserved, 0) > 0
GROUP BY owner_user_id
ON DUPLICATE KEY UPDATE
  credit_balance = user_hosted_credit_accounts.credit_balance + VALUES(credit_balance),
  updated_at = NOW();

INSERT IGNORE INTO user_hosted_credit_ledger (
  user_id,
  source_type,
  idempotency_key,
  credit_delta,
  reserved_delta,
  reason,
  metadata_json,
  created_at
)
SELECT
  owner_user_id,
  'migration',
  CONCAT('migration:api_keys:', owner_user_id),
  SUM(GREATEST(credit_balance - credit_reserved, 0)),
  0,
  'migrated hosted credits from api key balances',
  JSON_OBJECT('source', 'api_keys.credit_balance', 'owner_user_id', owner_user_id),
  NOW()
FROM api_keys
WHERE owner_user_id IS NOT NULL
  AND GREATEST(credit_balance - credit_reserved, 0) > 0
GROUP BY owner_user_id;
