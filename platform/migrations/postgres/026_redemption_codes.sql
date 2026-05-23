-- 026_redemption_codes.sql
--
-- Adds a redemption-code system that grants hosted credits to users.
--
-- Tables:
--   redemption_codes              admin-managed redeemable codes
--   redemption_code_redemptions   per-user, per-code redemption events
--
-- Forward-only and idempotent: all DDL uses IF NOT EXISTS guards.

CREATE TABLE IF NOT EXISTS redemption_codes (
  id BIGSERIAL PRIMARY KEY,
  code VARCHAR(64) NOT NULL UNIQUE,
  credit_amount INTEGER NOT NULL CHECK (credit_amount > 0),
  max_redemptions INTEGER NULL CHECK (max_redemptions IS NULL OR max_redemptions > 0),
  redemptions_used INTEGER NOT NULL DEFAULT 0 CHECK (redemptions_used >= 0),
  per_user_limit INTEGER NOT NULL DEFAULT 1 CHECK (per_user_limit >= 1),
  status VARCHAR(16) NOT NULL DEFAULT 'disabled' CHECK (status IN ('enabled', 'disabled')),
  expires_at TIMESTAMPTZ NULL,
  notes TEXT NULL,
  created_by VARCHAR(191) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_redemption_codes_status ON redemption_codes(status);
CREATE INDEX IF NOT EXISTS idx_redemption_codes_expires_at ON redemption_codes(expires_at);
CREATE INDEX IF NOT EXISTS idx_redemption_codes_created_at ON redemption_codes(created_at);

CREATE TABLE IF NOT EXISTS redemption_code_redemptions (
  id BIGSERIAL PRIMARY KEY,
  redemption_code_id BIGINT NOT NULL,
  code VARCHAR(64) NOT NULL,
  user_id BIGINT NOT NULL,
  credit_amount INTEGER NOT NULL CHECK (credit_amount >= 0),
  ledger_entry_id BIGINT NOT NULL,
  source VARCHAR(16) NOT NULL,
  client_ip VARCHAR(64) NOT NULL DEFAULT '',
  user_agent VARCHAR(512) NOT NULL DEFAULT '',
  idempotency_key VARCHAR(191) NOT NULL UNIQUE,
  redeemed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_redemption_code_redemptions_code_id ON redemption_code_redemptions(redemption_code_id);
CREATE INDEX IF NOT EXISTS idx_redemption_code_redemptions_user_id ON redemption_code_redemptions(user_id);
CREATE INDEX IF NOT EXISTS idx_redemption_code_redemptions_code ON redemption_code_redemptions(code);
CREATE INDEX IF NOT EXISTS idx_redemption_code_redemptions_redeemed_at ON redemption_code_redemptions(redeemed_at);
CREATE INDEX IF NOT EXISTS idx_redemption_code_redemptions_source ON redemption_code_redemptions(source);
