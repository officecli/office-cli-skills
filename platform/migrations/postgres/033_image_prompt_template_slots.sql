ALTER TABLE image_prompt_templates
  ADD COLUMN IF NOT EXISTS slots JSON DEFAULT '[]'::json;
