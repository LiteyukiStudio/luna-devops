# Docker Compose Deployment

Docker Compose is the quickest way to try Luna DevOps on a personal server, in a test environment, or with a small team. It starts every required service together, so PostgreSQL and Redis do not need to be installed separately.

If you want the platform itself to run in Kubernetes, start with [Kubernetes (Helm)](/en/start/install/kubernetes).

## Before You Start

You need:

- A machine that can run Docker.
- Docker Compose.
- Network access to pull DockerHub images.

## Choose A Version

The repository root `docker-compose.yaml` uses `nightly` images by default.

To verify a specific release, set the image tag before starting:

```bash
DEVOPS_IMAGE_TAG=v0.1.0-rc.1 docker compose up -d
```

## Start

Prepare production settings first:

```bash
cp .env.example .env
```

The root `.env` is the single configuration entry for Compose. Set `SECRET_ENCRYPTION_KEY` to a stable random key and replace the placeholder in `REDIS_PASSWORD`. For a fresh database, also set `INITIAL_ADMIN_EMAIL` and `INITIAL_ADMIN_PASSWORD`; leave them empty when an active administrator already exists. Set the optional administrator name and language with `INITIAL_ADMIN_NAME` and `INITIAL_ADMIN_LANGUAGE`. Set `PUBLIC_BASE_URL` to the HTTP(S) root that users actually open; use `http://localhost:8088` for local-only access. Use URL-safe letters and digits for the Redis password. Compose passes it directly to the built-in Redis server and assembles the complete URI for API and Worker. The complete stack always runs in production mode and includes no fixed administrator credentials.

Compose distributes configuration through a consumer allowlist: logging and common OpenTelemetry settings reach API, Worker, and Agent; `PUBLIC_BASE_URL`, the volume-transfer limit, and the transfer image reach only API and Worker; initial-administrator, CORS, metrics, and AI Client settings reach only API; build and deployment policy reaches only Worker; and Agent database-pool and diagnostic settings reach only Agent. API and Worker connection pools use separate `API_DB_*` and `WORKER_DB_*` budgets. The source-development `AI_AGENT_BASE_URL` in the root `.env` never enters Compose; API always reaches the containerized Agent at `http://agent:8091`.

Run this from the repository root:

```bash
docker compose up -d
```

This starts the platform with PostgreSQL and Redis. API creates the first administrator from `INITIAL_ADMIN_*` when the database is empty; after the health check passes, sign in at `/login`. You may clear these variables after creation, and later changes never overwrite an existing administrator account or password. Compose always passes the optional values only to API, which validates them according to the database state.

### Enable The AI Assistant

The AI Agent uses an explicit profile and does not start by default. Generate one stable internal root and append it to `.env`:

```bash
printf 'AI_INTERNAL_SECRET=%s\n' "$(openssl rand -hex 32)" >> .env
```

The platform isolates and protects the AI assistant's internal credentials. Keep this secret stable and do not reuse another encryption key.

Set `AI_ASSISTANT_AVAILABLE=true` in the same `.env`, then start the profile. Compose fixes the container-internal Agent address, so the source-development `AI_AGENT_BASE_URL` needs no change:

```bash
docker compose --profile ai up -d
```

Configure the Provider, model catalog, access rules, and quotas under **Global Settings → AI Assistant**. The Provider API key is stored by the platform Secret Store and does not belong in `.env`. For diagnostics, run `docker compose --profile ai logs -f agent`.

## Expose The Console As Needed

After deployment, configure the port, reverse proxy, domain, and TLS for the required access scope. The default local verification URL is:

```text
http://localhost:8088
```

Compose passes the single `PUBLIC_BASE_URL` from `.env` to API and Worker for OAuth, webhooks, and notification detail links; Agent does not consume it. Recreate API and Worker after changing it. When the console is deployed cross-origin, also configure `APP_CORS_ORIGINS` as described in [API Configuration](/en/start/configuration/api). PostgreSQL and Redis remain inside the container network and do not need external exposure.

## Check Services

```bash
docker compose ps
docker compose logs -f api
docker compose logs -f worker
```

When API is healthy, the console opens in the browser. Worker must also be healthy for builds, deployments, and status sync to run. If the page opens but tasks never start, check the Worker logs first.

## Next

1. Open [First Sign-In](/en/start/first-login) and sign in with the configured administrator account.
2. Open [Add Base Resources](/en/start/connect-resources) and prepare a runtime cluster, registry, and Git Provider OAuth.
3. Follow [Daily Delivery](/en/use/workflow) to create and deploy an application.

## Stop

```bash
docker compose down
```

If the current data is no longer needed, remove its data volumes as well:

```bash
docker compose down -v
```

This permanently deletes the bundled PostgreSQL and Redis data. Make sure you have a backup first.

<div class="hint">
Start first, configure gradually. The first goal is to enter the console, not to connect every external system at once.
</div>
