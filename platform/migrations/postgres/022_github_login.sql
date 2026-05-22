ALTER TABLE users ALTER COLUMN google_sub DROP NOT NULL;

ALTER TABLE users ADD COLUMN IF NOT EXISTS github_sub VARCHAR(191) NULL;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname = current_schema()
      AND tablename = 'users'
      AND indexname = 'users_google_sub_key'
  ) THEN
    EXECUTE 'ALTER TABLE users DROP CONSTRAINT IF EXISTS users_google_sub_key';
    EXECUTE 'DROP INDEX IF EXISTS users_google_sub_key';
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uk_users_google_sub
  ON users(google_sub)
  WHERE google_sub IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uk_users_github_sub
  ON users(github_sub)
  WHERE github_sub IS NOT NULL;
