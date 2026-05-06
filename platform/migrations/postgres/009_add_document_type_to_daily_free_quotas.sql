-- Add document_type column to daily_free_quotas
ALTER TABLE daily_free_quotas ADD COLUMN IF NOT EXISTS document_type VARCHAR(32) NOT NULL DEFAULT 'document';

-- Update existing rows to have document_type = 'document'
UPDATE daily_free_quotas SET document_type = 'document' WHERE document_type IS NULL OR document_type = '';

-- Drop old unique constraint (name may vary by Postgres version)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'daily_free_quotas_fingerprint_hash_usage_date_key'
    ) THEN
        ALTER TABLE daily_free_quotas DROP CONSTRAINT daily_free_quotas_fingerprint_hash_usage_date_key;
    END IF;
    IF EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'idx_daily_free_quotas_fingerprint_date'
    ) THEN
        ALTER TABLE daily_free_quotas DROP CONSTRAINT idx_daily_free_quotas_fingerprint_date;
    END IF;
END $$;

-- Add new unique constraint with document_type
ALTER TABLE daily_free_quotas ADD CONSTRAINT daily_free_quotas_fingerprint_date_type_key UNIQUE (fingerprint_hash, usage_date, document_type);
