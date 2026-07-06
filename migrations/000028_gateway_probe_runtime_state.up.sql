ALTER TABLE system_component_installations
  DROP COLUMN IF EXISTS last_reported_at,
  DROP COLUMN IF EXISTS last_window_start,
  DROP COLUMN IF EXISTS last_window_end;
