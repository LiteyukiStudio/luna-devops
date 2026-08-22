# Agent 配置

## 基础配置

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `NODE_ENV` | `development` | 选择 Agent 运行模式；可填 `development`、`test` 或 `production`。 |
| `HOST` | `127.0.0.1` | 设置 Agent 监听主机；填写 IP 或主机名。 |
| `PORT` | `8091` | 设置 Agent 监听端口；填写 `1`–`65535` 的整数。 |
| `LOG_FORMAT` | `auto` | 选择终端日志渲染；可填 `auto`、`console` 或 `json`，生产容器应使用 `json`。 |
| `LOG_COLOR` | `auto` | 控制 console 日志颜色；可填 `auto`、`always` 或 `never`，`NO_COLOR` 会强制关闭。 |
| `LOG_LEVEL` | `info` | 设置日志级别；可填 `debug`、`info`、`warn` 或 `error`。 |
| `DATABASE_URL` | 空 | 连接 PostgreSQL；填写 PostgreSQL 连接 URI。 |
| `AUTH_MODE` | `development` | 选择内部请求鉴权模式；可填 `development` 或 `bff-hmac`。 |
| `AI_INTERNAL_SECRET`<sup>1</sup> | 空 | 鉴权 API 与 Agent 的内部请求；填写至少 32 字节的密钥。 |
| `LUNA_API_BASE_URL` | 空 | 设置 Agent 访问 API 的地址；填写 HTTP(S) URL。 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 空 | 设置遥测数据接收端；填写 Collector 的 OTLP/HTTP URL。 |

1. 说明：生产环境必须使用 `bff-hmac`，并与 API 共享同一个 `AI_INTERNAL_SECRET`。

## 高级配置

### 运行时

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `INSTANCE_ID` | 自动生成 | 标识当前 Agent 实例；填写 `1`–`128` 个字符。 |

### 可观测

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `OTEL_RESOURCE_ATTRIBUTES` | 空 | 设置 OpenTelemetry 资源属性；填写逗号分隔的 `key=value`。 |
| `OTEL_EXPORTER_OTLP_HEADERS` | 空 | 鉴权 Collector 请求；填写逗号分隔的 `key=value` Header。 |
| `OTEL_SERVICE_VERSION` | 空 | 标记 Agent 的遥测版本；填写版本字符串。 |
| `AI_OBSERVABILITY_CAPTURE_CONTENT` | `false` | 控制是否在受控 Trace 中记录脱敏且限长的模型与工具内容；可填 `true` 或 `false`，日志始终只保留元数据。 |
| `AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS` | `false` | 控制是否为每条 PostgreSQL 查询生成 Span；可填 `true` 或 `false`。 |

### 上下文管理

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `AI_CONTEXT_COMPRESSION_TRIGGER_RATIO`<sup>2</sup> | `0.9` | 设置上下文使用率达到何值时触发压缩；填写 `0.5`–`0.95`。 |
| `AI_CONTEXT_COMPRESSION_TARGET_RATIO` | `0.7` | 设置压缩后的目标上下文使用率；填写 `0.1`–`0.8`。 |
| `AI_CONTEXT_RECENT_TURN_COUNT`<sup>3</sup> | `16` | 设置压缩时保留的近期对话轮数；填写 `1`–`32` 的整数。 |
| `AI_CONTEXT_MAX_RECENT_TURN_COUNT` | `32` | 限制上下文携带的近期对话轮数；填写 `2`–`64` 的整数。 |
| `AI_CONTEXT_HISTORICAL_TOOL_K_TOKENS` | `64` | 限制历史工具结果占用的上下文；填写 `1`–`256` 的整数，单位为 KiToken。 |
| `AI_TOOLS_RESULT_PAYLOAD_K_BYTES` | `512` | 限制单次工具结果进入上下文的大小；填写 `4`–`4096` 的整数，单位为 KiB。 |

2. 说明：压缩触发比例必须大于目标比例。
3. 说明：近期轮数不能大于近期轮数上限。

### Docker Compose

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | 选择 Docker Compose 使用的 Agent 镜像版本；填写镜像标签。 |
