UPDATE usage_events
SET user_id = fingerprint_credit_accounts.migrated_to_user_id
FROM fingerprint_credit_accounts
WHERE usage_events.fingerprint_hash = fingerprint_credit_accounts.fingerprint_hash
  AND fingerprint_credit_accounts.migrated_to_user_id IS NOT NULL
  AND fingerprint_credit_accounts.migrated_to_user_id <> 0
  AND (usage_events.user_id IS NULL OR usage_events.user_id = 0);
