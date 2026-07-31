# Connect an Observability Backend

Luna DevOps can send traces, metrics, and structured logs through OpenTelemetry. The platform does not bundle or require a specific observability backend. Prepare an OpenTelemetry Collector with OTLP HTTP enabled, then configure the same endpoint for the API, Worker, and Agent.

## Minimal configuration

Inject this environment variable into all three services:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
```

After restart, the services report as `luna-devops-api`, `luna-worker`, and `luna-agent`. An empty value keeps exporters disabled, and an unavailable Collector does not block business requests.

Add resource attributes when you need to identify an environment or cluster:

```bash
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production,k8s.cluster.name=main
```

Only add headers when the Collector requires authentication. Do not place the secret in Compose or Helm values; inject it from a Secret in production:

```bash
OTEL_EXPORTER_OTLP_HEADERS=api-key=replace-me
```

For Helm, set the endpoint and resource attributes directly:

```yaml
observability:
  otlpEndpoint: http://otel-collector.observability.svc.cluster.local:4318
  resourceAttributes: deployment.environment.name=production,k8s.cluster.name=main
```

## Local verification

For temporary local verification, use Grafana's OpenTelemetry LGTM image. It is not part of Luna DevOps and does not need to be added to the platform Compose file:

```bash
docker run --rm --name otel-lgtm \
  -p 3000:3000 \
  -p 4317:4317 \
  -p 4318:4318 \
  grafana/otel-lgtm:latest
```

When running from source, set:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=development
```

For Luna DevOps containers that need to reach the host, use `http://host.docker.internal:4318`. Open `http://localhost:3000` and filter by `service.name` to inspect traces, metrics, and logs.

Verify at least one database-backed list request, one asynchronous Worker task, and one Agent tool call. A healthy trace should include the request entry, database or external dependency child calls, and business stages. A failed trace should correlate with structured error logs through the same Trace ID.

## Production recommendations

- Use batch and memory limiter processors in the Collector so a temporary backend outage cannot exhaust application resources.
- Keep error and slow traces, then sample healthy traces according to capacity. Apply the sampling policy centrally in the Collector instead of configuring conflicting policies per service.
- Keep the Collector and backend on a controlled network. Use TLS and authenticated headers across network boundaries.
- Telemetry does not intentionally record cookies, tokens, secrets, passwords, request bodies, model prompts, or sensitive tool arguments. The backend should still enforce access control and retention limits.
- Existing Prometheus `/metrics` endpoints remain available for current scrape systems. OTLP Metrics are intended for a unified Collector pipeline; avoid maintaining different alert semantics in both paths.

Configure Collector receivers, processors, and exporters for the selected backend by following the [OpenTelemetry Collector deployment guide](https://opentelemetry.io/docs/collector/deploy/).
