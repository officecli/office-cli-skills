SET @add_daily_free_quotas_document_type := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'daily_free_quotas' AND COLUMN_NAME = 'document_type'
  ),
  'SELECT 1',
  'ALTER TABLE daily_free_quotas ADD COLUMN document_type VARCHAR(32) NOT NULL DEFAULT "document" AFTER usage_date'
);
PREPARE stmt FROM @add_daily_free_quotas_document_type;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_old_daily_free_quota_unique := IF(
  EXISTS(
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'daily_free_quotas' AND INDEX_NAME = 'uk_daily_free_quotas_fingerprint_date'
  ),
  'ALTER TABLE daily_free_quotas DROP INDEX uk_daily_free_quotas_fingerprint_date',
  'SELECT 1'
);
PREPARE stmt FROM @drop_old_daily_free_quota_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_daily_free_quota_type_unique := IF(
  EXISTS(
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'daily_free_quotas' AND INDEX_NAME = 'uk_daily_free_quotas_fingerprint_date_type'
  ),
  'SELECT 1',
  'ALTER TABLE daily_free_quotas ADD UNIQUE KEY uk_daily_free_quotas_fingerprint_date_type (fingerprint_hash, usage_date, document_type)'
);
PREPARE stmt FROM @add_daily_free_quota_type_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_daily_free_quota_type_index := IF(
  EXISTS(
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'daily_free_quotas' AND INDEX_NAME = 'idx_daily_free_quotas_document_type'
  ),
  'SELECT 1',
  'ALTER TABLE daily_free_quotas ADD KEY idx_daily_free_quotas_document_type (document_type)'
);
PREPARE stmt FROM @add_daily_free_quota_type_index;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
