ALTER TABLE IF EXISTS runtime_clusters
    ADD COLUMN IF NOT EXISTS gateway_public_port bigint NOT NULL DEFAULT 80,
    ADD COLUMN IF NOT EXISTS gateway_http_listener_name text NOT NULL DEFAULT 'web',
    ADD COLUMN IF NOT EXISTS gateway_http_listener_port bigint NOT NULL DEFAULT 8080,
    ADD COLUMN IF NOT EXISTS gateway_https_listener_name text NOT NULL DEFAULT 'websecure',
    ADD COLUMN IF NOT EXISTS gateway_https_listener_port bigint NOT NULL DEFAULT 8443;

UPDATE runtime_clusters
SET gateway_public_port = CASE
        WHEN gateway_public_scheme = 'https' AND gateway_public_port = 80 THEN 443
        WHEN gateway_public_port BETWEEN 1 AND 65535 THEN gateway_public_port
        WHEN gateway_public_scheme = 'https' THEN 443
        ELSE 80
    END,
    gateway_http_listener_name = COALESCE(NULLIF(gateway_http_listener_name, ''), 'web'),
    gateway_http_listener_port = CASE
        WHEN gateway_http_listener_port BETWEEN 1 AND 65535 THEN gateway_http_listener_port
        ELSE 8080
    END,
    gateway_https_listener_name = COALESCE(NULLIF(gateway_https_listener_name, ''), 'websecure'),
    gateway_https_listener_port = CASE
        WHEN gateway_https_listener_port BETWEEN 1 AND 65535 THEN gateway_https_listener_port
        ELSE 8443
    END;
