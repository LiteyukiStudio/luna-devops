# Luna DevOps Helm Chart

This chart installs Luna DevOps with the `liteyukistudio/luna-devops` API image,
the `liteyukistudio/luna-worker` worker image, PostgreSQL, and Redis. It can also
deploy the independently released `liteyukistudio/luna-agent` image.

## Install

For a fresh database, create the API-only initial administrator Secret before
the first install. Use your secret manager in production; the manifest below
only shows the supported keys and must not be committed:

```bash
kubectl create namespace luna-devops
kubectl -n luna-devops apply -f - <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: luna-devops-initial-admin
type: Opaque
stringData:
  initial-admin-email: admin@example.com
  initial-admin-name: Platform Admin
  initial-admin-password: replace-with-a-strong-password
  initial-admin-language: zh-CN
EOF

helm install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --set api.initialAdmin.existingSecret=luna-devops-initial-admin
```

For a fresh database, the external Secret must contain `initial-admin-email`
and `initial-admin-password`. The `initial-admin-name` and
`initial-admin-language` keys are optional; API falls back to the email and
`zh-CN`. Override the corresponding `api.initialAdmin.*Key` values when your
Secret uses different key names. The password must contain 8 to 72 bytes, and
the language must be `zh-CN` or `en-US`. These values are injected into API
only; Worker and Agent never receive them.

For a controlled non-production install, the chart creates its own Secret when
any `api.initialAdmin.email`, `name`, `password`, or `language` value is set.
For a fresh database, provide at least `email` and `password`. Avoid this path
in production because the password becomes part of Helm values and release
history. No default initial administrator Secret or credentials are generated.

```yaml
api:
  initialAdmin:
    email: admin@example.com
    name: Platform Admin
    password: replace-with-a-strong-password
    language: zh-CN
```

Once an active administrator exists, API no longer requires these settings and
never uses them to reconcile or reset the account. You may clear
`api.initialAdmin.existingSecret`, `email`, `name`, `password`, and `language`,
then remove the external Secret after a successful login. An upgrade with none
of those initial administrator values renders no chart-managed Secret; all
four API Secret references are optional so the deployment remains valid.

Open the console:

```bash
kubectl -n luna-devops port-forward svc/luna-devops-api 8088:80
```

Then visit:

```text
http://localhost:8088/login
```

## Set a public URL

When exposing the console with Ingress, set `app.publicBaseUrl` to the browser-facing URL.

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

Treat `10.42.0.0/16` as a placeholder for a dedicated Ingress or reverse-proxy source subnet, and include any proxy egress ranges in the trusted forwarding chain. Do not trust an entire Pod CIDR unless network isolation prevents every other Pod from reaching API directly. The chart requires this explicit boundary whenever its Ingress is enabled and rejects `0.0.0.0/0` and `::/0`.

## Use an external database or Redis

```yaml
postgresql:
  enabled: false
externalDatabase:
  url: postgres://devops:password@postgres.example.com:5432/devops?sslmode=disable

redis:
  enabled: false
externalRedis:
  url: redis://default:password@redis.example.com:6379/0
```

For production, keep `app.secretEncryptionKey` stable. If you do not set it, the chart creates one on first install and reuses the existing Secret during upgrades. The chart stores the built-in Redis password and application connection URI as separate Secret keys, so Redis does not parse its own URI. An external Redis still uses one complete URI; use `rediss://` when it requires TLS.

## Configuration ownership and rollouts

The chart renders three separate ConfigMaps instead of sharing every setting
with every process:

- The shared ConfigMap contains `PUBLIC_BASE_URL` and the project-volume
  transfer contract consumed by both API and Worker.
- The API ConfigMap contains the listener, API database-pool, CORS,
  trusted-proxy, and Agent-client settings. `ai.agentAddress` is exposed to API
  as `AI_AGENT_BASE_URL`.
- The Worker ConfigMap contains the Worker database-pool, build, deployment,
  and certificate settings.

Database, Redis, encryption, logging, and OpenTelemetry settings continue to
use an explicit workload allowlist. Initial administrator values remain API
only, browser Trace Relay authentication remains API only, and the AI internal
Secret remains API-and-Agent only.

Use `api.extraEnv` or `worker.extraEnv` only for non-sensitive, workload-local
variables that the chart does not already manage. The chart rejects attempts
to override managed configuration or Secret-backed variables. The former
shared `app.extraEnv` value is no longer accepted because it crossed process
and Secret boundaries.

Managed ConfigMap and Secret content is hashed into the consuming Pod template,
so a Helm upgrade rolls out affected workloads. API-only and Worker-only
ConfigMap changes do not restart the other workload; shared changes restart
both. Kubernetes cannot observe content changes inside an externally managed
Secret through Helm templating, so rotate those Secrets with a versioned name
or explicitly restart the consuming workload.

## Enable the AI Agent

AI is fail-closed and disabled by default. Set `ai.enabled=true` and point
`ai.existingSecret` to a Secret containing one stable `ai-internal-secret` key
(or override `ai.internalSecretKey`). Generate it with `openssl rand -hex 32`;
API and Agent derive all purpose-specific internal keys. The Agent image tag
follows the chart `appVersion` unless `ai.agent.image.tag` is set explicitly.

For a short, controlled diagnostic window, set
`ai.agent.observabilityCaptureContent=true` to export redacted model input,
model output, tool arguments, and tool results to controlled traces. Logs keep
event metadata only. It is disabled by default because the trace content can
contain user and platform data. Set
`ai.agent.observabilityCaptureDatabaseSpans=true` only when individual
PostgreSQL query timings are needed; it remains disabled by default.

The chart enables an API-to-Agent ingress NetworkPolicy by default. Agent
egress isolation is opt-in because the Agent may connect to externally managed
model Providers, OpenTelemetry Collectors, and PostgreSQL endpoints. To
enable it, allow every destination first:

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

Native Kubernetes NetworkPolicy does not match dynamic FQDNs. A model Provider
whose addresses change cannot be expressed reliably as a static IP block; use
a CNI with an FQDN policy or a stable egress proxy for that case. The separate
non-root image user, read-only Agent root filesystem, disabled ServiceAccount
token mount, and Secret references remain enforced independently of egress.

## Send telemetry to an OpenTelemetry Collector

The Collector is deployed separately from this chart. Configure one OTLP HTTP
endpoint to export traces, metrics, and structured logs from the API, Worker,
and Agent:

```yaml
observability:
  otlpEndpoint: http://otel-collector.observability.svc.cluster.local:4318
  resourceAttributes: deployment.environment.name=production,k8s.cluster.name=main
```

Leave `observability.otlpEndpoint` empty to keep exporters disabled. When the
Collector requires headers, store the complete `key=value` header value in a
Secret, then set `observability.existingSecret` and
`observability.headersKey`. See the public observability reference for local
verification and production Collector guidance.

Browser traces are relayed by API. When that relay requires credentials
different from the shared Collector credentials, create a separate Secret
whose selected key contains the complete comma-separated
`OTEL_EXPORTER_OTLP_TRACES_HEADERS` value. For example, prepare a protected
local file containing `Authorization=Bearer%20replace-with-relay-token`, then
create the Secret without placing the credential in Helm values:

```bash
kubectl -n luna-devops create secret generic luna-devops-browser-trace-auth \
  --from-file=otlp-traces-headers=browser-trace-headers.txt
```

Reference it from the API-only browser Trace settings. A dedicated relay
endpoint is non-sensitive and may be set through `api.extraEnv`; omit it to use
the shared OTLP endpoint with the API-only credentials:

```yaml
api:
  browserTrace:
    existingSecret: luna-devops-browser-trace-auth
    headersKey: otlp-traces-headers
  extraEnv:
    OTEL_EXPORTER_OTLP_TRACES_ENDPOINT: https://trace-relay.example.com/v1/traces
```

Leave `api.browserTrace.existingSecret` empty when browser traces use the same
authentication as the shared Collector. API then keeps its existing fallback
to `observability.existingSecret`; Worker and Agent never receive the dedicated
browser Trace Secret. The chart rejects
`api.extraEnv.OTEL_EXPORTER_OTLP_TRACES_HEADERS` so credentials cannot be stored
as plaintext Helm values.

The chart defaults API, Worker, and Agent to `app.logFormat=json`,
`app.logColor=never`, and `app.logLevel=info`. Terminal rendering remains
independent from OTel export; override these values only for a controlled
interactive diagnostic session.

## Configure project-volume transfers

Project-volume import and export stream directly between the client and a
temporary workload in the runtime cluster. They do not require an object store
or a transfer credential Secret. The chart uses the Worker image and release
tag by default because that image also contains
`/usr/local/bin/luna-volume-transfer`.

Override the transfer size limit or helper image only when needed:

```yaml
volumeTransfer:
  maxBytes: 100Gi
  jobImage: ""
```

Only override `jobImage` with an image from the same application version.
Direct transfers require a stable client connection and an ingress that does
not buffer request or response bodies. An interrupted import or export must be
started again.
