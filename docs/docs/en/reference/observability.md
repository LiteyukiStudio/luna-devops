# Observability

Luna DevOps exports traces, metrics, and structured logs through OpenTelemetry. Prepare an OpenTelemetry Collector with OTLP HTTP support and, when required, Prometheus, Loki, and Tempo.

## Export telemetry

Configure the same Collector endpoint for API, Worker, and Agent:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production,k8s.cluster.name=main
```

Leaving the endpoint empty disables export. If the Collector requires authentication, inject `OTEL_EXPORTER_OTLP_HEADERS` from a Secret instead of storing credentials in public configuration.

After restart, confirm that the Collector receives:

- `luna-devops-api`
- `luna-worker`
- `luna-agent` when AI Assistant is enabled

## Read and search logs

Log records are always structured, while terminal rendering and OTel export are independent. Local processes use `LOG_FORMAT=auto` by default: interactive terminals render console output and redirected output switches to JSON. Production Docker Compose and Helm deployments set `LOG_FORMAT=json`. `LOG_COLOR=auto|always|never` affects console output only, and `NO_COLOR` always disables color; JSON and OTel records never contain ANSI sequences.

Failure records use stable `event.name`, `operation`, `outcome`, and `error.code` fields. `error.message` keeps the complete credential-redacted error chain, and contextual records also include `trace_id`, `span_id`, `request_id`, and resource IDs. Start incident searches with the response `requestId` or `traceId`, then inspect the complete dependency error. Internal addresses, file paths, SQLSTATE values, and resource IDs remain available for diagnosis; actual token, Authorization, Cookie, password, API key, and private-key values are redacted.

Production API error responses contain only a stable `code`, a generic `message` localization key, and available `requestId` / `traceId` values. They do not expose database errors, internal addresses, or stacks. With `APP_ENV=development`, responses also contain a credential-redacted `developerDetail`.

## Configure the Agent observability page

Open **Global Settings → AI Assistant → Advanced AI Settings** and enter:

- Prometheus query URL, such as `http://prometheus:9090`
- Loki query URL, such as `http://loki:3100`
- Tempo query URL, such as `http://tempo:3200`
- Optional Tenant IDs and bearer tokens

Enable **Agent Observability** after all three URLs are present and test each connection. Only Luna API receives these query URLs and tokens; they are not sent to browsers. Use **Operations → Agent Observability** to view model usage, tool calls, turns, and traces.

Open a turn to see its complete Trace ID prominently in the header. Click the Trace ID block to copy it for searching the same request chain in Tempo, a log platform, or a diagnostic ticket.

## Prometheus scraping

To scrape API metrics directly, configure:

```bash
METRICS_ENABLED=true
METRICS_ADDR=:9090
METRICS_PATH=/metrics
```

Expose this listener only to a controlled monitoring network. Worker and Agent metrics continue to use OTLP.

## Sensitive content capture

`AI_OBSERVABILITY_CAPTURE_CONTENT=true` may write redacted model inputs, outputs, tool arguments, and results to controlled traces. Correlated logs retain event metadata only and never include prompts or tool arguments. Keep it disabled in production. Enable it only during a controlled diagnostic window after restricting Tempo access and retention, then restore `false` and restart Agent.

## Verify

1. Make one successful API request and start one build or Release task.
2. Confirm that traces, logs, and metrics for the same operation can be correlated.
3. Stop the Collector temporarily and confirm that business requests continue and export resumes later.
4. Check that telemetry contains no tokens, cookies, Secrets, request bodies, or model prompts.

Production environments should enable TLS, authentication, least-privilege access, sampling, and retention policies for the Collector and backends.
