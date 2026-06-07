DROP INDEX IF EXISTS idx_applications_project_slug_active;

ALTER TABLE applications ADD CONSTRAINT applications_project_id_slug_key UNIQUE (project_id, slug);
