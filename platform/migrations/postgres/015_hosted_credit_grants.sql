CREATE TABLE IF NOT EXISTS hosted_credit_grants (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  api_key_id BIGINT NOT NULL,
  source_type VARCHAR(64) NOT NULL,
  idempotency_key VARCHAR(191) NOT NULL UNIQUE,
  credit_amount INTEGER NOT NULL,
  reason VARCHAR(191) NOT NULL,
  metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hosted_credit_grants_user_id ON hosted_credit_grants(user_id);
CREATE INDEX IF NOT EXISTS idx_hosted_credit_grants_api_key_id ON hosted_credit_grants(api_key_id);
CREATE INDEX IF NOT EXISTS idx_hosted_credit_grants_source_type ON hosted_credit_grants(source_type);
