CREATE TABLE IF NOT EXISTS admin_user_preferences (
  id BIGSERIAL PRIMARY KEY,
  admin_email VARCHAR(191) NOT NULL,
  page_key VARCHAR(128) NOT NULL,
  preferences_json JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT idx_admin_user_preferences_email_page UNIQUE (admin_email, page_key)
);
