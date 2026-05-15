SET @flow_column_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'cli_login_challenges'
    AND COLUMN_NAME = 'flow'
);
SET @flow_column_sql := IF(
  @flow_column_exists = 0,
  'ALTER TABLE cli_login_challenges ADD COLUMN flow VARCHAR(16) NOT NULL DEFAULT ''callback''',
  'SELECT 1'
);
PREPARE flow_column_stmt FROM @flow_column_sql;
EXECUTE flow_column_stmt;
DEALLOCATE PREPARE flow_column_stmt;

SET @user_code_hash_column_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'cli_login_challenges'
    AND COLUMN_NAME = 'user_code_hash'
);
SET @user_code_hash_column_sql := IF(
  @user_code_hash_column_exists = 0,
  'ALTER TABLE cli_login_challenges ADD COLUMN user_code_hash VARCHAR(128) NULL',
  'SELECT 1'
);
PREPARE user_code_hash_column_stmt FROM @user_code_hash_column_sql;
EXECUTE user_code_hash_column_stmt;
DEALLOCATE PREPARE user_code_hash_column_stmt;

SET @flow_index_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'cli_login_challenges'
    AND INDEX_NAME = 'idx_cli_login_challenges_flow'
);
SET @flow_index_sql := IF(
  @flow_index_exists = 0,
  'CREATE INDEX idx_cli_login_challenges_flow ON cli_login_challenges(flow)',
  'SELECT 1'
);
PREPARE flow_index_stmt FROM @flow_index_sql;
EXECUTE flow_index_stmt;
DEALLOCATE PREPARE flow_index_stmt;

SET @user_code_hash_index_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'cli_login_challenges'
    AND INDEX_NAME = 'idx_cli_login_challenges_user_code_hash'
);
SET @user_code_hash_index_sql := IF(
  @user_code_hash_index_exists = 0,
  'CREATE UNIQUE INDEX idx_cli_login_challenges_user_code_hash ON cli_login_challenges(user_code_hash)',
  'SELECT 1'
);
PREPARE user_code_hash_index_stmt FROM @user_code_hash_index_sql;
EXECUTE user_code_hash_index_stmt;
DEALLOCATE PREPARE user_code_hash_index_stmt;
