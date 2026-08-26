# Worker Configuration

## Basic configuration

| Setting | Default | Description |
| --- | --- | --- |
| `APP_ENV` | `production` | Selects the Worker runtime mode; use `production` or `development`. |
| `DATABASE_URL` | Local PostgreSQL | Connects to PostgreSQL; use a PostgreSQL connection URI. |
| `REDIS_ADDR` | `redis://localhost:6379/0` | Connects to the Redis task queue; use a `redis://` or `rediss://` URI. |
| `SECRET_ENCRYPTION_KEY`<sup>1</sup> | Empty | Decrypts credentials stored by the platform; use the same stable key as API. |
| `PUBLIC_BASE_URL` | Empty | Sets the platform root used in task-notification links; use an HTTP(S) URL. |
| `LOG_FORMAT` | `auto` | Selects terminal log rendering; use `auto`, `console`, or `json`, and use `json` in production containers. |
| `LOG_COLOR` | `auto` | Controls console log colors; use `auto`, `always`, or `never`; `NO_COLOR` always disables colors. |
| `LOG_LEVEL` | `info` | Sets log verbosity; use `debug`, `info`, `warn`, or `error`. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Empty | Sets the telemetry receiver; use the Collector OTLP/HTTP URL. |
| `BUILD_EXECUTOR_IMAGE` | `moby/buildkit:v0.24.0-rootless` | Selects the BuildKit used by build jobs; use an OCI image reference. |
| `BUILD_EGRESS_MODE`<sup>2</sup> | `restricted` | Sets the build-network egress policy; use `restricted` or `permissive`. |
| `BUILD_PRIVATE_EGRESS_CIDRS` | Empty | Allows builds to reach private targets; use comma-separated CIDRs. |
| `DEPLOY_ROLLOUT_TIMEOUT_SECONDS` | `600` | Sets the deployment wait timeout; use a positive number of seconds. |
| `CERT_MANAGER_CLUSTER_ISSUER` | `letsencrypt-http01` | Selects the certificate issuer; use a Kubernetes ClusterIssuer name. |

1. Note: API and Worker must use the same `SECRET_ENCRYPTION_KEY`.
2. Note: `restricted` blocks metadata endpoints and private targets that were not allowed.

## Advanced configuration

### Runtime and database

| Setting | Default | Description |
| --- | --- | --- |
| `ENV_FILE` | `.env` | Selects the environment file read by Worker; use a file path. |
| `DB_MAX_OPEN_CONNS` | `20` | Limits open database connections; use a positive integer. |
| `DB_MAX_IDLE_CONNS` | `5` | Limits idle database connections; use a non-negative integer. |
| `DB_CONN_MAX_LIFETIME` | `30m` | Limits each database connection's lifetime; use a Go duration such as `30m`. |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | Limits each database connection's idle time; use a Go duration such as `5m`. |

### Observability

| Setting | Default | Description |
| --- | --- | --- |
| `OTEL_RESOURCE_ATTRIBUTES` | Empty | Sets OpenTelemetry resource attributes; use comma-separated `key=value` pairs. |
| `OTEL_EXPORTER_OTLP_HEADERS` | Empty | Authenticates Collector requests; use comma-separated `key=value` headers. |

### Build

| Setting | Default | Description |
| --- | --- | --- |
| `BUILD_JOB_TIMEOUT_SECONDS` | `1800` | Sets the build-job execution timeout; use a positive number of seconds. |
| `BUILD_JOB_TTL_SECONDS` | `3600` | Sets how long completed build jobs remain; use a non-negative number of seconds. |
| `BUILD_CACHE_ENABLED` | `false` | Controls whether BuildKit Registry cache is read and written; use `true` or `false`. |
| `BUILD_CACHE_TAG` | `buildcache` | Sets the Registry cache tag; use a valid OCI image tag. |
| `BUILD_PRIVATE_EGRESS_PORTS` | `443` | Limits ports used to reach private targets; use comma-separated ports from `1` to `65535`. |
| `BUILD_BLOCKED_EGRESS_CIDRS` | Empty | Adds networks that builds cannot reach; use comma-separated CIDRs. |

### Volume import and export

| Setting | Default | Description |
| --- | --- | --- |
| `VOLUME_TRANSFER_MAX_BYTES` | `100Gi` | Limits one volume import or export; use a quantity from `1Gi` to `5Ti`. |
| `VOLUME_TRANSFER_JOB_IMAGE` | Empty | Selects the program used by volume-transfer Pods; use an OCI image matching the Worker version. |

Helm reuses the current Worker image by default. Minimal Docker Compose, source, and binary deployments leave imports and exports disabled; when needed, set `VOLUME_TRANSFER_JOB_IMAGE` to the same version for both API and Worker. Transfer bytes do not pass through object storage.

### Docker Compose

| Setting | Default | Description |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | Selects the Worker image version used by Docker Compose; use an image tag. |
