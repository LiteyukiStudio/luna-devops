# API Configuration

When running from source, API reads these variables from the root `.env`. Docker Compose reads the same single `.env`, but its consumer allowlist injects only API settings into the API container. Shared values such as `PUBLIC_BASE_URL`, logging, OpenTelemetry, and the volume-transfer limit are authored once and forwarded only to their actual consumers. API reads and validates configuration only at startup; restart it after changes, and expect startup to fail when an explicitly selected `ENV_FILE` is missing or malformed.

## Basic configuration

| Setting | Default | Description |
| --- | --- | --- |
| `APP_ENV` | `production` | Selects the API runtime mode; use `production` or `development`. |
| `API_ADDR` | `:8080` | Sets the API listen address; use `IP:port` or `:port`. |
| `PUBLIC_BASE_URL` | Required in production | Sets the trusted user-facing platform root and the Server written into kubectl kubeconfig; in production, use the absolute HTTPS URL users actually open, with HTTP allowed only for localhost or loopback addresses. |
| `DATABASE_URL` | Local PostgreSQL | Connects to PostgreSQL; use a PostgreSQL connection URI. |
| `REDIS_ADDR` | `redis://localhost:6379/0` | Connects to Redis; use a `redis://` or `rediss://` URI. |
| `REDIS_PASSWORD` | Empty | Sets the bundled Docker Compose Redis password; use a password string or leave empty when authentication is disabled. |
| `SECRET_ENCRYPTION_KEY`<sup>1</sup> | Empty | Encrypts credentials stored by the platform; use a stable non-empty key. |
| `INITIAL_ADMIN_EMAIL`<sup>2</sup> | Empty | Creates the first administrator in a fresh database; use a valid bare email address. |
| `INITIAL_ADMIN_PASSWORD`<sup>2</sup> | Empty | Sets the first administrator's initial password; use a strong 8–72 byte value supplied through a Secret. |
| `INITIAL_ADMIN_NAME`<sup>2</sup> | Empty | Sets the first administrator's display name; use a name or leave empty to use the email. |
| `INITIAL_ADMIN_LANGUAGE`<sup>2</sup> | `zh-CN` | Sets the first administrator's language; use `zh-CN` or `en-US`. |
| `APP_CORS_ORIGINS` | Empty | Allows browser cross-origin access; use comma-separated HTTP(S) origins. |
| `TRUSTED_PROXY_CIDRS`<sup>4</sup> | Empty | Establishes the forwarded client-IP trust boundary; use comma-separated egress CIDRs only for proxies allowed to supply forwarded addresses to API, or leave empty for direct traffic. |
| `LOG_FORMAT` | `auto` | Selects terminal log rendering; use `auto`, `console`, or `json`, and use `json` in production containers. |
| `LOG_COLOR` | `auto` | Controls console log colors; use `auto`, `always`, or `never`; `NO_COLOR` always disables colors. |
| `LOG_LEVEL` | `info` | Sets log verbosity; use `debug`, `info`, `warn`, or `error`. |
| `AI_ASSISTANT_AVAILABLE` | `false` | Controls whether the AI assistant is available; use `true` or `false`. |
| `AI_AGENT_BASE_URL` | Empty | Sets the Agent address used by API; use an HTTP(S) URL. |
| `AI_AGENT_TIMEOUT` | `10s` | Bounds non-streaming API-to-Agent requests; use a Go duration such as `10s` or `1m`; SSE is not bounded by this setting. |
| `AI_INTERNAL_SECRET`<sup>3</sup> | Empty | Authenticates internal API-Agent requests; use a secret of at least 32 bytes. |

1. Note: `SECRET_ENCRYPTION_KEY` is required in production; changing it makes stored encrypted credentials unreadable.
2. Note: These settings create the first administrator only when the database has never contained a user. They never overwrite an active administrator; API refuses to start if users exist but no active administrator remains.
3. Note: API and Agent must use the same `AI_INTERNAL_SECRET`.
4. Note: Enabling the Helm chart Ingress requires the matching `app.trustedProxyCidrs` value. Prefer dedicated Ingress or reverse-proxy source subnets actually seen by API and the proxy egress ranges in the trusted forwarding chain; use a whole Pod CIDR only when network isolation prevents every other Pod from reaching API directly. Do not use client ranges; API rejects `0.0.0.0/0` and `::/0`. Direct requests are limited by their socket peer, and a forwarded client IP is used only when that peer belongs to a trusted proxy CIDR. Public CLI device start, device-code polling, authorization-code, refresh, and revoke flows use independent source buckets.

The kubectl gateway never guesses a kubeconfig Server from the request Host. Restart API after changing `PUBLIC_BASE_URL` and issue replacement kubeconfig files for affected users. The reverse proxy must also preserve `/kube/`, support upgrades, disable stream buffering, and avoid logging raw query strings. See [Kubernetes (Helm) Deployment](/en/start/install/kubernetes#configure-the-kubectl-gateway-reverse-proxy).

## Advanced configuration

### Runtime and development

| Setting | Default | Description |
| --- | --- | --- |
| `ENV_FILE` | `.env` | Selects the environment file read by API; use a valid path, because an explicitly selected missing or malformed file fails startup. |
| `APP_ENABLE_HSTS` | `true` in production | Controls whether API sends the HSTS header; use `true` or `false`. |
| `APP_VERSION` | Build version | Sets the version reported by API; use a version string. |
| `LOCAL_ADMIN_FREE_QUOTA_CREDITS` | `1000` | Grants free quota when the first administrator is created in development; use a non-negative Credits value. |

### Database

| Setting | Default | Description |
| --- | --- | --- |
| `API_DB_MAX_OPEN_CONNS` | `20` | Limits open database connections per API replica; use a positive integer. |
| `API_DB_MAX_IDLE_CONNS` | `5` | Limits idle database connections per API replica; use a non-negative integer. |
| `API_DB_CONN_MAX_LIFETIME` | `30m` | Limits each API database connection's lifetime; use a Go duration such as `30m`. |
| `API_DB_CONN_MAX_IDLE_TIME` | `5m` | Limits each API database connection's idle time; use a Go duration such as `5m`. |

### Metrics and observability

| Setting | Default | Description |
| --- | --- | --- |
| `METRICS_ENABLED` | `false` | Controls whether Prometheus metrics are exposed; use `true` or `false`. |
| `METRICS_ADDR` | `:9090` | Sets the metrics listen address; use `IP:port` or `:port`. |
| `METRICS_PATH` | `/metrics` | Sets the metrics endpoint path; use an HTTP path beginning with `/`. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Empty | Sets the telemetry receiver; use the Collector OTLP/HTTP URL. |
| `OTEL_RESOURCE_ATTRIBUTES` | Empty | Sets OpenTelemetry resource attributes; use comma-separated `key=value` pairs. |
| `OTEL_EXPORTER_OTLP_HEADERS` | Empty | Authenticates Collector requests; use comma-separated `key=value` headers. |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Empty | Sets the browser Trace Relay receiver; use an HTTP(S) URL. |
| `OTEL_EXPORTER_OTLP_TRACES_HEADERS` | Empty | Authenticates browser Trace Relay requests; use comma-separated `key=value` headers. |

### Volume import and export

| Setting | Default | Description |
| --- | --- | --- |
| `VOLUME_TRANSFER_MAX_BYTES` | `100Gi` | Limits one volume import or export; use a quantity from `1Gi` to `5Ti`. |

Volume content streams directly among the client, API, and runtime-cluster Transfer Pod. It does not require object storage, a callback URL, or a complete local API spool.

### Docker Compose and Web build

| Setting | Default | Description |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | Selects the API and Web image version used by Docker Compose; use an image tag. |
| `VITE_DOCS_BASE_URL` | Official documentation | Sets the Web documentation link; use an HTTP(S) URL available at build time. |
