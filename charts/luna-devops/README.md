# Luna DevOps Helm Chart

This chart installs Luna DevOps with the `liteyukistudio/luna-devops` API image,
the `liteyukistudio/luna-worker` worker image, PostgreSQL, and Redis. It can also
deploy the independently released `liteyukistudio/luna-agent` image.

## Install

```bash
helm install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace
```

Open the console:

```bash
kubectl -n luna-devops port-forward svc/luna-devops-api 8088:80
```

Then visit:

```text
http://localhost:8088
```

## Set a public URL

When exposing the console with Ingress, set `app.publicBaseUrl` to the browser-facing URL.

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  --set app.publicBaseUrl=https://devops.example.com \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=devops.example.com
```

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

## Enable the AI Agent

AI is fail-closed and disabled by default. Set `ai.enabled=true` and point
`ai.existingSecret` to a Secret containing one stable `ai-internal-secret` key
(or override `ai.internalSecretKey`). Generate it with `openssl rand -hex 32`;
API and Agent derive all purpose-specific internal keys. The Agent image tag
follows the chart `appVersion` unless `ai.agent.image.tag` is set explicitly.

For a short, controlled diagnostic window, set
`ai.agent.observabilityCaptureContent=true` to export redacted model input,
model output, tool arguments, and tool results. It is disabled by default
because the content can contain user and platform data.

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

## Enable project-volume transfers

Volume import and export are disabled by default. Create a private
S3-compatible bucket and a Kubernetes Secret containing the access key before
enabling the feature:

```bash
kubectl -n luna-devops create secret generic volume-transfer-s3 \
  --from-literal=access-key-id='<access-key-id>' \
  --from-literal=secret-access-key='<secret-access-key>'
```

Then supply non-sensitive settings through values:

```yaml
volumeTransfer:
  enabled: true
  s3:
    endpoint: https://objects.example.com
    region: us-east-1
    bucket: luna-volume-transfers
    existingSecret: volume-transfer-s3
  spool:
    dir: /tmp/luna-devops-volume-transfer-spool
    maxBytes: 2Gi
    minFreeBytes: 1Gi
  callbackBaseUrl: https://devops.example.com
```

Both endpoints must use HTTPS. The callback URL must be reachable from runtime
clusters. `volumeTransfer.jobImage` defaults to the Worker image and release
tag, which contains both the Worker and `/usr/local/bin/luna-volume-transfer`.
Override it only when the replacement image uses the same application version.
The spool path is local to each API replica. Size its writable filesystem for
at least `maxBytes + minFreeBytes`; the API enforces that budget before reading
an upload part and removes stale owned files after `spool.orphanAge`.
The chart never stores S3 credentials in a ConfigMap or rendered values.
