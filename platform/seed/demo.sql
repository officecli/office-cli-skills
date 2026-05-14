INSERT INTO free_quotas (fingerprint_hash, free_limit, free_used)
VALUES ('demo-fingerprint', 5, 2)
ON CONFLICT (fingerprint_hash) DO UPDATE
SET free_limit = EXCLUDED.free_limit,
    free_used = EXCLUDED.free_used;
