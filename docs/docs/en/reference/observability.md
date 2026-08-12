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

Content capture is disabled by default. Enable it temporarily only when required in a controlled development, test, or security-audit environment:

```bash
AI_OBSERVABILITY_CAPTURE_CONTENT=true
```

After restarting Agent, traces and logs may record model input messages, model output messages, tool-call arguments, tool execution results, and model or tool error responses. Even after redaction, this data may contain user input, resource names, configuration content, or diagnostic information. This feature is not a data-isolation boundary.

Keep content capture disabled in production. Enable it temporarily only after an administrator has assessed data access, retention, and sensitive-information risks. Set the switch back to `false` and restart Agent when the task is complete. Do not use sensitive content capture as a permanent audit log.

## Configure Luna's embedded Agent observability

To view Agent data in Luna DevOps Operations, open **Global Settings → AI Assistant → Advanced AI settings**, enter the Prometheus, Loki, and Tempo query root URLs, and enable **Agent observability**. These query URLs are separate from the `OTEL_EXPORTER_OTLP_ENDPOINT` used for export.

- Prometheus: Agent metrics query URL, for example `http://prometheus:9090`.
- Loki: structured log query URL, for example `http://loki:3100`.
- Tempo: trace query URL, for example `http://tempo:3200`.
- When multi-tenancy or bearer authentication is enabled, enter the matching tenant IDs and tokens.

All three query URLs are required. Tokens are encrypted and are not displayed again. Keep these data sources reachable from Luna API and do not expose them directly to browsers.

Each source has its own **Test connection** button. The test uses the URL, tenant ID, and newly entered token currently in the form; a blank token reuses the saved value. Results distinguish between a working connection with data and a working connection with no data in the last hour. A failed test does not block saving, which allows operators to save deployment configuration before networking is ready. When a source queried by the page is unreachable, the corresponding metrics or details are shown as unavailable.

Open **Operations → Agent observability**. The compact header summarizes input tokens, output tokens, tool calls, conversation turns, terminal-turn success, and execution P95 for the selected 1-, 6-, or 24-hour period. The list below treats one user input through the end of the Agent response as one turn and supports cross-user search with server-side pagination. Select any turn to open its details from the right; the Agent view no longer exposes a conversation-level mode.

Turn details render real spans by start time as a vertical execution-step timeline, with filters for all, model, tool, or error steps. Receiving the user message, starting Agent, model generation, tool calls, state persistence, and external requests are driven by their corresponding spans. Expand a step to inspect duration, status, parent identifiers, and allowlisted safe attributes. User and Agent messages are read from Luna's authoritative conversation store and are not written to Tempo, Loki, or Prometheus. Prompts, secrets, and sensitive tool arguments are excluded from the attribute list. Cross-user turns and content are restricted to platform administrators, and trace-detail reads are audited.

The browser only calls fixed-query Luna API endpoints. It cannot submit arbitrary PromQL, LogQL, or TraceQL, and it never receives source URLs or credentials. Luna API reads and normalizes trace details through the Tempo 2.x query API while retaining compatibility with the legacy proxied OTLP JSON shape, so details are no longer available after the Tempo retention window expires.

When Prometheus has no Agent metrics or returns non-finite query results, the affected period totals appear as zero. The turn list still uses server-side pagination from Luna's database; a Tempo outage affects only the right-side execution timeline.

Source developers can start the repository's optional loopback-only Compose observability environment:

```bash
docker compose -f docker-compose-dev-observability.yaml up -d
```

Host processes use `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`. The Agent observability query roots are `http://localhost:9090`, `http://localhost:3100`, and `http://localhost:3200`. This environment has no production-grade authentication and must be used only for local development. See the repository's `observability/README.md` for container-specific addresses and cleanup instructions.

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
