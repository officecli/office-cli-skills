CREATE TABLE IF NOT EXISTS daily_free_quotas (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  fingerprint_hash VARCHAR(128) NOT NULL,
  usage_date VARCHAR(10) NOT NULL,
  document_type VARCHAR(32) NOT NULL DEFAULT 'document',
  daily_limit INT NOT NULL,
  daily_used INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_daily_free_quotas_fingerprint_date_type (fingerprint_hash, usage_date, document_type),
  KEY idx_daily_free_quotas_fingerprint (fingerprint_hash)
);

SET @add_users_invite_code := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'invite_code'
  ),
  'SELECT 1',
  'ALTER TABLE users ADD COLUMN invite_code VARCHAR(64) NOT NULL DEFAULT "" AFTER name'
);
PREPARE stmt FROM @add_users_invite_code;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @fill_users_invite_code := '
  UPDATE users
  SET invite_code = CONCAT("invite-", LPAD(CONV(id, 10, 36), 6, "0"))
  WHERE invite_code = ""
';
PREPARE stmt FROM @fill_users_invite_code;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_users_invite_code_unique := IF(
  EXISTS(
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND INDEX_NAME = 'uk_users_invite_code'
  ),
  'SELECT 1',
  'ALTER TABLE users ADD UNIQUE KEY uk_users_invite_code (invite_code)'
);
PREPARE stmt FROM @add_users_invite_code_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_usage_events_user_id := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'usage_events' AND COLUMN_NAME = 'user_id'
  ),
  'SELECT 1',
  'ALTER TABLE usage_events ADD COLUMN user_id BIGINT UNSIGNED NULL AFTER document_type'
);
PREPARE stmt FROM @add_usage_events_user_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS reward_grants (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  source_type VARCHAR(64) NOT NULL,
  idempotency_key VARCHAR(191) NOT NULL,
  amount_total INT NOT NULL,
  amount_used INT NOT NULL DEFAULT 0,
  reason VARCHAR(191) NOT NULL,
  metadata_json JSON NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_reward_grants_idempotency_key (idempotency_key),
  KEY idx_reward_grants_user_id (user_id),
  KEY idx_reward_grants_source_type (source_type)
);

CREATE TABLE IF NOT EXISTS user_referrals (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  inviter_user_id BIGINT UNSIGNED NOT NULL,
  invited_user_id BIGINT UNSIGNED NOT NULL,
  invite_code VARCHAR(64) NOT NULL,
  registered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  activated_at DATETIME NULL,
  reward_granted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_user_referrals_invited_user_id (invited_user_id),
  KEY idx_user_referrals_inviter_user_id (inviter_user_id),
  KEY idx_user_referrals_invite_code (invite_code)
);

CREATE TABLE IF NOT EXISTS discord_connections (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  discord_user_id VARCHAR(191) NOT NULL,
  username VARCHAR(191) NOT NULL,
  guild_member TINYINT(1) NOT NULL DEFAULT 0,
  connected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  reward_granted_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_discord_connections_user_id (user_id),
  UNIQUE KEY uk_discord_connections_discord_user_id (discord_user_id)
);
