ALTER TABLE api_keys
  ADD COLUMN key_ciphertext TEXT NULL AFTER key_prefix;
