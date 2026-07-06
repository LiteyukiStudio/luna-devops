ALTER TABLE system_component_installations
  ADD COLUMN IF NOT EXISTS last_reported_at timestamptz,
  ADD COLUMN IF NOT EXISTS last_window_start timestamptz,
  ADD COLUMN IF NOT EXISTS last_window_end timestamptz;
