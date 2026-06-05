UPDATE usage_events
JOIN fingerprint_credit_accounts
  ON usage_events.fingerprint_hash = fingerprint_credit_accounts.fingerprint_hash
SET usage_events.user_id = fingerprint_credit_accounts.migrated_to_user_id
WHERE fingerprint_credit_accounts.migrated_to_user_id IS NOT NULL
  AND fingerprint_credit_accounts.migrated_to_user_id <> 0
  AND (usage_events.user_id IS NULL OR usage_events.user_id = 0);
