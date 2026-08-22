# Agent Configuration

## Basic configuration

| Setting | Default | Description |
| --- | --- | --- |
| `NODE_ENV` | `development` | Selects the Agent runtime mode; use `development`, `test`, or `production`. |
| `HOST` | `127.0.0.1` | Sets the Agent listen host; use an IP address or hostname. |
| `PORT` | `8091` | Sets the Agent listen port; use an integer from `1` to `65535`. |
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
| `AI_OBSERVABILITY_CAPTURE_CONTENT` | `false` | Controls capture of redacted, length-limited model and tool content; use `true` or `false`. |
| `AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS` | `false` | Controls whether each PostgreSQL query produces a span; use `true` or `false`. |

### Context management

| Setting | Default | Description |
| --- | --- | --- |
| `AI_CONTEXT_COMPRESSION_TRIGGER_RATIO`<sup>2</sup> | `0.9` | Sets the context-usage ratio that triggers compression; use `0.5`–`0.95`. |
| `AI_CONTEXT_COMPRESSION_TARGET_RATIO` | `0.7` | Sets the target context usage after compression; use `0.1`–`0.8`. |
| `AI_CONTEXT_RECENT_TURN_COUNT`<sup>3</sup> | `16` | Sets how many recent turns compression preserves; use an integer from `1` to `32`. |
| `AI_CONTEXT_MAX_RECENT_TURN_COUNT` | `32` | Limits recent turns included in context; use an integer from `2` to `64`. |
| `AI_CONTEXT_HISTORICAL_TOOL_K_TOKENS` | `64` | Limits context used by historical tool results; use an integer from `1` to `256` KiTokens. |
| `AI_TOOLS_RESULT_PAYLOAD_K_BYTES` | `512` | Limits one tool result added to context; use an integer from `4` to `4096` KiB. |

2. Note: The compression trigger ratio must exceed the target ratio.
3. Note: The recent-turn count cannot exceed its maximum.

### Docker Compose

| Setting | Default | Description |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | Selects the Agent image version used by Docker Compose; use an image tag. |
