ALTER TABLE git_oauth_states
  ADD COLUMN IF NOT EXISTS callback_origin text NOT NULL DEFAULT '';
