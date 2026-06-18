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

OIDC identity provider Redirect URI is generated from `PUBLIC_BASE_URL`, and the admin identity provider form shows a copyable value. Admission policy requires OIDC to return a non-empty email and `email_verified=true` by default. For trusted internal identity providers that cannot return the standard `email_verified` claim, disable “Require verified OIDC email” in the admission policy; the platform still requires a non-empty email.

## Worker Settings

| Type | Key | Default | Purpose and when to change |
| --- | --- | --- | --- |
| Basic | `APP_ENV` | `development` | Runtime mode; keep it aligned with API. |
| Basic | `SECRET_ENCRYPTION_KEY` | Empty | Decrypts saved secrets; must match API. |
| Basic | `DATABASE_URL` | `postgres://devops:devops@postgres:5432/devops?sslmode=disable` | PostgreSQL URL; point to the same database as API. |
| Basic | `REDIS_ADDR` | `redis:6379` | Redis address; point to the same Redis as API. |
| Basic | `BUILD_EXECUTOR_IMAGE` | `moby/buildkit:v0.24.0-rootless` | BuildKit image; change when the build cluster cannot pull the default image. |
| Advanced | `LOG_LEVEL` | `debug` | Log level; production usually uses `info`. |
| Advanced | `DEPLOY_ROLLOUT_TIMEOUT_SECONDS` | `600` | Release wait timeout; increase for slow-starting apps. |
| Advanced | `CERT_MANAGER_CLUSTER_ISSUER` | `letsencrypt-http01` | Certificate Issuer name; change when your cluster uses another name. |
| Advanced | `BUILD_JOB_TIMEOUT_SECONDS` | `5400` | Build timeout; increase for large projects. |
| Advanced | `BUILD_JOB_TTL_SECONDS` | `3600` | Completed build Pod retention; increase for a longer log window. |
| Advanced | `BUILD_CACHE_ENABLED` | `false` | Build cache switch; enable for faster repeated builds. |
| Advanced | `BUILD_CACHE_TAG` | `buildcache` | Build cache tag; change to isolate cache. |
| Advanced | `BUILD_NPM_REGISTRY` | Empty | npm registry; set when using an internal mirror. |
| Advanced | `BUILD_PRIVATE_EGRESS_CIDRS` | Empty | Private CIDRs builds may access; set for internal registries or mirrors. |
| Advanced | `BUILD_BLOCKED_EGRESS_CIDRS` | Empty | CIDRs builds must not access; set for stricter isolation. |
