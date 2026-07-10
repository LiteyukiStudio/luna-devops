ALTER TABLE gateway_routes
    DROP COLUMN IF EXISTS certificate_issuer_name,
    DROP COLUMN IF EXISTS certificate_issuer_kind,
    DROP COLUMN IF EXISTS certificate_not_after,
    DROP COLUMN IF EXISTS certificate_message;
