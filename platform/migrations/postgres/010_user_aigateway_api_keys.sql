CREATE TABLE IF NOT EXISTS user_aigateway_api_keys (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL UNIQUE,
  key_ciphertext TEXT NOT NULL DEFAULT '',
  key_prefix VARCHAR(32) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL,
  upstream_id VARCHAR(128) NOT NULL DEFAULT '',
  upstream_name VARCHAR(191) NOT NULL,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_aigateway_api_keys_status ON user_aigateway_api_keys(status);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_user_aigateway_api_keys_updated_at') THEN
    CREATE TRIGGER set_user_aigateway_api_keys_updated_at BEFORE UPDATE ON user_aigateway_api_keys FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
END $$;
