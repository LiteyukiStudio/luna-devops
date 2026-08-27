# Kubernetes (Helm) Deployment

For a long-running Luna DevOps installation on Kubernetes, use Helm. The chart deploys API, Worker, PostgreSQL, and Redis together, and it can also connect to existing external database services.

These commands target standard Kubernetes. K3s and other distributions are supported when they meet the [compatibility requirements](/en/reference/compatibility); distribution-specific storage, Ingress, and security policies remain the cluster administrator's responsibility.

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

API, Worker, PostgreSQL, and Redis are installed by default. The AI assistant is disabled; enable it with `ai.enabled=true` and provide a stable `ai-internal-secret` through `ai.existingSecret`.

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
  --set worker.image.tag=v0.1.0-rc.1 \
  --set ai.agent.image.tag=v0.1.0-rc.1
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

External Redis can use a connection URI or an existing Secret; use `rediss://` for TLS. In production, provide credentials through Kubernetes Secrets instead of committing passwords to a values file.

Then install:

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  -f values-prod.yaml
```

## Configure The Agent Network Policy

The chart enables API-to-Agent ingress isolation by default but does not restrict Agent egress by default. The
Agent must reach its model Provider, OpenTelemetry Collector, and PostgreSQL; native Kubernetes NetworkPolicy
cannot reliably express destinations backed by dynamic DNS records.

Enable egress isolation only after listing every real destination, for example:

```yaml
ai:
  agent:
    networkPolicy:
      egress:
        enabled: true
        additionalCIDRs:
          - 203.0.113.10/32
        additionalRules:
          - to:
              - namespaceSelector:
                  matchLabels:
                    kubernetes.io/metadata.name: observability
            ports:
              - protocol: TCP
                port: 4318
```

For model Providers with dynamic addresses, use a CNI that supports FQDN policies or a stable egress proxy. A
deny-all rule that omits real destinations is not a working configuration. The non-root user, read-only Agent
root filesystem, and disabled ServiceAccount token remain enforced independently.

## Common Values

| Value | Default | Notes |
| --- | --- | --- |
| `app.publicBaseUrl` | `http://localhost:8088` | Sets the user-facing platform root; use an HTTP(S) URL. |
| `app.secretEncryptionKey` | Generated | Encrypts credentials stored by the platform; use a stable non-empty key. |
| `api.image.tag` / `worker.image.tag` | `nightly` | Selects the API and Worker image versions; use image tags. |
| `ai.enabled` / `ai.existingSecret` | `false` / empty | Enables Agent and selects its internal secret; use a boolean and a Kubernetes Secret name. |
| `ai.agent.networkPolicy.ingress.enabled` / `egress.enabled` | `true` / `false` | Controls API-to-Agent ingress isolation and Agent egress isolation; use booleans and enable egress only after its destination rules are complete. |
| `ai.agent.networkPolicy.egress.additionalCIDRs` / `additionalRules` | `[]` / `[]` | Adds Agent destinations; use a CIDR list and a list of Kubernetes NetworkPolicy egress rules. |
| `postgresql.enabled` / `externalDatabase.url` | `true` / empty | Selects bundled or external PostgreSQL; use a boolean and a PostgreSQL connection URI. |
| `redis.enabled` / `externalRedis.url` | `true` / empty | Selects bundled or external Redis; use a boolean and a `redis://` or `rediss://` URI. |
| `worker.buildEgressMode` | `restricted` | Sets the build-network egress policy; use `restricted` or `permissive`. |

## Uninstall

```bash
helm uninstall luna-devops -n luna-devops
```

PVCs are retained by default to prevent accidental data loss. Remove them manually only after confirming the data is no longer needed:

```bash
kubectl -n luna-devops delete pvc -l app.kubernetes.io/instance=luna-devops
```
