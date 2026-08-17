# Environment Variable Reference

The API and Worker both read runtime configuration from environment variables. With Docker Compose, Helm, or another container platform, inject these values into the matching container.

For a first deployment, configure the Basic values only. Once the platform is running, adjust Advanced values when a real requirement appears instead of setting every option up front.

## API Settings

| Type | Key | Default | Purpose and when to change |
| --- | --- | --- | --- |
| Basic | `APP_ENV` | `production` | Runtime mode; set `development` explicitly only for local development. |
| Advanced | `LOCAL_ADMIN_FREE_QUOTA_CREDITS` | `1000` | In `development` mode only, grants this many free credits to the local administrator wallet once using an idempotent transaction. Set it to `0` to create the wallet without a grant. Production ignores this setting. |
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

See [Connect an Observability Backend](./observability.md) for monitoring setup and verification.

OIDC identity provider Redirect URI is generated from `PUBLIC_BASE_URL`, and the admin identity provider form shows a copyable value. Admission policy requires OIDC to return a non-empty email and `email_verified=true` by default. For trusted internal identity providers that cannot return the standard `email_verified` claim, disable “Require verified OIDC email” in the admission policy; the platform still requires a non-empty email.

## Console global settings

Administrators change the following setting in the console; it is not an environment variable:

| Key | Default | Purpose |
| --- | --- | --- |
| `storage.projectManagedCapacityLimitGiB` | `0` | Managed-volume capacity limit in GiB for each project space. `0` means unlimited. Lowering it never truncates an existing volume, but blocks new creation and expansion until usage is below the limit. |


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

Worker metrics are exported through OTLP; see [Connect an Observability Backend](./observability.md).

For S3-compatible storage, callback URL, job image, and size limits used by volume import/export, see [Volume Transfer Configuration](./volume-transfer.md). These variables can remain unset when transfers are not used; volume creation, mounting, and deletion remain available.

## Agent Settings

| Type | Variable | Default | Purpose and when to change it |
| --- | --- | --- | --- |
| Advanced | `AI_OBSERVABILITY_CAPTURE_CONTENT` | `false` | Writes redacted model input/output, reasoning summaries, tool arguments, and tool results to trace events and structured logs. Enable it only for a controlled diagnostic window after restricting Tempo/Loki access and retention. |
| Advanced | `AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS` | `false` | Records every PostgreSQL query span inside the Agent. It is disabled by default because event persistence otherwise creates many low-value child spans; enable it temporarily for SQL latency or transaction diagnostics. |

These switches affect only the Agent and require a restart after changes. Content capture may include sensitive business data, so enable it only during a controlled diagnostic window. See [Connect an Observability Backend](./observability.md#agent-full-content-observability-sensitive).
