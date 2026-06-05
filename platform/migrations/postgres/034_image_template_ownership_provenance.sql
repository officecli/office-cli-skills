ALTER TABLE image_prompt_templates
  ADD COLUMN IF NOT EXISTS owner_user_id BIGINT NULL,
  ADD COLUMN IF NOT EXISTS visibility VARCHAR(32) NOT NULL DEFAULT 'platform_public',
  ADD COLUMN IF NOT EXISTS source_template_id BIGINT NULL,
  ADD COLUMN IF NOT EXISTS generation_id BIGINT NULL;

CREATE INDEX IF NOT EXISTS idx_image_prompt_templates_owner_visibility
  ON image_prompt_templates (owner_user_id, visibility, sort_order, id);

CREATE TABLE IF NOT EXISTS image_generation_provenance (
  id BIGSERIAL PRIMARY KEY,
  request_id VARCHAR(128) NOT NULL UNIQUE,
  user_id BIGINT NOT NULL,
  prompt TEXT NOT NULL,
  prompt_sha256 VARCHAR(64) NOT NULL,
  image_sha256 VARCHAR(64) NOT NULL,
  image_storage_key VARCHAR(512) NOT NULL,
  image_content_type VARCHAR(128) NOT NULL DEFAULT '',
  image_original_name VARCHAR(191) NOT NULL DEFAULT '',
  source_template_id BIGINT NULL,
  source_template_prompt TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_image_generation_provenance_user
  ON image_generation_provenance (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_image_generation_provenance_prompt_image
  ON image_generation_provenance (prompt_sha256, image_sha256);

CREATE TABLE IF NOT EXISTS image_template_publish_requests (
  id BIGSERIAL PRIMARY KEY,
  private_template_id BIGINT NOT NULL,
  requester_user_id BIGINT NOT NULL,
  generation_id BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  submitter_note TEXT NOT NULL DEFAULT '',
  admin_notes TEXT NOT NULL DEFAULT '',
  reviewed_by VARCHAR(191) NOT NULL DEFAULT '',
  public_template_id BIGINT NULL,
  reviewed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_image_template_publish_requests_status
  ON image_template_publish_requests (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_image_template_publish_requests_requester
  ON image_template_publish_requests (requester_user_id, created_at DESC);
