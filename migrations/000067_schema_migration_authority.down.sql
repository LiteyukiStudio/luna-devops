-- This migration is intentionally irreversible. It removes obsolete columns
-- and converts startup-time data repair into a versioned, one-time operation;
-- rolling it back cannot restore discarded values safely.
DO $$
BEGIN
    RAISE EXCEPTION 'migration 000067 is irreversible: removed legacy schema and one-time repairs cannot be restored safely'
        USING ERRCODE = 'feature_not_supported';
END
$$;
