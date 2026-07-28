# Binary Deployment

Run the binaries directly only for debugging, offline troubleshooting, or unusual environment validation. For regular use, prefer [Kubernetes (Helm)](/en/start/install/kubernetes) or [Docker Compose](/en/start/install/docker-compose).

With binaries, PostgreSQL, Redis, process supervision, logs, and upgrades are all your responsibility. Containerized deployment already handles much of that repeated work.

## Before You Start

You need:

- PostgreSQL.
- Redis.
- API and worker binaries.
- Environment variables for API and worker.
- A process manager such as systemd, supervisor, or your own startup scripts.

## Build Binaries

Run this from the repository root:

```bash
go build -o bin/luna-devops-api ./cmd/api
go build -o bin/luna-devops-worker ./cmd/worker
```

The API serves the embedded web console. You do not need to start Vite in production.

## Prepare Configuration

At minimum, configure PostgreSQL, Redis, the public URL, and the secret encryption key:

```bash
export APP_ENV=production
export DATABASE_URL='postgres://devops:password@127.0.0.1:5432/devops?sslmode=disable'
export REDIS_ADDR='redis://default:password@127.0.0.1:6379/0'
export PUBLIC_BASE_URL='https://devops.example.com'
export SECRET_ENCRYPTION_KEY='replace-with-a-stable-random-value'
```

`SECRET_ENCRYPTION_KEY` encrypts Git, registry, and OIDC secrets. Keep it stable after deployment, otherwise old secrets may not decrypt.

## Start API

```bash
./bin/luna-devops-api
```

The default listening port follows the project configuration. Check health:

```bash
curl http://127.0.0.1:8080/healthz
```

For public access, put Caddy, Nginx, or another reverse proxy in front of API, then set `PUBLIC_BASE_URL` to the real browser-facing URL.

## Start Worker

Start worker as a separate process:

```bash
./bin/luna-devops-worker
```

The worker handles builds, deployments, status sync, certificate issuance, and cleanup tasks. API alone can open the console, but the full delivery path requires worker.

## Example systemd Shape

Use this as a starting point rather than a universal production file. Adjust paths and environment variables for your server:

```ini
[Unit]
Description=Luna DevOps API
After=network.target postgresql.service redis.service

[Service]
WorkingDirectory=/opt/luna-devops
Environment=APP_ENV=production
Environment=DATABASE_URL=postgres://devops:password@127.0.0.1:5432/devops?sslmode=disable
Environment=REDIS_ADDR=redis://default:password@127.0.0.1:6379/0
Environment=PUBLIC_BASE_URL=https://devops.example.com
Environment=SECRET_ENCRYPTION_KEY=replace-with-a-stable-random-value
ExecStart=/opt/luna-devops/bin/luna-devops-api
Restart=always

[Install]
WantedBy=multi-user.target
```

For worker, reuse the same environment variables and change `ExecStart`:

```ini
ExecStart=/opt/luna-devops/bin/luna-devops-worker
```

## Upgrade

1. Stop API and worker.
2. Replace the binaries.
3. Confirm environment variables are still present.
4. Start API and worker.
5. Open the console and check worker logs.

## When Not To Use This

- You do not want to maintain PostgreSQL and Redis manually.
- You want simpler upgrades, rollbacks, and log collection.
- You are not sure about platform configuration yet.
- You only want to try the product first.

These cases are better served by containerized deployment.
