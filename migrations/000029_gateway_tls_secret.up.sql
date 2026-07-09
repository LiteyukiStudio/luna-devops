ALTER TABLE IF EXISTS runtime_clusters
    ADD COLUMN IF NOT EXISTS gateway_tls_secret_name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gateway_tls_secret_namespace text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gateway_cert_issuer_kind text NOT NULL DEFAULT 'ClusterIssuer',
    ADD COLUMN IF NOT EXISTS gateway_cert_issuer_name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gateway_certificate_namespace text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gateway_wildcard_cert_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS gateway_wildcard_cert_domain text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gateway_wildcard_cert_secret_name text NOT NULL DEFAULT '';

UPDATE runtime_clusters
SET gateway_cert_issuer_kind = CASE
        WHEN gateway_cert_issuer_kind = 'Issuer' THEN 'Issuer'
        ELSE 'ClusterIssuer'
    END;
