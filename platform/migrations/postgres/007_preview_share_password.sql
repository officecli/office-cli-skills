ALTER TABLE preview_shares
    ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';
