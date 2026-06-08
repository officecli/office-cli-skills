SET @add_paid_entitlement := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'paid_entitlement'
  ),
  'SELECT 1',
  'ALTER TABLE users ADD COLUMN paid_entitlement TINYINT(1) NOT NULL DEFAULT 0 AFTER status'
);
PREPARE stmt FROM @add_paid_entitlement;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_paid_entitlement_updated_at := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'paid_entitlement_updated_at'
  ),
  'SELECT 1',
  'ALTER TABLE users ADD COLUMN paid_entitlement_updated_at DATETIME NULL AFTER paid_entitlement'
);
PREPARE stmt FROM @add_paid_entitlement_updated_at;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_paid_entitlement_source := IF(
  EXISTS(
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'paid_entitlement_source'
  ),
  'SELECT 1',
  'ALTER TABLE users ADD COLUMN paid_entitlement_source VARCHAR(32) NOT NULL DEFAULT ''none'' AFTER paid_entitlement_updated_at'
);
PREPARE stmt FROM @add_paid_entitlement_source;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
