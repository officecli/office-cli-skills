ALTER TABLE api_keys
  ADD COLUMN IF NOT EXISTS key_ciphertext TEXT;
