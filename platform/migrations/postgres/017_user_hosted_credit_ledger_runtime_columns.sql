ALTER TABLE user_hosted_credit_ledger
  ADD COLUMN IF NOT EXISTS usage_event_id BIGINT NULL,
  ADD COLUMN IF NOT EXISTS order_id BIGINT NULL;

CREATE INDEX IF NOT EXISTS idx_user_hosted_credit_ledger_usage_event_id ON user_hosted_credit_ledger(usage_event_id);
CREATE INDEX IF NOT EXISTS idx_user_hosted_credit_ledger_order_id ON user_hosted_credit_ledger(order_id);
