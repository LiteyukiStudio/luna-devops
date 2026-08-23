# Agent Configuration

## Basic configuration

| Setting | Default | Description |
| --- | --- | --- |
| `NODE_ENV` | `development` | Selects the Agent runtime mode; use `development`, `test`, or `production`. |
| `HOST` | `127.0.0.1` | Sets the Agent listen host; use an IP address or hostname. |
| `PORT` | `8091` | Sets the Agent listen port; use an integer from `1` to `65535`. |
| `LOG_FORMAT` | `auto` | Selects terminal log rendering; use `auto`, `console`, or `json`, and use `json` in production containers. |
| `LOG_COLOR` | `auto` | Controls console log colors; use `auto`, `always`, or `never`; `NO_COLOR` always disables colors. |
| `LOG_LEVEL` | `info` | Sets log verbosity; use `debug`, `info`, `warn`, or `error`. |
| `DATABASE_URL` | Empty | Connects to PostgreSQL; use a PostgreSQL connection URI. |
| `AUTH_MODE` | `development` | Selects internal-request authentication; use `development` or `bff-hmac`. |
| `AI_INTERNAL_SECRET`<sup>1</sup> | Empty | Authenticates internal API-Agent requests; use a secret of at least 32 bytes. |
| `LUNA_API_BASE_URL` | Empty | Sets the API address used by Agent; use an HTTP(S) URL. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Empty | Sets the telemetry receiver; use the Collector OTLP/HTTP URL. |

1. Note: Production requires `bff-hmac` and the same `AI_INTERNAL_SECRET` used by API.

## Advanced configuration

### Runtime

| Setting | Default | Description |
| --- | --- | --- |
| `INSTANCE_ID` | Generated | Identifies the current Agent instance; use `1`–`128` characters. |

### Observability

| Setting | Default | Description |
| --- | --- | --- |
| `OTEL_RESOURCE_ATTRIBUTES` | Empty | Sets OpenTelemetry resource attributes; use comma-separated `key=value` pairs. |
| `OTEL_EXPORTER_OTLP_HEADERS` | Empty | Authenticates Collector requests; use comma-separated `key=value` headers. |
| `OTEL_SERVICE_VERSION` | Empty | Labels Agent telemetry with a version; use a version string. |
| `AI_OBSERVABILITY_CAPTURE_CONTENT` | `false` | Controls redacted, length-limited model and tool content in controlled traces; use `true` or `false`; logs always retain metadata only. |
| `AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS` | `false` | Controls whether each PostgreSQL query produces a span; use `true` or `false`. |

### Context management

| Setting | Default | Description |
| --- | --- | --- |
| `AI_CONTEXT_COMPRESSION_TRIGGER_RATIO`<sup>2</sup> | `0.9` | Triggers compression from the previous same-model official `prompt_tokens` ratio; use `0.5`–`0.95`. |
| `AI_CONTEXT_RECENT_TURN_COUNT` | `16` | Sets how many recent turns proactive compression preserves; use an integer from `1` to `32`. |
| `AI_CONTEXT_MAX_HISTORY_PAYLOAD_K_BYTES` | `4096` | Bounds history payload per context compilation; use an integer from `64` to `16384` KiB. |
| `AI_CONTEXT_MAX_SUMMARY_PAYLOAD_K_BYTES` | `512` | Bounds history payload per summary request; use an integer from `16` to `4096` KiB. |
| `AI_CONTEXT_MAX_CONTINUATION_PAYLOAD_K_BYTES` | `1024` | Bounds tool-continuation messages; use an integer from `16` to `4096` KiB. |
| `AI_TOOLS_RESULT_PAYLOAD_K_BYTES` | `512` | Limits one tool result added to context; use an integer from `4` to `4096` KiB. |

2. Note: A new conversation without official usage calls the Provider directly. Byte limits protect transport and memory only; they are not token counts.

### Docker Compose

| Setting | Default | Description |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | Selects the Agent image version used by Docker Compose; use an image tag. |
