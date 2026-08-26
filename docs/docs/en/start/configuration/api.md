# API Configuration

## Basic configuration

| Setting | Default | Description |
| --- | --- | --- |
| `APP_ENV` | `production` | Selects the API runtime mode; use `production` or `development`. |
| `API_ADDR` | `:8080` | Sets the API listen address; use `IP:port` or `:port`. |
| `PUBLIC_BASE_URL` | Empty | Sets the user-facing platform root; use an HTTP(S) URL. |
| `DATABASE_URL` | Local PostgreSQL | Connects to PostgreSQL; use a PostgreSQL connection URI. |
| `REDIS_ADDR` | `redis://localhost:6379/0` | Connects to Redis; use a `redis://` or `rediss://` URI. |
| `REDIS_PASSWORD` | Empty | Sets the bundled Docker Compose Redis password; use a password string or leave empty when authentication is disabled. |
| `SECRET_ENCRYPTION_KEY`<sup>1</sup> | Empty | Encrypts credentials stored by the platform; use a stable non-empty key. |
| `BOOTSTRAP_TOKEN`<sup>2</sup> | Empty | Initializes the first administrator; use a one-time token string. |
| `APP_CORS_ORIGINS` | Empty | Allows browser cross-origin access; use comma-separated HTTP(S) origins. |
| `TRUSTED_PROXY_CIDRS` | Empty | Identifies trusted reverse proxies; use comma-separated CIDRs. |
| `LOG_FORMAT` | `auto` | Selects terminal log rendering; use `auto`, `console`, or `json`, and use `json` in production containers. |
| `LOG_COLOR` | `auto` | Controls console log colors; use `auto`, `always`, or `never`; `NO_COLOR` always disables colors. |
| `LOG_LEVEL` | `info` | Sets log verbosity; use `debug`, `info`, `warn`, or `error`. |
| `AI_ASSISTANT_AVAILABLE` | `false` | Controls whether the AI assistant is available; use `true` or `false`. |
| `AI_AGENT_BASE_URL` | Empty | Sets the Agent address used by API; use an HTTP(S) URL. |
| `AI_AGENT_TIMEOUT` | `10s` | Bounds non-streaming API-to-Agent requests; use a Go duration such as `10s` or `1m`; SSE is not bounded by this setting. |
| `AI_INTERNAL_SECRET`<sup>3</sup> | Empty | Authenticates internal API-Agent requests; use a secret of at least 32 bytes. |

1. Note: `SECRET_ENCRYPTION_KEY` is required in production; changing it makes stored encrypted credentials unreadable.
2. Note: Remove or rotate `BOOTSTRAP_TOKEN` after initialization.
3. Note: API and Agent must use the same `AI_INTERNAL_SECRET`.

## Advanced configuration

### Runtime and development

| Setting | Default | Description |
| --- | --- | --- |
| `ENV_FILE` | `.env` | Selects the environment file read by API; use a file path. |
| `APP_ENABLE_HSTS` | `true` in production | Controls whether API sends the HSTS header; use `true` or `false`. |
| `APP_VERSION` | Build version | Sets the version reported by API; use a version string. |
| `LOCAL_ADMIN_EMAIL` | `admin@luna.dev` | Sets the development administrator email; use a valid email address. |
| `LOCAL_ADMIN_PASSWORD` | `devops` | Sets the development administrator password; use a non-empty string. |
| `LOCAL_ADMIN_NAME` | `Platform Admin` | Sets the development administrator name; use a display name. |
| `LOCAL_ADMIN_FREE_QUOTA_CREDITS` | `1000` | Sets the development administrator's free quota; use a non-negative Credits value. |

### Database

| Setting | Default | Description |
| --- | --- | --- |
| `DB_MAX_OPEN_CONNS` | `20` | Limits open database connections; use a positive integer. |
| `DB_MAX_IDLE_CONNS` | `5` | Limits idle database connections; use a non-negative integer. |
| `DB_CONN_MAX_LIFETIME` | `30m` | Limits each database connection's lifetime; use a Go duration such as `30m`. |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | Limits each database connection's idle time; use a Go duration such as `5m`. |

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

### AI Assistant

| Setting | Default | Description |
| --- | --- | --- |
| `AI_AGENT_ADDR` | Empty | Sets a fallback Agent address for API; use an HTTP(S) URL. |

### Docker Compose and Web build

| Setting | Default | Description |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | Selects the API and Web image version used by Docker Compose; use an image tag. |
| `VITE_DOCS_BASE_URL` | Official documentation | Sets the Web documentation link; use an HTTP(S) URL available at build time. |
