# Connect an Observability Backend

Luna DevOps can send traces, metrics, and structured logs through OpenTelemetry. The platform does not bundle or require a specific observability backend. Prepare an OpenTelemetry Collector with OTLP HTTP enabled, then configure the same endpoint for the API, Worker, and Agent.

## Minimal configuration

Inject this environment variable into all three services:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
```

After restart, the services report as `luna-devops-api`, `luna-worker`, and `luna-agent`. An empty value keeps exporters disabled, and an unavailable Collector does not block business requests.

When an AI request enters the queue, Luna DevOps preserves its W3C Trace Context. The API request, Agent Run, model request, tool call, platform callback, and database access can therefore be inspected as one Trace in Tempo. Execution resumed after approval or user input keeps the same upstream Trace and uses the Run ID to distinguish execution stages.

## Agent full-content observability (sensitive)

By default, telemetry records model, run, tool, and dependency metadata without user prompts or tool bodies. To diagnose model behavior, enable the following variable only on the Agent for a short window:

```bash
AI_OBSERVABILITY_CAPTURE_CONTENT=true
```

After restarting the Agent, telemetry additionally captures:

- system prompts, conversation history, current user input, page context, tool definitions, and tool choice;
- model replies, reasoning summaries, tool-call requests, token usage, and provider error responses;
- platform and internal tool arguments, response status, response body, and request ID.

Content is emitted both as span events in traces and as trace-correlated `debug` structured logs. Each serialized field is capped at 32 KiB and sets `luna.ai.content.truncated=true` when truncated. Tokens, cookies, passwords, API keys, URL credentials, and secret form values are always replaced with `[REDACTED]`. This is not data isolation: prompts and business responses can still contain personal or platform data.

| Event | Content attribute | Stage |
| --- | --- | --- |
| `gen_ai.content.input` | `gen_ai.input.messages` | Model request |
| `gen_ai.content.output` | `gen_ai.output.messages` | Model completion or stream end |
| `gen_ai.content.error` | `gen_ai.response.error_body` | Non-success provider response |
| `gen_ai.tool.content.input` | `gen_ai.tool.call.arguments` | Before platform or internal tool execution |
| `gen_ai.tool.content.output` | `gen_ai.tool.call.result` | After platform or internal tool execution |

In Tempo, find the conversation, turn, or run first, then inspect Events on `agent.model.stream`, `gen_ai.chat.complete`, `agent.tool.execute`, or `agent.tool.internal`. In Loki, filter the Agent service and event names:

```text
{service_name="luna-agent"} | json | event_name=~"gen_ai\\.(content|tool\\.content)\\..*"
```

Set the switch back to `false` and restart the Agent after diagnosis. Restrict Tempo/Loki access and use short retention periods; do not use this mode as permanent audit logging.

## Import the Agent / LLM dashboard

The repository provides two Grafana dashboards that can be imported directly:

- `grafana/dashboards/luna-devops-overview.json` covers platform services, API, Worker, delivery workflows, Agent, and database health;
- `grafana/dashboards/luna-agent-llm-observability.json` focuses on Agent Runs, model latency, tokens, tools, human interaction, prompts/responses, and traces.

When importing the Agent / LLM dashboard, map its Prometheus, Tempo, and Loki variables to your existing data sources. The top filters accept `Conversation ID`, `Turn ID`, `Run ID`, `Trace ID`, and tool names. Leave an ID as `.*` to include all matching data in the selected time range. The dashboard trace tables query only `agent.run.execute` root spans, so database child spans such as `pg.query:*` and `pg-pool.connect` are not shown as Agent results by default. Use this investigation order:

1. Check Run success, model error ratio, and first-token p95 to distinguish orchestration, provider, and response-experience problems.
2. Inspect Run, model, token, and tool trends to identify the affected time and operation.
3. Enter a conversation, turn, or Run ID, then open the full trace from the trace table.
4. Enable sensitive content capture only when raw model behavior is required, then inspect prompt, response, and tool-content panels.

Content logs produced inside an Agent Run automatically carry `gen_ai.conversation.id`, `luna.turn.id`, `luna.run.id`, `trace_id`, and `span_id`. The same dashboard filters therefore constrain Tempo and Loki without copying correlation identifiers out of log messages.

## Query traces

Luna DevOps uses request-scoped traces. An AI conversation can contain multiple turns, so a conversation is not one indefinitely growing trace. One user turn normally produces one trace; resuming the same Run after approval or user input continues its original trace. Use these stable attributes to correlate each level:

| Scope | Attribute | Purpose |
| --- | --- | --- |
| AI conversation | `gen_ai.conversation.id` | Find every turn in one conversation |
| Conversation turn | `luna.turn.id` | Find one user turn and its complete processing path |
| Agent execution | `luna.run.id` | Find one queued, resumed, or retried execution instance |
| Tool call | `luna.tool_call.id` | Locate one specific tool call |
| Tool name | `gen_ai.tool.name` | Filter or aggregate by tool type |

Use these queries in the Grafana Tempo TraceQL editor:

```text title="Find all turns in one AI conversation"
{ resource.service.name = "luna-agent" && span.gen_ai.conversation.id = "aicnv_xxx" }
```

```text title="Find one conversation turn"
{ span.luna.turn.id = "aitrn_xxx" }
```

```text title="Find one Run or tool call"
{ span.luna.run.id = "airun_xxx" }
{ span.luna.tool_call.id = "aitool_xxx" }
```

```text title="Find failed traces in one conversation"
{ span.gen_ai.conversation.id = "aicnv_xxx" } && { span:status = error }
```

```text title="Find model stages slower than five seconds"
{ resource.service.name = "luna-agent" && span:name = "agent.model.stream" && span:duration > 5s }
```

The TraceQL intrinsic for a span name is `span:name`. After opening a trace, the trace-view filter uses `span.name`. Detail-view filters affect only the current browser view; dashboard JSON cannot persist one into a subsequently opened Explore page. The Agent does not capture automatic PostgreSQL query spans by default. For per-query SQL diagnostics, temporarily set `AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS=true`, restart the Agent, and disable it again when the investigation is complete.

### Main span names

| Category | Span name or pattern | Meaning |
| --- | --- | --- |
| Inbound HTTP | Usually `METHOD /route` | API or Agent HTTP entry; prefer HTTP method, route, and status attributes for filtering |
| Outbound HTTP | Automatically generated HTTP client spans | Calls to Agent, model providers, external platforms, or callbacks |
| Agent Run | `agent.run.execute` | Execution boundary of one Agent Run |
| Streaming model | `agent.model.stream` | Streaming generation for the main conversation |
| Non-streaming model | `gen_ai.chat.complete` | Titles, suggestions, and other non-streaming model calls |
| Model health | `gen_ai.chat.health` | Provider connectivity check |
| Platform tool | `agent.tool.execute` | Tool execution that calls the Luna API |
| Internal Agent tool | `agent.tool.internal` | Agent-local cards, options, routing, and similar tools |
| Tool API request | `luna_api.tool.execute` | Delegation exchange and tool request from Agent to Luna API |
| Provider configuration | `luna_api.provider_config.get` | Agent retrieval of the current model and runtime configuration |
| Task enqueue | `task.enqueue.<task_type>` | API sends work to Asynq; `:` in a task type becomes `.` |
| Task processing | `task.process.<task_type>` | Worker consumes a task, for example `task.process.deploy.run` |
| Build stages | `worker.build.*` | Capacity, namespace, task resolution, BuildKit Job, result following, and settlement |
| Deployment stages | `worker.deploy.*` | Dependency resolution, runtime configuration, resources, hooks, and rollout verification |
| Gateway stages | `worker.gateway.*` | Namespace, route resources, DNS, and certificate observation |
| Runtime and billing | `worker.runtime.*`, `worker.billing.*` | Release status synchronization and runtime or storage settlement |
| Notifications and cleanup | `worker.notification.*`, `worker.cleanup.*`, `worker.retention.*` | Notification delivery, resource cleanup, and retention work |
| Database migration | `database.migrate` | Schema migration; successful startup connectivity checks do not create traces |
| PostgreSQL automatic spans | `pg.query:*`, `pg-pool.connect`, or SQL operation names | Fine-grained SQL and pool latency; hide them in the trace view when not needed |
| DNS and gateway probes | `dns.lookup_cname`, `gateway_probe.*` | DNS lookup and gateway probe collection |

Worker stages also carry `task.type`, `task.id`, and `task.retry_count`. To find one asynchronous task, use:

```text
{ span.task.id = "task_xxx" }
```

To find deployment task traces by type, use:

```text
{ resource.service.name = "luna-worker" && span.task.type = "deploy:run" }
```

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

- Successful machine probes such as `/healthz`, `/internal/health/live`, and `/internal/health/ready` remain available as health metrics but do not emit traces or access logs. A failed Agent readiness check still emits a structured warning. User-initiated Provider, cluster, and registry connectivity tests are diagnostics rather than probes and retain full telemetry.
- Use batch and memory limiter processors in the Collector so a temporary backend outage cannot exhaust application resources.
- Keep error and slow traces, then sample healthy traces according to capacity. Apply the sampling policy centrally in the Collector instead of configuring conflicting policies per service.
- Keep the Collector and backend on a controlled network. Use TLS and authenticated headers across network boundaries.
- Telemetry does not record cookies, tokens, secrets, passwords, request bodies, model prompts, or sensitive tool arguments by default. Only the explicit Agent sensitive-content mode records AI content after mandatory redaction and size limits. The backend should still enforce access control and retention limits.
- API's dedicated Prometheus `/metrics` endpoint is a compatibility scrape surface only. Worker and Agent do not expose separate metrics ports. Complete platform metrics flow through OTLP into the Collector and metrics backend, avoiding cross-process scraping, duplicate ingestion, and counter jitter across replicas.
- After importing `grafana/dashboards/luna-devops-overview.json`, start with reporting-service count, error ratio, and success ratios; then inspect API latency, Worker queues and delivery outcomes, Agent first-token/tool behavior, and database capacity. Move to `grafana/dashboards/luna-agent-llm-observability.json` only when model behavior needs investigation, so high-cardinality content does not crowd the platform overview. Stats represent current health, time series show trends and percentiles, logs correlate raw events with traces, and tables are used for trace search and slow dependency details.

Configure Collector receivers, processors, and exporters for the selected backend by following the [OpenTelemetry Collector deployment guide](https://opentelemetry.io/docs/collector/deploy/).
