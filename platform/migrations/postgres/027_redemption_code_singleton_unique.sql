-- 027_redemption_code_singleton_unique.sql
--
-- Defense-in-depth: ensure that when a redemption code has per_user_limit = 1
-- no single user can ever end up with more than one redemption row against
-- that code, even if the application-layer FOR UPDATE serialization ever
-- breaks (refactor mistake, replica routing bug, etc).
--
-- We denormalize per_user_limit into the redemption row as a snapshot at the
-- time of redemption (codes can be edited later, but historical rows must
-- reflect the rule that applied at claim time). Then a partial UNIQUE index
-- constrains only the singleton case; multi-claim codes are unaffected.
--
-- Forward-only and idempotent.

ALTER TABLE redemption_code_redemptions
  ADD COLUMN IF NOT EXISTS per_user_limit_at_claim INTEGER NOT NULL DEFAULT 1;

-- Partial unique index: only enforces uniqueness on (code_id, user_id) for
-- claims that were captured under per_user_limit = 1.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_redemption_code_redemptions_singleton
  ON redemption_code_redemptions (redemption_code_id, user_id)
  WHERE per_user_limit_at_claim = 1;
