# Docker Compose Deployment

Docker Compose is the quickest way to try Luna DevOps on a personal server, in a test environment, or with a small team. It starts every required service together, so PostgreSQL and Redis do not need to be installed separately.

If you want the platform itself to run in Kubernetes, start with [Kubernetes (Helm)](/en/start/install/kubernetes).

## Before You Start

You need:

- A machine that can run Docker.
- Docker Compose.
- Network access to pull DockerHub images.
- Host port `8088` available.

## Choose A Version

The repository root `docker-compose.yaml` pulls these images by default:

```text
liteyukistudio/devops-api:nightly
liteyukistudio/devops-worker:nightly
liteyukistudio/devops-agent:nightly (AI profile only)
```

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

This starts PostgreSQL, password-protected Redis, API, and Worker. API completes database migrations first, and Compose starts Worker only after `/healthz` passes, so Worker cannot access a fresh schema too early. The API image already embeds the web console, so you do not need to start Vite separately. On the first visit, open `/bootstrap` and use the `BOOTSTRAP_TOKEN` from `.env` to create the first administrator, then rotate or remove that one-time token.

### Enable The AI Assistant

The AI Agent uses an explicit profile and does not start by default. Configure these values in `.env` first:

- `AI_AGENT_SERVICE_TOKEN` and `AI_ACTOR_CONTEXT_SIGNING_KEY`: independent values of at least 32 characters;
- `AI_AGENT_CALLBACK_SERVICE_TOKEN`: the Agent's independent callback credential for Luna API;
- `AI_RUN_ACTOR_GRANT_SIGNING_KEY` and `AI_DELEGATION_TOKEN_SIGNING_KEY`: independent signing keys for Run Grants and short-lived delegations;
- `AI_RUN_GRANT_ENCRYPTION_KEY_BASE64`: 32 random bytes encoded as Base64 and kept stable.

Then start the profile:

```bash
AI_ASSISTANT_AVAILABLE=true docker compose --profile ai up -d
```

Configure the Provider, model, access rules, and quotas under **Global Settings → AI Assistant**. The Provider API key is stored by the platform Secret Store and does not belong in `.env`. For diagnostics, run `docker compose --profile ai logs -f agent`.

To build images from the current source tree:

```bash
docker compose -f docker-compose-build.yaml up -d --build
# Include the Agent:
AI_ASSISTANT_AVAILABLE=true docker compose -f docker-compose-build.yaml --profile ai up -d --build
```

## Open The Console

Visit:

```text
http://localhost:8088
```

The default Compose stack only exposes API on host port `8088`. PostgreSQL and Redis stay inside the container network and do not occupy host ports `5432` and `6379`.

## Check Services

```bash
docker compose ps
docker compose logs -f api
docker compose logs -f worker
```

When API is healthy, the console opens in the browser. Worker must also be healthy for builds, deployments, and status sync to run. If the page opens but tasks never start, check the Worker logs first.

## Next

1. Open [First Sign-in](/en/start/first-login) and create or sign in as an administrator.
2. Open [Connect Cluster and Registry](/en/start/connect-resources) to prepare runtime and image storage.
3. Follow [Deploy a Web Project](/en/start/first-project) to complete the first delivery path.

## Stop

```bash
docker compose down
```

To remove data as well:

```bash
docker compose down -v
```

<div class="hint">
Start first, configure gradually. The first goal is to enter the console, not to connect every external system at once.
</div>
