# Connect an Observability Backend

Luna DevOps exports traces, metrics, and structured logs through OpenTelemetry. It does not require a specific backend; prepare an OpenTelemetry Collector with OTLP HTTP support.

## Minimal configuration

Configure the same endpoint for API, Worker, and Agent:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
```

After restart, the services report as `luna-devops-api`, `luna-worker`, and `luna-agent`. Leaving the variable empty disables export. A temporary Collector outage does not block business requests.

To identify an environment or cluster, add:

```bash
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production,k8s.cluster.name=main
```

If the Collector requires authentication, inject `OTEL_EXPORTER_OTLP_HEADERS` from a Secret instead of storing credentials in public configuration.

## Agent full-content observability (sensitive)

Telemetry does not record user prompts, model replies, or tool bodies by default. Enable the following only for model diagnostics after restricting Tempo/Loki access and retention:

```bash
AI_OBSERVABILITY_CAPTURE_CONTENT=true
```

After restarting Agent, traces and logs include redacted model input, output, and tool-call content. Common tokens, cookies, passwords, and API keys are filtered, but prompts and business responses may still contain personal or platform data. This feature is not a data-isolation boundary.

Set the switch back to `false` and restart Agent after diagnosis. Do not use sensitive content capture as a permanent audit log.

## Configure Luna's embedded Agent observability

To view Agent data in Luna DevOps Operations, open **Global Settings → AI Assistant → Advanced AI settings**, enter the Prometheus, Loki, and Tempo query root URLs, and enable **Agent observability**. These query URLs are separate from the `OTEL_EXPORTER_OTLP_ENDPOINT` used for export.

- Prometheus: Agent metrics query URL, for example `http://prometheus:9090`.
- Loki: structured log query URL, for example `http://loki:3100`.
- Tempo: trace query URL, for example `http://tempo:3200`.
- When multi-tenancy or bearer authentication is enabled, enter the matching tenant IDs and tokens.

All three query URLs are required. Tokens are encrypted and are not displayed again. Keep these data sources reachable from Luna API and do not expose them directly to browsers.

Each source has its own **Test connection** button. The test uses the URL, tenant ID, and newly entered token currently in the form; a blank token reuses the saved value. Results distinguish between a working connection with data and a working connection with no data in the last hour. A failed test does not block saving, which allows operators to save deployment configuration before networking is ready. Once enabled, the Agent view reports an unreachable source as unavailable.

Open **Operations → Agent observability** to inspect Run success, active Runs, first-token P95, model and dependency errors, token throughput, tool failures, recent Agent Runs, and trace-correlated failure logs. Open **View trace** on a recent Run to inspect span hierarchy, parallel stages, latency share, error spans, and model, tool, HTTP, or database calls in the embedded waterfall. Its inspector only exposes allowlisted model, token, tool, and error attributes; prompts and sensitive tool arguments are excluded.

The browser only calls fixed-query Luna API endpoints. It cannot submit arbitrary PromQL, LogQL, or TraceQL, and it never receives source URLs or credentials. Luna API reads and normalizes trace details from Tempo, so details are no longer available after the Tempo retention window expires.

When Prometheus has no Agent metrics or returns non-finite query results, the affected metrics appear as zero or an empty trend without preventing Loki logs and Tempo traces from loading.

## Import Grafana dashboards

The repository provides two dashboards:

- `grafana/dashboards/luna-devops-overview.json`: platform, API, Worker, delivery, Agent, and database overview.
- `grafana/dashboards/luna-agent-llm-observability.json`: Agent Runs, model latency, tokens, tools, logs, and traces.

Select existing Prometheus, Tempo, and Loki data sources during import. Start with success rate, error rate, and latency, then locate a trace by conversation, turn, or Run ID. Enable sensitive content capture only when raw model behavior is required.

## Query traces

One user message normally corresponds to one trace. Common correlation fields are:

| Scope | Attribute |
| --- | --- |
| AI conversation | `gen_ai.conversation.id` |
| Conversation turn | `luna.turn.id` |
| Agent execution | `luna.run.id` |
| Tool call | `luna.tool_call.id` |

For example, query one Run in Tempo:

```text
{ span.luna.run.id = "airun_xxx" }
```

Refer to the documentation for your Tempo and Grafana versions for additional query syntax.

## Production recommendations

- Keep the Collector and observability backends on controlled networks. Use TLS and authentication across networks.
- Configure batching, memory limits, and sampling centrally in the Collector. Prioritize errors and slow traces.
- Apply least-privilege access and suitable retention to logs, traces, and sensitive Agent content.
- API can expose a Prometheus-compatible scrape endpoint; export complete platform metrics through OTLP.
- Successful health probes may not generate traces. User-initiated Provider, cluster, and registry tests remain observable.

See the [OpenTelemetry Collector documentation](https://opentelemetry.io/docs/collector/deploy/) for deployment and exporter configuration.
