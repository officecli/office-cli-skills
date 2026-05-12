ALTER TABLE hosted_pricing_settings ADD COLUMN IF NOT EXISTS credits_per_usd INT NOT NULL DEFAULT 100;

UPDATE hosted_pricing_settings
SET credits_per_usd = 100
WHERE credits_per_usd <= 0;
