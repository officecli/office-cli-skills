CREATE TABLE IF NOT EXISTS operational_events (
  id BIGSERIAL PRIMARY KEY,
  event_name VARCHAR(64) NOT NULL,
  surface VARCHAR(16) NOT NULL,
  visitor_id VARCHAR(96) NULL,
  fingerprint_hash VARCHAR(128) NULL,
  user_id BIGINT NULL,
  api_key_id BIGINT NULL,
  order_id BIGINT NULL,
  invite VARCHAR(191) NULL,
  utm_source VARCHAR(191) NULL,
  utm_medium VARCHAR(191) NULL,
  utm_campaign VARCHAR(191) NULL,
  utm_term VARCHAR(191) NULL,
  utm_content VARCHAR(191) NULL,
  utm_id VARCHAR(191) NULL,
  page_path VARCHAR(512) NULL,
  referrer VARCHAR(512) NULL,
  user_agent VARCHAR(512) NULL,
  metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_operational_events_event_name_created_at ON operational_events(event_name, created_at);
CREATE INDEX IF NOT EXISTS idx_operational_events_surface_created_at ON operational_events(surface, created_at);
CREATE INDEX IF NOT EXISTS idx_operational_events_visitor_id_created_at ON operational_events(visitor_id, created_at);
CREATE INDEX IF NOT EXISTS idx_operational_events_fingerprint_hash_created_at ON operational_events(fingerprint_hash, created_at);
CREATE INDEX IF NOT EXISTS idx_operational_events_user_id_created_at ON operational_events(user_id, created_at);

CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);
CREATE INDEX IF NOT EXISTS idx_cli_login_challenges_created_at ON cli_login_challenges(created_at);
CREATE INDEX IF NOT EXISTS idx_cli_login_challenges_completed_at ON cli_login_challenges(completed_at);
CREATE INDEX IF NOT EXISTS idx_cli_sessions_created_at ON cli_sessions(created_at);
CREATE INDEX IF NOT EXISTS idx_cli_sessions_user_id_created_at ON cli_sessions(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_created_at ON usage_events(created_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_fingerprint_created_at ON usage_events(fingerprint_hash, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_user_id_created_at ON usage_events(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_mode_created_at ON usage_events(mode, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_result_created_at ON usage_events(result, created_at);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at);
CREATE INDEX IF NOT EXISTS idx_orders_updated_at ON orders(updated_at);
CREATE INDEX IF NOT EXISTS idx_orders_status_updated_at ON orders(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_orders_user_id_status_updated_at ON orders(user_id, status, updated_at);
