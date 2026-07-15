# Kubernetes (Helm) Deployment

For a long-running Luna DevOps installation on Kubernetes or K3s, use Helm. The chart deploys API, Worker, PostgreSQL, and Redis together, and it can also connect to existing external database services.

## Before You Start

You need:

- A Kubernetes or K3s cluster.
- `kubectl` and `helm` configured locally.
- Network access from the cluster to pull DockerHub images.
- A default StorageClass for PostgreSQL and Redis data.

## Install

Run this from the repository root:

```bash
helm install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace
```

This starts:

```text
liteyukistudio/devops-api:nightly
liteyukistudio/devops-worker:nightly
postgres:17-alpine
redis:8-alpine
```

## Open The Console

Forward the API Service:

```bash
kubectl -n luna-devops port-forward svc/luna-devops-api 8088:80
```

Then visit:

```text
http://localhost:8088
```

## Use A Fixed Version

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  --set api.image.tag=v0.1.0-rc.1 \
  --set worker.image.tag=v0.1.0-rc.1
```

## Access the Console Through a Public Domain

When exposing the console with Ingress, set `app.publicBaseUrl` to the real browser-facing URL:

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  --set app.publicBaseUrl=https://devops.example.com \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=devops.example.com
```

`app.publicBaseUrl` affects OIDC callbacks, webhook callbacks, and browser origin checks. Do not set it to an internal Service address.

## Use External PostgreSQL Or Redis

The built-in services are convenient for getting started. If production already has managed PostgreSQL or Redis, disable the matching built-in component:

```yaml
postgresql:
  enabled: false
externalDatabase:
  url: postgres://devops:password@postgres.example.com:5432/devops?sslmode=disable

redis:
  enabled: false
externalRedis:
  url: redis://default:replace-with-a-strong-password@redis.example.com:6379/0
```

The chart generates the built-in Redis password on first install and stores `redis-password` separately from the `redis-url` consumed by API and Worker; upgrades reuse the existing Secret. A `redis.auth.existingSecret` for the built-in service must contain both keys. For external Redis, set `externalRedis.url` directly or use an `externalRedis.existingSecret` containing only `redis-url`. External URIs use `redis://username:password@host:port/database`; use `rediss://` for TLS.

Then install:

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  -f values-prod.yaml
```

## Common Values

| Value | Default | Notes |
| --- | --- | --- |
| `app.publicBaseUrl` | `http://localhost:8088` | Public console URL. Required when Ingress is enabled. |
| `app.secretEncryptionKey` | Generated on first install | Encrypts Git, registry, and OIDC secrets. Keep it stable in production. |
| `api.image.tag` / `worker.image.tag` | `nightly` | API and worker image tag. |
| `postgresql.enabled` | `true` | Install built-in PostgreSQL. |
| `redis.enabled` | `true` | Install built-in Redis. |
| `externalRedis.url` | Empty | Complete external Redis URI, used when built-in Redis is disabled. |
| `worker.buildEgressMode` | `permissive` | Build Job egress mode. Use `restricted` when you need stronger isolation. |

## Uninstall

```bash
helm uninstall luna-devops -n luna-devops
```

PVCs are retained by default to prevent accidental data loss. Remove them manually only after confirming the data is no longer needed:

```bash
kubectl -n luna-devops delete pvc -l app.kubernetes.io/instance=luna-devops
```
