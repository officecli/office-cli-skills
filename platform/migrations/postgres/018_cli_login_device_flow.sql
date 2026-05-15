ALTER TABLE cli_login_challenges
  ADD COLUMN IF NOT EXISTS flow VARCHAR(16) NOT NULL DEFAULT 'callback';

ALTER TABLE cli_login_challenges
  ADD COLUMN IF NOT EXISTS user_code_hash VARCHAR(128) NULL;

CREATE INDEX IF NOT EXISTS idx_cli_login_challenges_flow ON cli_login_challenges(flow);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cli_login_challenges_user_code_hash
  ON cli_login_challenges(user_code_hash)
  WHERE user_code_hash IS NOT NULL;
