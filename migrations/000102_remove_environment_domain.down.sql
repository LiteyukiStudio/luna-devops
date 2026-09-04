ALTER TABLE deployment_targets ADD COLUMN environment_id text DEFAULT ''::text NOT NULL;
UPDATE deployment_targets SET environment_id = id;

ALTER TABLE gateway_routes ADD COLUMN environment_id text DEFAULT ''::text NOT NULL;
UPDATE gateway_routes SET environment_id = deployment_target_id;

ALTER TABLE hook_runs ADD COLUMN environment_id text DEFAULT ''::text NOT NULL;
UPDATE hook_runs SET environment_id = deployment_target_id;

ALTER TABLE releases ADD COLUMN environment_id text DEFAULT ''::text NOT NULL;
UPDATE releases SET environment_id = deployment_target_id;
ALTER TABLE releases ALTER COLUMN environment_id DROP DEFAULT;

CREATE UNIQUE INDEX idx_deployment_targets_app_env_name_active ON deployment_targets USING btree (application_id, environment_id, name) WHERE (deleted_at IS NULL);
CREATE INDEX idx_deployment_targets_environment_id ON deployment_targets USING btree (environment_id);
CREATE INDEX idx_gateway_routes_environment_id ON gateway_routes USING btree (environment_id);
CREATE INDEX idx_hook_runs_environment_id ON hook_runs USING btree (environment_id);
CREATE INDEX idx_releases_environment_id ON releases USING btree (environment_id);
