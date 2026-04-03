INSERT INTO free_quotas (fingerprint_hash, free_limit, free_used)
VALUES ('demo-fingerprint', 10, 2)
ON DUPLICATE KEY UPDATE free_limit = VALUES(free_limit), free_used = VALUES(free_used);
