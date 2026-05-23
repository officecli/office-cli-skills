-- One-time retroactive bonus: grant 100 hosted credits to every existing user
-- as a follow-up to bumping the new-signup hosted credit bonus from 30 to 100.
-- Forward-only and idempotent: re-running this migration is a no-op for users
-- that already received this specific bonus, thanks to the unique
-- idempotency_key on user_hosted_credit_ledger.

WITH eligible_users AS (
  SELECT id AS user_id
  FROM users
),
account_upsert AS (
  INSERT INTO user_hosted_credit_accounts (user_id, credit_balance, credit_reserved, created_at, updated_at)
  SELECT
    eligible_users.user_id,
    100,
    0,
    NOW(),
    NOW()
  FROM eligible_users
  LEFT JOIN user_hosted_credit_ledger ledger
    ON ledger.idempotency_key = 'signup-bonus-bump-100:' || eligible_users.user_id
  WHERE ledger.idempotency_key IS NULL
  ON CONFLICT (user_id) DO UPDATE SET
    credit_balance = user_hosted_credit_accounts.credit_balance + EXCLUDED.credit_balance,
    updated_at = NOW()
  RETURNING user_id
)
INSERT INTO user_hosted_credit_ledger (
  user_id,
  source_type,
  idempotency_key,
  credit_delta,
  reserved_delta,
  reason,
  metadata_json,
  created_at
)
SELECT
  account_upsert.user_id,
  'migration',
  'signup-bonus-bump-100:' || account_upsert.user_id,
  100,
  0,
  'retroactive signup credit bonus bump (30 -> 100)',
  jsonb_build_object(
    'bonus', 'signup_bonus_bump_100',
    'user_id', account_upsert.user_id,
    'previous_signup_bonus', 30,
    'new_signup_bonus', 100
  ),
  NOW()
FROM account_upsert
ON CONFLICT (idempotency_key) DO NOTHING;
