-- Intentionally non-destructive. Rolling back application code must not erase
-- Run identity bindings, approval decisions or user exemptions that may already
-- be referenced by immutable AI events.
SELECT 1;
