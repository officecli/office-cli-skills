ALTER TABLE user_hosted_credit_accounts DROP COLUMN credit_reserved;
ALTER TABLE fingerprint_credit_accounts DROP COLUMN credit_reserved;
ALTER TABLE api_keys DROP COLUMN credit_reserved;
ALTER TABLE user_hosted_credit_ledger DROP COLUMN reserved_delta;
ALTER TABLE fingerprint_credit_ledger DROP COLUMN reserved_delta;
