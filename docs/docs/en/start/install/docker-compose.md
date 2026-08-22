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

Edit `.env`, set `SECRET_ENCRYPTION_KEY` to a stable random key, and replace the placeholders in `BOOTSTRAP_TOKEN` and `REDIS_PASSWORD`. Use URL-safe letters and digits for the Redis password. Compose passes it directly to the built-in Redis server and assembles the complete URI for API and Worker. The complete stack defaults to production mode and does not expose a fixed development administrator.

Run this from the repository root:

```bash
docker compose up -d
```

This starts the platform with PostgreSQL and Redis. On the first visit, open `/bootstrap`, create the first administrator with the `BOOTSTRAP_TOKEN` from `.env`, and then rotate or remove that one-time token.

### Enable The AI Assistant

The AI Agent uses an explicit profile and does not start by default. Generate one stable internal root and append it to `.env`:

```bash
printf 'AI_INTERNAL_SECRET=%s\n' "$(openssl rand -hex 32)" >> .env
```

The platform isolates and protects the AI assistant's internal credentials. Keep this secret stable and do not reuse another encryption key.

Then start the profile:

```bash
AI_ASSISTANT_AVAILABLE=true docker compose --profile ai up -d
```

Configure the Provider, model catalog, access rules, and quotas under **Global Settings → AI Assistant**. The Provider API key is stored by the platform Secret Store and does not belong in `.env`. For diagnostics, run `docker compose --profile ai logs -f agent`.

## Expose The Console As Needed

After deployment, configure the port, reverse proxy, domain, and TLS for the required access scope. The default local verification URL is:

```text
http://localhost:8088
```

When using another URL, update `PUBLIC_BASE_URL` and `APP_CORS_ORIGINS` together. PostgreSQL and Redis remain inside the container network and do not need external exposure.

## Check Services

```bash
docker compose ps
docker compose logs -f api
docker compose logs -f worker
```

When API is healthy, the console opens in the browser. Worker must also be healthy for builds, deployments, and status sync to run. If the page opens but tasks never start, check the Worker logs first.

## Next

1. Open [Initialize](/en/start/first-login) and create the first administrator.
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
