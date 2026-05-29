WITH eligible_users AS (
  SELECT users.id AS user_id
  FROM users
  LEFT JOIN user_hosted_credit_ledger
    ON user_hosted_credit_ledger.idempotency_key = 'signup-hosted-credits:' || users.id::text
  WHERE users.status = 'active'
    AND user_hosted_credit_ledger.idempotency_key IS NULL
),
ledger_insert AS (
  INSERT INTO user_hosted_credit_ledger (
    user_id,
    source_type,
    idempotency_key,
    credit_delta,
    reason,
    metadata_json,
    created_at
  )
  SELECT
    eligible_users.user_id,
    'signup_bonus',
    'signup-hosted-credits:' || eligible_users.user_id::text,
    100,
    'new user signup hosted credits',
    jsonb_build_object(
      'backfill', 'backfill_signup_hosted_credits_20260528',
      'user_id', eligible_users.user_id,
      'bonus', 'new_user_signup'
    ),
    NOW()
  FROM eligible_users
  ON CONFLICT (idempotency_key) DO NOTHING
  RETURNING user_id
)
INSERT INTO user_hosted_credit_accounts (
  user_id,
  credit_balance,
  created_at,
  updated_at
)
SELECT
  ledger_insert.user_id,
  100,
  NOW(),
  NOW()
FROM ledger_insert
ON CONFLICT (user_id) DO UPDATE SET
  credit_balance = user_hosted_credit_accounts.credit_balance + EXCLUDED.credit_balance,
  updated_at = NOW();
