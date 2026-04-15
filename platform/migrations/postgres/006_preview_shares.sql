CREATE TABLE IF NOT EXISTS preview_shares (
    id BIGSERIAL PRIMARY KEY,
    share_token TEXT NOT NULL UNIQUE,
    file_id TEXT NOT NULL UNIQUE,
    storage_key TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_type TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    last_access_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_preview_shares_storage_key ON preview_shares (storage_key);
CREATE INDEX IF NOT EXISTS idx_preview_shares_expires_at ON preview_shares (expires_at);
CREATE INDEX IF NOT EXISTS idx_preview_shares_status ON preview_shares (status);
