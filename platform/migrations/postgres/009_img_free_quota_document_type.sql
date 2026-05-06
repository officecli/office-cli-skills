ALTER TABLE daily_free_quotas
  ADD COLUMN IF NOT EXISTS document_type VARCHAR(32) NOT NULL DEFAULT 'document';

ALTER TABLE daily_free_quotas
  DROP CONSTRAINT IF EXISTS uk_daily_free_quotas_fingerprint_date;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'uk_daily_free_quotas_fingerprint_date_type'
  ) THEN
    ALTER TABLE daily_free_quotas
      ADD CONSTRAINT uk_daily_free_quotas_fingerprint_date_type UNIQUE (fingerprint_hash, usage_date, document_type);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_daily_free_quotas_document_type ON daily_free_quotas(document_type);
