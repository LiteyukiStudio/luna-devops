ALTER TABLE access_tokens
    ADD COLUMN oauth_family_id text DEFAULT ''::text NOT NULL;

UPDATE access_tokens
SET oauth_family_id = oauth_grant_id
WHERE oauth_family_id = ''
  AND oauth_grant_id <> '';

CREATE INDEX idx_access_tokens_oauth_family_id
    ON access_tokens USING btree (oauth_family_id)
    WHERE oauth_family_id <> '';

ALTER TABLE oauth_refresh_tokens
    ADD COLUMN family_id text DEFAULT ''::text NOT NULL;

UPDATE oauth_refresh_tokens
SET family_id = grant_id
WHERE family_id = '';

CREATE INDEX idx_oauth_refresh_tokens_family_id
    ON oauth_refresh_tokens USING btree (family_id);

ALTER TABLE oauth_authorization_codes
    ALTER COLUMN grant_id DROP NOT NULL;
