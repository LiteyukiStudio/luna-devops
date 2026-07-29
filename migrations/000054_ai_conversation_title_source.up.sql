ALTER TABLE ai.conversations
    ADD COLUMN title_source text NOT NULL DEFAULT 'default'
    CHECK (title_source IN ('default', 'assistant', 'user'));

-- Older non-default titles may have been written by either a user or an early
-- auto-title implementation. Preserve them as user-owned rather than risk
-- overwriting a manually chosen title.
UPDATE ai.conversations
SET title_source = 'user'
WHERE title <> '新会话';
