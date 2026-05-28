CREATE TABLE IF NOT EXISTS image_prompt_templates (
  id BIGSERIAL PRIMARY KEY,
  slug VARCHAR(128) NOT NULL UNIQUE,
  title VARCHAR(128) NOT NULL,
  description VARCHAR(512) NOT NULL DEFAULT '',
  prompt_preset TEXT NOT NULL,
  thumbnail_storage_key VARCHAR(512) NOT NULL DEFAULT '',
  thumbnail_content_type VARCHAR(128) NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_image_prompt_templates_enabled_sort
  ON image_prompt_templates (enabled, sort_order, id);
