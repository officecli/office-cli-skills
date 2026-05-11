CREATE TABLE IF NOT EXISTS hosted_pricing_settings (
  id BIGSERIAL PRIMARY KEY,
  markup_bps INT NOT NULL DEFAULT 3000,
  currency VARCHAR(8) NOT NULL DEFAULT 'usd',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO hosted_pricing_settings (id, markup_bps, currency)
VALUES (1, 3000, 'usd')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS hosted_pricing_rules (
  id BIGSERIAL PRIMARY KEY,
  document_profile VARCHAR(64) NOT NULL,
  provider VARCHAR(64) NOT NULL,
  model VARCHAR(128) NOT NULL,
  prompt_per_1k_cost_microusd BIGINT NOT NULL DEFAULT 0,
  output_per_1k_cost_microusd BIGINT NOT NULL DEFAULT 0,
  reasoning_per_1k_cost_microusd BIGINT NOT NULL DEFAULT 0,
  image_per_asset_cost_microusd BIGINT NOT NULL DEFAULT 0,
  reservation_credits INT NOT NULL DEFAULT 0,
  minimum_charge_credits INT NOT NULL DEFAULT 0,
  markup_bps INT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_hosted_pricing_rules_profile ON hosted_pricing_rules(document_profile);
CREATE INDEX IF NOT EXISTS idx_hosted_pricing_rules_enabled ON hosted_pricing_rules(enabled);

CREATE TABLE IF NOT EXISTS hosted_credit_packs (
  id BIGSERIAL PRIMARY KEY,
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description VARCHAR(512) NOT NULL,
  currency VARCHAR(8) NOT NULL DEFAULT 'usd',
  amount_total BIGINT NOT NULL,
  credit_amount INT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_hosted_credit_packs_enabled ON hosted_credit_packs(enabled);

ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS hosted_pricing_rule_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS markup_bps INT NOT NULL DEFAULT 0;
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS upstream_cost_microusd BIGINT NOT NULL DEFAULT 0;
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS uncapped_charge_credits INT NOT NULL DEFAULT 0;
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS profit_microusd BIGINT NOT NULL DEFAULT 0;
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS cap_applied BOOLEAN NOT NULL DEFAULT FALSE;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_hosted_pricing_settings_updated_at') THEN
    CREATE TRIGGER set_hosted_pricing_settings_updated_at BEFORE UPDATE ON hosted_pricing_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_hosted_pricing_rules_updated_at') THEN
    CREATE TRIGGER set_hosted_pricing_rules_updated_at BEFORE UPDATE ON hosted_pricing_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_hosted_credit_packs_updated_at') THEN
    CREATE TRIGGER set_hosted_credit_packs_updated_at BEFORE UPDATE ON hosted_credit_packs FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
END;
$$;
