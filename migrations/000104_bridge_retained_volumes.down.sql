-- Intentionally irreversible. Once exposed through the volume center, a
-- migrated retained volume can acquire bindings or user-authored changes.
-- Removing that durable identity during rollback would orphan it again.
SELECT 1;
