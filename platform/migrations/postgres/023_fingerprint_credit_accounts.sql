CREATE TABLE IF NOT EXISTS fingerprint_credit_accounts (
  fingerprint_hash VARCHAR(128) PRIMARY KEY,
  credit_balance INTEGER NOT NULL DEFAULT 0,
  credit_reserved INTEGER NOT NULL DEFAULT 0,
  bonus_granted BOOLEAN NOT NULL DEFAULT FALSE,
  migrated_to_user_id BIGINT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_fingerprint_credit_accounts_migrated_to_user_id
  ON fingerprint_credit_accounts(migrated_to_user_id);

CREATE TABLE IF NOT EXISTS fingerprint_credit_ledger (
  id BIGSERIAL PRIMARY KEY,
  fingerprint_hash VARCHAR(128) NOT NULL,
  source_type VARCHAR(64) NOT NULL,
  idempotency_key VARCHAR(191) NOT NULL UNIQUE,
  credit_delta INTEGER NOT NULL DEFAULT 0,
  reserved_delta INTEGER NOT NULL DEFAULT 0,
  usage_event_id BIGINT NULL,
  order_id BIGINT NULL,
  reason VARCHAR(191) NOT NULL,
  metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_fingerprint_credit_ledger_fingerprint_hash
  ON fingerprint_credit_ledger(fingerprint_hash);
CREATE INDEX IF NOT EXISTS idx_fingerprint_credit_ledger_source_type
  ON fingerprint_credit_ledger(source_type);
CREATE INDEX IF NOT EXISTS idx_fingerprint_credit_ledger_usage_event_id
  ON fingerprint_credit_ledger(usage_event_id);
CREATE INDEX IF NOT EXISTS idx_fingerprint_credit_ledger_order_id
  ON fingerprint_credit_ledger(order_id);
