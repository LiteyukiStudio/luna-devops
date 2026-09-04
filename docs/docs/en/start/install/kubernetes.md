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

For a fresh database, first store the initial administrator configuration in a Kubernetes Secret. Keep the following file readable only by its owner and remove it securely after creating the Secret. The name and language keys may be omitted; API falls back to the email and `zh-CN`:

```dotenv title="initial-admin.env"
initial-admin-email=admin@example.com
initial-admin-password=replace-with-a-strong-8-to-72-byte-password
```

Then run this from the repository root:

```bash
kubectl create namespace luna-devops
kubectl -n luna-devops create secret generic luna-devops-initial-admin \
  --from-env-file=initial-admin.env
helm install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --set api.initialAdmin.existingSecret=luna-devops-initial-admin
```

API, Worker, PostgreSQL, and Redis are installed by default. The administrator Secret is injected only into API; API creates the account only in a fresh database and never overwrites it during upgrades or restarts. A database with an active administrator can omit `api.initialAdmin`, and the chart still installs or upgrades normally. The AI assistant is disabled; enable it with `ai.enabled=true` and provide a stable `ai-internal-secret` through `ai.existingSecret`.

After confirming that sign-in works, detach API from the initialization Secret and remove it:

```bash
helm upgrade luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --reuse-values \
  --set-string api.initialAdmin.existingSecret=
kubectl -n luna-devops delete secret luna-devops-initial-admin
```

All four Secret key references are optional. When `existingSecret` is empty, the chart neither creates nor references an initial administrator Secret. The initial password can only be provided through an external Secret, so it never enters Helm values or release history.

## Open The Console

Forward the API Service:

```bash
kubectl -n luna-devops port-forward svc/luna-devops-api 8088:80
```

Then visit:

```text
http://localhost:8088/login
```

## Use A Fixed Version

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  --set api.initialAdmin.existingSecret=luna-devops-initial-admin \
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
  --set api.initialAdmin.existingSecret=luna-devops-initial-admin \
  --set app.publicBaseUrl=https://devops.example.com \
  --set-string app.trustedProxyCidrs=10.42.0.0/16 \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=devops.example.com
```

`app.publicBaseUrl` affects OIDC callbacks, webhook callbacks, and browser origin checks. Do not set it to an internal Service address. The example `10.42.0.0/16` only represents a dedicated Ingress or reverse-proxy source subnet; replace it with the sources actually seen by API and the proxy egress ranges in the trusted forwarding chain. Use a whole Pod CIDR only when network isolation prevents every other Pod from reaching API directly. The chart refuses to render an enabled Ingress without this boundary and always rejects `0.0.0.0/0` and `::/0`.

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

## Configure Browser Trace Relay Authentication

API relays browser traces. If that relay uses different authentication from the Collector shared by API, Worker, and Agent, first store the complete `OTEL_EXPORTER_OTLP_TRACES_HEADERS` value in a separate Kubernetes Secret. Keep the local file below readable only by its owner and remove it securely after creating the Secret. URL-encode spaces and other special characters, such as `%20`:

```text title="browser-trace-headers.txt"
Authorization=Bearer%20replace-with-relay-token
```

```bash
kubectl -n luna-devops create secret generic luna-devops-browser-trace-auth \
  --from-file=otlp-traces-headers=browser-trace-headers.txt
```

Store only the Secret and key names in production values, never the Header credential. A dedicated relay endpoint is not secret and can use API-scoped `extraEnv`; when it is omitted, browser traces still use the shared OTLP endpoint with the API-only authentication configured here:

```yaml
api:
  browserTrace:
    existingSecret: luna-devops-browser-trace-auth
    headersKey: otlp-traces-headers
  extraEnv:
    OTEL_EXPORTER_OTLP_TRACES_ENDPOINT: https://trace-relay.example.com/v1/traces
```

When browser traces use the same authentication as the shared Collector, leave `api.browserTrace.existingSecret` empty and API keeps falling back to `observability.existingSecret`. The dedicated Secret is injected only into API, never Worker or Agent. The chart also rejects plaintext credentials supplied through `api.extraEnv.OTEL_EXPORTER_OTLP_TRACES_HEADERS`.

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
| `app.publicBaseUrl` | `http://localhost:8088` | Sets the shared platform root for API callbacks and Worker notification detail links; in production, use the absolute HTTP(S) URL users actually open. |
| `ingress.annotations` | `{}` | Passes controller-specific settings to the selected Ingress controller; use an annotation map supported by that controller. |
| `app.secretEncryptionKey` | Generated | Encrypts credentials stored by the platform; use a stable non-empty key. |
| `api.initialAdmin.existingSecret` | Empty | Selects the initial administrator Secret; a fresh database requires `initial-admin-email/password`, while `initial-admin-name/language` are optional. |
| `api.image.tag` / `worker.image.tag` | `nightly` | Selects the API and Worker image versions; use image tags. |
| `api.database.maxOpenConns` / `maxIdleConns` | `20` / `5` | Caps open and idle PostgreSQL connections per API replica; use a positive integer and a non-negative integer no greater than the first value. |
| `api.browserTrace.existingSecret` / `headersKey` | Empty / `otlp-traces-headers` | Selects the API browser Trace Relay authentication Secret and key; use a Kubernetes Secret name and the key containing the complete Header list. |
| `worker.database.maxOpenConns` / `maxIdleConns` | `20` / `5` | Caps open and idle PostgreSQL connections per Worker replica; use a positive integer and a non-negative integer no greater than the first value. |
| `ai.enabled` / `ai.existingSecret` | `false` / empty | Enables Agent and selects its internal secret; use a boolean and a Kubernetes Secret name. |
| `ai.agent.observabilityCaptureDatabaseSpans` | `false` | Controls temporary per-query Agent PostgreSQL spans; use a boolean and keep it disabled during normal operation. |
| `ai.agent.networkPolicy.ingress.enabled` / `egress.enabled` | `true` / `false` | Controls API-to-Agent ingress isolation and Agent egress isolation; use booleans and enable egress only after its destination rules are complete. |
| `ai.agent.networkPolicy.egress.additionalCIDRs` / `additionalRules` | `[]` / `[]` | Adds Agent destinations; use a CIDR list and a list of Kubernetes NetworkPolicy egress rules. |
| `observability.otlpEndpoint` / `existingSecret` | Empty / empty | Sets the OTLP/HTTP endpoint and authentication Secret shared by API, Worker, and Agent; use a Collector URL and a Kubernetes Secret name containing `headersKey`. |
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
