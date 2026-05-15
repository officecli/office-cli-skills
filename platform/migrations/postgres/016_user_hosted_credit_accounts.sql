CREATE TABLE IF NOT EXISTS user_hosted_credit_accounts (
  user_id BIGINT PRIMARY KEY,
  credit_balance INTEGER NOT NULL DEFAULT 0,
  credit_reserved INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_hosted_credit_ledger (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
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

CREATE INDEX IF NOT EXISTS idx_user_hosted_credit_ledger_user_id ON user_hosted_credit_ledger(user_id);
CREATE INDEX IF NOT EXISTS idx_user_hosted_credit_ledger_source_type ON user_hosted_credit_ledger(source_type);
CREATE INDEX IF NOT EXISTS idx_user_hosted_credit_ledger_usage_event_id ON user_hosted_credit_ledger(usage_event_id);
CREATE INDEX IF NOT EXISTS idx_user_hosted_credit_ledger_order_id ON user_hosted_credit_ledger(order_id);

CREATE TABLE IF NOT EXISTS cli_login_challenges (
  id BIGSERIAL PRIMARY KEY,
  challenge_id VARCHAR(128) NOT NULL UNIQUE,
  flow VARCHAR(16) NOT NULL DEFAULT 'callback',
  code_challenge VARCHAR(191) NOT NULL,
  code_challenge_method VARCHAR(16) NOT NULL,
  redirect_uri VARCHAR(512) NOT NULL,
  state VARCHAR(191) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  user_id BIGINT NULL,
  user_code_hash VARCHAR(128) NULL UNIQUE,
  exchange_code_hash VARCHAR(128) NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ NULL,
  consumed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cli_login_challenges_flow ON cli_login_challenges(flow);
CREATE INDEX IF NOT EXISTS idx_cli_login_challenges_user_id ON cli_login_challenges(user_id);
CREATE INDEX IF NOT EXISTS idx_cli_login_challenges_expires_at ON cli_login_challenges(expires_at);

CREATE TABLE IF NOT EXISTS cli_sessions (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  token_hash VARCHAR(128) NOT NULL UNIQUE,
  token_prefix VARCHAR(32) NOT NULL,
  name VARCHAR(191) NOT NULL,
  revoked_at TIMESTAMPTZ NULL,
  last_used_at TIMESTAMPTZ NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cli_sessions_user_id ON cli_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_cli_sessions_expires_at ON cli_sessions(expires_at);

WITH migrated AS (
  SELECT
    owner_user_id AS user_id,
    SUM(GREATEST(credit_balance - credit_reserved, 0))::INTEGER AS credits
  FROM api_keys
  WHERE owner_user_id IS NOT NULL
    AND GREATEST(credit_balance - credit_reserved, 0) > 0
  GROUP BY owner_user_id
),
upserted AS (
  INSERT INTO user_hosted_credit_accounts (user_id, credit_balance, credit_reserved, created_at, updated_at)
  SELECT user_id, credits, 0, NOW(), NOW()
  FROM migrated
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
  migrated.user_id,
  'migration',
  'migration:api_keys:' || migrated.user_id,
  migrated.credits,
  0,
  'migrated hosted credits from api key balances',
  jsonb_build_object('source', 'api_keys.credit_balance', 'owner_user_id', migrated.user_id),
  NOW()
FROM migrated
ON CONFLICT (idempotency_key) DO NOTHING;
