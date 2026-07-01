ALTER TABLE IF EXISTS runtime_clusters
    DROP COLUMN IF EXISTS gateway_public_port,
    DROP COLUMN IF EXISTS gateway_http_listener_name,
    DROP COLUMN IF EXISTS gateway_http_listener_port,
    DROP COLUMN IF EXISTS gateway_https_listener_name,
    DROP COLUMN IF EXISTS gateway_https_listener_port;
