CREATE TABLE IF NOT EXISTS platform_app_templates (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL,
    icon TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL DEFAULT '',
    is_multi_component BOOLEAN NOT NULL DEFAULT FALSE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    featured BOOLEAN NOT NULL DEFAULT FALSE,
    popularity_weight INTEGER NOT NULL DEFAULT 0,
    spec_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_app_templates_slug
    ON platform_app_templates (slug);

CREATE INDEX IF NOT EXISTS idx_platform_app_templates_enabled_category
    ON platform_app_templates (enabled, category);

CREATE INDEX IF NOT EXISTS idx_platform_app_templates_created_by
    ON platform_app_templates (created_by);
