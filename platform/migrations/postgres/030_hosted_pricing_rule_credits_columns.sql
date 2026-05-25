-- Persist HostedPricingRule *Credits fields. Previously tagged `gorm:"-"`
-- in model.HostedPricingRule, these were never stored in DB, so the
-- per-image fast-path in priceUsage (service.go) never triggered in prod.
-- After this migration, image rules use ImagePerAssetCredits directly.
ALTER TABLE hosted_pricing_rules
    ADD COLUMN IF NOT EXISTS prompt_per_1k_credits    INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS output_per_1k_credits    INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS reasoning_per_1k_credits INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS image_per_asset_credits  INTEGER NOT NULL DEFAULT 0;

-- Seed: pin existing image rule to 10 credits/asset with a 10-credit floor.
-- Guarded by image_per_asset_credits = 0 so re-runs / manual edits are preserved.
UPDATE hosted_pricing_rules
SET image_per_asset_credits = 10,
    minimum_charge_credits  = 10
WHERE document_profile = 'image'
  AND image_per_asset_credits = 0;
