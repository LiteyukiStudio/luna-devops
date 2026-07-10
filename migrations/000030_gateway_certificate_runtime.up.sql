ALTER TABLE gateway_routes
    ADD COLUMN IF NOT EXISTS certificate_message TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS certificate_not_after TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS certificate_issuer_kind TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS certificate_issuer_name TEXT NOT NULL DEFAULT '';
