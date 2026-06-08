CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  google_sub VARCHAR(191) NOT NULL UNIQUE,
  email VARCHAR(191) NOT NULL UNIQUE,
  name VARCHAR(191) NOT NULL,
  invite_code VARCHAR(64) NOT NULL UNIQUE,
  avatar_url VARCHAR(512) NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  paid_entitlement BOOLEAN NOT NULL DEFAULT FALSE,
  paid_entitlement_updated_at TIMESTAMPTZ NULL,
  paid_entitlement_source VARCHAR(32) NOT NULL DEFAULT 'none',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);

CREATE TABLE IF NOT EXISTS api_keys (
  id BIGSERIAL PRIMARY KEY,
  owner_user_id BIGINT NULL,
  key_hash VARCHAR(128) NOT NULL UNIQUE,
  key_prefix VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  plan_name VARCHAR(128) NOT NULL,
  plan_code VARCHAR(64) NULL,
  allowed_modes VARCHAR(32) NOT NULL DEFAULT 'external_only',
  hosted_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  default_runtime_mode VARCHAR(16) NULL,
  expires_at TIMESTAMPTZ NULL,
  note TEXT NULL,
  last_used_at TIMESTAMPTZ NULL,
  quota_total INT NULL,
  quota_used INT NOT NULL DEFAULT 0,
  credit_balance INT NOT NULL DEFAULT 0,
  credit_reserved INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_owner_user_id ON api_keys(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_prefix ON api_keys(key_prefix);
CREATE INDEX IF NOT EXISTS idx_api_keys_status ON api_keys(status);

CREATE TABLE IF NOT EXISTS free_quotas (
  id BIGSERIAL PRIMARY KEY,
  fingerprint_hash VARCHAR(128) NOT NULL UNIQUE,
  free_limit INT NOT NULL,
  free_used INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS daily_free_quotas (
  id BIGSERIAL PRIMARY KEY,
  fingerprint_hash VARCHAR(128) NOT NULL,
  usage_date VARCHAR(10) NOT NULL,
  document_type VARCHAR(32) NOT NULL DEFAULT 'document',
  daily_limit INT NOT NULL,
  daily_used INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT uk_daily_free_quotas_fingerprint_date_type UNIQUE (fingerprint_hash, usage_date, document_type)
);
CREATE INDEX IF NOT EXISTS idx_daily_free_quotas_fingerprint ON daily_free_quotas(fingerprint_hash);
CREATE INDEX IF NOT EXISTS idx_daily_free_quotas_document_type ON daily_free_quotas(document_type);

CREATE TABLE IF NOT EXISTS usage_events (
  id BIGSERIAL PRIMARY KEY,
  request_id VARCHAR(128) NULL UNIQUE,
  fingerprint_hash VARCHAR(128) NOT NULL,
  mode VARCHAR(16) NOT NULL,
  action VARCHAR(16) NOT NULL,
  api_key_id BIGINT NULL,
  result VARCHAR(16) NOT NULL,
  reason_code VARCHAR(64) NULL,
  cli_version VARCHAR(64) NULL,
  document_type VARCHAR(32) NULL,
  runtime_mode VARCHAR(16) NULL,
  user_id BIGINT NULL,
  billed_units INT NOT NULL DEFAULT 0,
  unit_type VARCHAR(32) NOT NULL DEFAULT 'document',
  charged BOOLEAN NOT NULL DEFAULT FALSE,
  provider VARCHAR(64) NULL,
  model_name VARCHAR(128) NULL,
  prompt_tokens INT NOT NULL DEFAULT 0,
  completion_tokens INT NOT NULL DEFAULT 0,
  reasoning_tokens INT NOT NULL DEFAULT 0,
  image_count INT NOT NULL DEFAULT 0,
  reserved_credits INT NOT NULL DEFAULT 0,
  settled_credits INT NOT NULL DEFAULT 0,
  refund_credits INT NOT NULL DEFAULT 0,
  client_ip VARCHAR(64) NULL,
  forwarded_for VARCHAR(512) NULL,
  user_agent VARCHAR(512) NULL,
  request_host VARCHAR(191) NULL,
  request_path VARCHAR(191) NULL,
  request_method VARCHAR(16) NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_usage_events_fingerprint ON usage_events(fingerprint_hash);
CREATE INDEX IF NOT EXISTS idx_usage_events_mode ON usage_events(mode);
CREATE INDEX IF NOT EXISTS idx_usage_events_action ON usage_events(action);
CREATE INDEX IF NOT EXISTS idx_usage_events_result ON usage_events(result);
CREATE INDEX IF NOT EXISTS idx_usage_events_reason_code ON usage_events(reason_code);
CREATE INDEX IF NOT EXISTS idx_usage_events_api_key_id ON usage_events(api_key_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_runtime_mode ON usage_events(runtime_mode);
CREATE INDEX IF NOT EXISTS idx_usage_events_user_id ON usage_events(user_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_client_ip ON usage_events(client_ip);

CREATE TABLE IF NOT EXISTS reward_grants (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  source_type VARCHAR(64) NOT NULL,
  idempotency_key VARCHAR(191) NOT NULL UNIQUE,
  amount_total INT NOT NULL,
  amount_used INT NOT NULL DEFAULT 0,
  reason VARCHAR(191) NOT NULL,
  metadata_json JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reward_grants_user_id ON reward_grants(user_id);
CREATE INDEX IF NOT EXISTS idx_reward_grants_source_type ON reward_grants(source_type);

CREATE TABLE IF NOT EXISTS user_referrals (
  id BIGSERIAL PRIMARY KEY,
  inviter_user_id BIGINT NOT NULL,
  invited_user_id BIGINT NOT NULL UNIQUE,
  invite_code VARCHAR(64) NOT NULL,
  registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  activated_at TIMESTAMPTZ NULL,
  reward_granted_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_referrals_inviter_user_id ON user_referrals(inviter_user_id);
CREATE INDEX IF NOT EXISTS idx_user_referrals_invite_code ON user_referrals(invite_code);

CREATE TABLE IF NOT EXISTS discord_connections (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL UNIQUE,
  discord_user_id VARCHAR(191) NOT NULL UNIQUE,
  username VARCHAR(191) NOT NULL,
  guild_member BOOLEAN NOT NULL DEFAULT FALSE,
  connected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  reward_granted_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS stripe_customers (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL UNIQUE,
  stripe_customer_id VARCHAR(191) NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS orders (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  stripe_checkout_session_id VARCHAR(191) NULL UNIQUE,
  stripe_payment_intent_id VARCHAR(191) NULL,
  stripe_customer_id VARCHAR(191) NULL,
  status VARCHAR(16) NOT NULL,
  currency VARCHAR(16) NOT NULL,
  amount_total BIGINT NOT NULL,
  pack_code VARCHAR(64) NOT NULL,
  pack_name VARCHAR(128) NOT NULL,
  pack_kind VARCHAR(32) NOT NULL DEFAULT 'external_generation',
  quota_amount INT NOT NULL,
  credit_amount INT NOT NULL DEFAULT 0,
  target_api_key_id BIGINT NULL,
  note TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_pack_code ON orders(pack_code);
CREATE INDEX IF NOT EXISTS idx_orders_stripe_payment_intent_id ON orders(stripe_payment_intent_id);
CREATE INDEX IF NOT EXISTS idx_orders_stripe_customer_id ON orders(stripe_customer_id);
CREATE INDEX IF NOT EXISTS idx_orders_target_api_key_id ON orders(target_api_key_id);

CREATE TABLE IF NOT EXISTS billing_events (
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NULL,
  event_id VARCHAR(191) NOT NULL UNIQUE,
  event_type VARCHAR(128) NOT NULL,
  status VARCHAR(16) NOT NULL,
  payload_json JSONB NOT NULL,
  error_message TEXT NULL,
  processed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_billing_events_order_id ON billing_events(order_id);
CREATE INDEX IF NOT EXISTS idx_billing_events_event_type ON billing_events(event_type);
CREATE INDEX IF NOT EXISTS idx_billing_events_status ON billing_events(status);

CREATE TABLE IF NOT EXISTS admin_audit_logs (
  id BIGSERIAL PRIMARY KEY,
  action VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id VARCHAR(128) NOT NULL,
  payload_json JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_action ON admin_audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_target_type ON admin_audit_logs(target_type);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_target_id ON admin_audit_logs(target_id);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_users_updated_at') THEN
    CREATE TRIGGER set_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_api_keys_updated_at') THEN
    CREATE TRIGGER set_api_keys_updated_at BEFORE UPDATE ON api_keys FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_free_quotas_updated_at') THEN
    CREATE TRIGGER set_free_quotas_updated_at BEFORE UPDATE ON free_quotas FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_daily_free_quotas_updated_at') THEN
    CREATE TRIGGER set_daily_free_quotas_updated_at BEFORE UPDATE ON daily_free_quotas FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_reward_grants_updated_at') THEN
    CREATE TRIGGER set_reward_grants_updated_at BEFORE UPDATE ON reward_grants FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_user_referrals_updated_at') THEN
    CREATE TRIGGER set_user_referrals_updated_at BEFORE UPDATE ON user_referrals FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_discord_connections_updated_at') THEN
    CREATE TRIGGER set_discord_connections_updated_at BEFORE UPDATE ON discord_connections FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_stripe_customers_updated_at') THEN
    CREATE TRIGGER set_stripe_customers_updated_at BEFORE UPDATE ON stripe_customers FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_orders_updated_at') THEN
    CREATE TRIGGER set_orders_updated_at BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
END $$;
