CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_slug_active ON projects (slug) WHERE deleted_at IS NULL;

ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_project_id_slug_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_applications_project_slug_active ON applications (project_id, slug) WHERE deleted_at IS NULL;
