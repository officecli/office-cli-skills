-- Phase 6: drop credit_reserved column from all three account tables and
-- reserved_delta from both ledger tables. Hosted credit lifecycle moved to
-- charge-only in 0.2.93; legacy Reserve/Settle/Release paths deleted in 0.2.95.
ALTER TABLE user_hosted_credit_accounts DROP COLUMN IF EXISTS credit_reserved;
ALTER TABLE fingerprint_credit_accounts DROP COLUMN IF EXISTS credit_reserved;
ALTER TABLE api_keys DROP COLUMN IF EXISTS credit_reserved;
ALTER TABLE user_hosted_credit_ledger DROP COLUMN IF EXISTS reserved_delta;
ALTER TABLE fingerprint_credit_ledger DROP COLUMN IF EXISTS reserved_delta;
