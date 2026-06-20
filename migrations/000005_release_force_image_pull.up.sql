ALTER TABLE releases
  ADD COLUMN IF NOT EXISTS force_image_pull boolean NOT NULL DEFAULT false;
