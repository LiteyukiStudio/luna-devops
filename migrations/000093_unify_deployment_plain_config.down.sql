ALTER TABLE deployment_targets
    ADD COLUMN config_refs text DEFAULT ''::text NOT NULL;
