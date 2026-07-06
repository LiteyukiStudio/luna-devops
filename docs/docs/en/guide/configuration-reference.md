# Configuration Reference

For containerized deployment, inject settings through environment variables.

Read Basic first. Use Advanced only when you need it.

## API Settings

| Type | Key | Default | Purpose and when to change |
| --- | --- | --- | --- |
| Basic | `APP_ENV` | `development` | Runtime mode; set `production` when going live. |
| Basic | `SECRET_ENCRYPTION_KEY` | Empty | Secret encryption key; required and stable in production. |
| Basic | `DATABASE_URL` | `postgres://devops:devops@postgres:5432/devops?sslmode=disable` | PostgreSQL URL; change when using another database or credential. |
| Basic | `REDIS_ADDR` | `redis:6379` | Redis address; change when using external Redis. |
| Basic | `PUBLIC_BASE_URL` | `http://localhost:8088` | Public platform URL; change for public domain, HTTPS, or reverse proxy. OIDC Redirect URI is generated as `{PUBLIC_BASE_URL}/api/v1/auth/oidc/callback`. |
| Advanced | `API_ADDR` | `:8080` | API listen address; change for custom container ports. |
| Advanced | `APP_CORS_ORIGINS` | `http://localhost:8088` | Allowed frontend origins; change when frontend and API use different origins. |
| Advanced | `LOG_LEVEL` | `debug` | Log level; production usually uses `info`. |
| Advanced | `DB_MAX_OPEN_CONNS` | `20` | Maximum PostgreSQL connections opened by this API process; size it across all API and worker replicas to avoid exhausting the database. |
| Advanced | `DB_MAX_IDLE_CONNS` | `5` | Idle PostgreSQL connections kept by this API process; lower it when database connections are tight. |
| Advanced | `DB_CONN_MAX_LIFETIME` | `30m` | Maximum lifetime of a reused database connection; shorten it for load balancers, connection proxies, or database rolling maintenance. |
| Advanced | `DB_CONN_MAX_IDLE_TIME` | `5m` | Maximum idle time for database connections; shorten it when connection slots are tight. |
| Advanced | `DB_CONNECT_RETRY_ATTEMPTS` | `12` | Startup PostgreSQL connection retry attempts; increase when the database starts slowly or temporarily runs out of slots. |
| Advanced | `DB_CONNECT_RETRY_INTERVAL` | `5s` | Startup connection retry interval. Values like `5s`, `1m`, or plain seconds are accepted. |
| Advanced | `METRICS_ENABLED` | `false` | Enables the dedicated Prometheus metrics listener; disabled by default. When set to `true`, the API uses `:9090` by default. |
| Advanced | `METRICS_ADDR` | `:9090` | Metrics listen address; change only when overriding the API metrics port or bind address. |
| Advanced | `METRICS_PATH` | `/metrics` | Prometheus scrape path; registered only on the dedicated metrics listener. |

When metrics are enabled, the API exports HTTP request, latency, error response, PostgreSQL connection pool, and PostgreSQL/Redis health metrics. Helm deployments can render the Grafana dashboard ConfigMap with `metrics.grafanaDashboard.enabled=true`.

OIDC identity provider Redirect URI is generated from `PUBLIC_BASE_URL`, and the admin identity provider form shows a copyable value. Admission policy requires OIDC to return a non-empty email and `email_verified=true` by default. For trusted internal identity providers that cannot return the standard `email_verified` claim, disable “Require verified OIDC email” in the admission policy; the platform still requires a non-empty email.

Before login, the frontend picks the first supported language from the browser language preference list. The supported languages are currently `zh-CN` and `en-US`. After login, the account language preference wins and is cached locally so the next page load uses the same language immediately.

Available access-route domain suffixes, external access schemes, external access ports, and Gateway API defaults are managed on runtime clusters. Different clusters can use different gateway domain suffixes, GatewayClasses, and shared Gateways; the same cluster can also define multiple suffixes. A deployment target's cluster decides which suffixes are selectable, and each access route chooses exactly one suffix for default-domain generation, short-host expansion, and console access links. Set a cluster's external access scheme to `https` when an outer CDN or reverse proxy already terminates HTTPS; this only changes console display and link targets, does not change internal Gateway listeners, and does not request certificates.

## Worker Settings

| Type | Key | Default | Purpose and when to change |
| --- | --- | --- | --- |
| Basic | `APP_ENV` | `development` | Runtime mode; keep it aligned with API. |
| Basic | `SECRET_ENCRYPTION_KEY` | Empty | Decrypts saved secrets; must match API. |
| Basic | `DATABASE_URL` | `postgres://devops:devops@postgres:5432/devops?sslmode=disable` | PostgreSQL URL; point to the same database as API. |
| Basic | `REDIS_ADDR` | `redis:6379` | Redis address; point to the same Redis as API. |
| Basic | `BUILD_EXECUTOR_IMAGE` | `moby/buildkit:v0.24.0-rootless` | BuildKit image; change when the build cluster cannot pull the default image. |
| Advanced | `LOG_LEVEL` | `debug` | Log level; production usually uses `info`. |
| Advanced | `DB_MAX_OPEN_CONNS` | `20` | Maximum PostgreSQL connections opened by this worker process; size it across all API and worker replicas to avoid exhausting the database. |
| Advanced | `DB_MAX_IDLE_CONNS` | `5` | Idle PostgreSQL connections kept by this worker process; lower it when database connections are tight. |
| Advanced | `DB_CONN_MAX_LIFETIME` | `30m` | Maximum lifetime of a reused database connection; shorten it for load balancers, connection proxies, or database rolling maintenance. |
| Advanced | `DB_CONN_MAX_IDLE_TIME` | `5m` | Maximum idle time for database connections; shorten it when connection slots are tight. |
| Advanced | `DB_CONNECT_RETRY_ATTEMPTS` | `12` | Startup PostgreSQL connection retry attempts; increase when the database starts slowly or temporarily runs out of slots. |
| Advanced | `DB_CONNECT_RETRY_INTERVAL` | `5s` | Startup connection retry interval. Values like `5s`, `1m`, or plain seconds are accepted. |
| Advanced | `METRICS_ENABLED` | `false` | Enables the dedicated Prometheus metrics listener; disabled by default. When set to `true`, the worker uses `:9091` by default. |
| Advanced | `METRICS_ADDR` | `:9091` | Metrics listen address; change only when overriding the worker metrics port or bind address. |
| Advanced | `METRICS_PATH` | `/metrics` | Prometheus scrape path; registered only on the dedicated metrics listener. |

When metrics are enabled, the worker exports task, retry, queue depth, queue latency, build/release result and duration, runtime replica, gateway sync, and dependency health metrics. Helm deployments can render the Grafana dashboard ConfigMap with `metrics.grafanaDashboard.enabled=true`.
| Advanced | `DEPLOY_ROLLOUT_TIMEOUT_SECONDS` | `600` | Release wait timeout; increase for slow-starting apps. |
| Advanced | `CERT_MANAGER_CLUSTER_ISSUER` | `letsencrypt-http01` | Certificate Issuer name; change when your cluster uses another name. |
| Advanced | `BUILD_EGRESS_MODE` | `permissive` | Build egress mode; set to `restricted` when strong isolation is required. |
| Advanced | `BUILD_JOB_TIMEOUT_SECONDS` | `1800` | Build timeout fallback used when a deployment target does not set one; increase for large projects. |
| Advanced | `BUILD_JOB_TTL_SECONDS` | `3600` | Completed build Pod retention; increase for a longer log window. |
| Advanced | `BUILD_CACHE_ENABLED` | `false` | Build cache switch; enable for faster repeated builds. |
| Advanced | `BUILD_CACHE_TAG` | `buildcache` | Build cache tag; change to isolate cache. |
| Advanced | `BUILD_NPM_REGISTRY` | Empty | npm registry; set when using an internal mirror. |
| Advanced | `BUILD_PRIVATE_EGRESS_CIDRS` | Empty | Extra private CIDRs in `restricted` mode. |
| Advanced | `BUILD_PRIVATE_EGRESS_PORTS` | `443` | Private allowlist ports in `restricted` mode; use ports like `5000` or `8081` for non-standard registries. |
| Advanced | `BUILD_BLOCKED_EGRESS_CIDRS` | Empty | Extra blocked CIDRs in `restricted` mode. |
