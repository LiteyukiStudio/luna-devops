# Environment Variable Reference

The API and Worker both read runtime configuration from environment variables. With Docker Compose, Helm, or another container platform, inject these values into the matching container.

For a first deployment, configure the Basic values only. Once the platform is running, adjust Advanced values when a real requirement appears instead of setting every option up front.

## API Settings

| Type | Key | Default | Purpose and when to change |
| --- | --- | --- | --- |
| Basic | `APP_ENV` | `production` | Runtime mode; set `development` explicitly only for local development. |
| Basic | `SECRET_ENCRYPTION_KEY` | Empty | Secret encryption key; required and stable in production. |
| Basic | `DATABASE_URL` | `postgres://devops:devops@postgres:5432/devops?sslmode=disable` | PostgreSQL URL; change when using another database or credential. |
| Basic | `REDIS_ADDR` | `redis://localhost:6379/0` | Complete Redis URI in the form `redis://username:password@host:port/database`; use `rediss://` for TLS. Username, password, and DB are no longer configured separately. |
| Basic | `PUBLIC_BASE_URL` | `http://localhost:8088` | Public platform URL; change for public domain, HTTPS, or reverse proxy. OIDC Redirect URI is generated as `{PUBLIC_BASE_URL}/api/v1/auth/oidc/callback`. |
| Advanced | `API_ADDR` | `:8080` | API listen address; change for custom container ports. |
| Advanced | `APP_CORS_ORIGINS` | `http://localhost:8088` | Allowed frontend origins; change when frontend and API use different origins. |
| Advanced | `TRUSTED_PROXY_CIDRS` | Empty | Reverse-proxy CIDRs allowed to provide the real client address, comma-separated. No proxy is trusted by default. Configure only controlled proxies so a forged `X-Forwarded-For` cannot bypass per-IP rate limits. |
| Advanced | `LOG_LEVEL` | `info` | Log level; temporarily use `debug` for local troubleshooting. |
| Advanced | `DB_MAX_OPEN_CONNS` | `20` | Maximum PostgreSQL connections opened by this API process; size it across all API and worker replicas to avoid exhausting the database. |
| Advanced | `DB_MAX_IDLE_CONNS` | `5` | Idle PostgreSQL connections kept by this API process; lower it when database connections are tight. |
| Advanced | `DB_CONN_MAX_LIFETIME` | `30m` | Maximum lifetime of a reused database connection; shorten it for load balancers, connection proxies, or database rolling maintenance. |
| Advanced | `DB_CONN_MAX_IDLE_TIME` | `5m` | Maximum idle time for database connections; shorten it when connection slots are tight. |
| Advanced | `METRICS_ENABLED` | `false` | Enables the dedicated Prometheus metrics listener; disabled by default. When set to `true`, the API uses `:9090` by default. |
| Advanced | `METRICS_ADDR` | `:9090` | Metrics listen address; change only when overriding the API metrics port or bind address. |
| Advanced | `METRICS_PATH` | `/metrics` | Prometheus scrape path; registered only on the dedicated metrics listener. |
| Advanced | `OTEL_EXPORTER_OTLP_ENDPOINT` | Empty | OpenTelemetry Collector OTLP HTTP endpoint. Leave empty to disable traces, OTLP metrics, and OTel log export. Use the same endpoint for API, Worker, and Agent. |
| Advanced | `OTEL_RESOURCE_ATTRIBUTES` | Empty | Additional environment or cluster resource attributes as comma-separated `key=value` pairs. |
| Advanced | `OTEL_EXPORTER_OTLP_HEADERS` | Empty | Collector authentication headers. Inject from a Secret in production instead of storing them in public configuration. |

When metrics are enabled, only the API exposes a dedicated Prometheus-compatible listener with API HTTP, connection-pool, and dependency-health metrics. Worker and Agent do not expose separate metrics ports; task, queue, build, release, model, and tool metrics flow through OTLP to the unified metrics backend. Grafana dashboard JSON lives under `grafana/dashboards/` and can be imported when needed.

When `OTEL_EXPORTER_OTLP_ENDPOINT` is configured, the API, Worker, and Agent report traces, metrics, and structured logs through OpenTelemetry. See [Connect an Observability Backend](./observability.md) for the minimal setup and local verification.

Before listening on its HTTP port, the API performs one real connection check against both Redis and PostgreSQL. The process exits immediately when either dependency is unreachable, authentication fails, or a PostgreSQL migration fails; it never starts in a partially available state. After startup, go-redis and the `database/sql` pool recover from transient connection interruptions, while the container platform is responsible for restarting a process whose startup check fails.

OIDC identity provider Redirect URI is generated from `PUBLIC_BASE_URL`, and the admin identity provider form shows a copyable value. Admission policy requires OIDC to return a non-empty email and `email_verified=true` by default. For trusted internal identity providers that cannot return the standard `email_verified` claim, disable “Require verified OIDC email” in the admission policy; the platform still requires a non-empty email.

Before login, the frontend picks the first supported language from the browser language preference list. The supported languages are currently `zh-CN` and `en-US`. After login, the account language preference wins and is cached locally so the next page load uses the same language immediately.

Available access-route domain suffixes, external access schemes, external access ports, and Gateway API defaults are managed on runtime clusters. Different clusters can use different gateway domain suffixes, GatewayClasses, and shared Gateways; the same cluster can also define multiple suffixes. A deployment target's cluster decides which suffixes are selectable, and each access route chooses exactly one suffix for default-domain generation, short-host expansion, and console access links. Set a cluster's external access scheme to `https` when an outer CDN or reverse proxy already terminates HTTPS; this only changes console display and link targets, does not change internal Gateway listeners, and does not request certificates.

## Frontend Build Settings

| Type | Key | Default | Purpose and when to change |
| --- | --- | --- | --- |
| Advanced | `VITE_DOCS_BASE_URL` | `https://luna-devops.liteyuki.org` | Documentation site base URL. Help links on pages such as Billing are generated from it. Set it before building the frontend when the docs site uses another domain or path. |

## Worker Settings

| Type | Key | Default | Purpose and when to change |
| --- | --- | --- | --- |
| Basic | `APP_ENV` | `production` | Runtime mode; keep it aligned with API. |
| Basic | `SECRET_ENCRYPTION_KEY` | Empty | Decrypts saved secrets; must match API. |
| Basic | `DATABASE_URL` | `postgres://devops:devops@postgres:5432/devops?sslmode=disable` | PostgreSQL URL; point to the same database as API. |
| Basic | `REDIS_ADDR` | `redis://localhost:6379/0` | Complete Redis URI; use the same URI as API. Use `rediss://` for TLS. |
| Basic | `BUILD_EXECUTOR_IMAGE` | `moby/buildkit:v0.24.0-rootless` | BuildKit image; change when the build cluster cannot pull the default image. |
| Advanced | `LOG_LEVEL` | `info` | Log level; temporarily use `debug` for local troubleshooting. |
| Advanced | `DB_MAX_OPEN_CONNS` | `20` | Maximum PostgreSQL connections opened by this worker process; size it across all API and worker replicas to avoid exhausting the database. |
| Advanced | `DB_MAX_IDLE_CONNS` | `5` | Idle PostgreSQL connections kept by this worker process; lower it when database connections are tight. |
| Advanced | `DB_CONN_MAX_LIFETIME` | `30m` | Maximum lifetime of a reused database connection; shorten it for load balancers, connection proxies, or database rolling maintenance. |
| Advanced | `DB_CONN_MAX_IDLE_TIME` | `5m` | Maximum idle time for database connections; shorten it when connection slots are tight. |
| Advanced | `DEPLOY_ROLLOUT_TIMEOUT_SECONDS` | `600` | Release wait timeout; increase for slow-starting apps. |
| Advanced | `CERT_MANAGER_CLUSTER_ISSUER` | `letsencrypt-http01` | Certificate Issuer name; change when your cluster uses another name. |
| Advanced | `BUILD_EGRESS_MODE` | `restricted` | Build egress mode. The default allows DNS, public HTTP(S), and configured private sources only. Use `permissive` only when you explicitly accept that builds can reach arbitrary services in the cluster. |
| Advanced | `BUILD_JOB_TIMEOUT_SECONDS` | `1800` | Build timeout fallback used when a deployment target does not set one; increase for large projects. |
| Advanced | `BUILD_JOB_TTL_SECONDS` | `3600` | Completed build Pod retention; increase for a longer log window. |
| Advanced | `BUILD_CACHE_ENABLED` | `false` | Build cache switch; enable for faster repeated builds. |
| Advanced | `BUILD_CACHE_TAG` | `buildcache` | Build cache tag; change to isolate cache. |
| Advanced | `BUILD_NPM_REGISTRY` | Empty | npm registry; set when using an internal mirror. |
| Advanced | `BUILD_PRIVATE_EGRESS_CIDRS` | Empty | Extra private CIDRs in `restricted` mode. |
| Advanced | `BUILD_PRIVATE_EGRESS_PORTS` | `443` | Private allowlist ports in `restricted` mode; use ports like `5000` or `8081` for non-standard registries. |
| Advanced | `BUILD_BLOCKED_EGRESS_CIDRS` | Empty | Extra blocked CIDRs in `restricted` mode. |

Worker no longer listens on a separate metrics port. With `OTEL_EXPORTER_OTLP_ENDPOINT` configured, task, retry, queue depth, queue latency, build/release result and duration, runtime replica, and gateway sync metrics are exported with the other telemetry signals.

The worker also starts consuming tasks only after both Redis and PostgreSQL pass their startup connection checks. After startup, Asynq, go-redis, and `database/sql` recover from transient connection interruptions.
