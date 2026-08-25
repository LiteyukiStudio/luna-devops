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
| `REDIS_ADDR` | 空 | 为活动 Run 提供短期事件传输与跨实例回放；填写必需的 Redis 连接 URI。 |
| `AUTH_MODE` | `development` | 选择内部请求鉴权模式；可填 `development` 或 `bff-hmac`。 |
| `AI_INTERNAL_SECRET`<sup>1</sup> | 空 | 鉴权 API 与 Agent 的内部请求；填写至少 32 字节的密钥。 |
| `LUNA_API_BASE_URL` | 空 | 设置 Agent 访问 API 的地址；填写 HTTP(S) URL。 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 空 | 设置遥测数据接收端；填写 Collector 的 OTLP/HTTP URL。 |

1. 说明：生产环境必须使用 `bff-hmac`，并与 API 共享同一个 `AI_INTERNAL_SECRET`。

生产模式必须同时提供 PostgreSQL、Redis 与 Luna API 配置。就绪探针会实时检查数据库 Schema、
Provider 配置和 Redis 活动流；任一依赖不可用时返回 `503`，实例不会继续接收新 Run。

启动日志出现 `error.code=ai.stream_redis_url_required` 时，表示 `REDIS_ADDR` 未注入或值为空，
Agent 此时尚未尝试连接 Redis。请使用当前版本的 Helm Chart 或 Docker Compose 清单重新创建 Agent；
旧 Compose 清单没有转发该变量时，只修改 `.env` 不会生效。

## 高级配置

### 运行时

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `INSTANCE_ID` | 自动生成 | 标识当前 Agent 实例；填写 `1`–`128` 个字符。 |

### 数据库连接

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `AI_DATABASE_MAX_CONNECTIONS` | `10` | 限制单个 Agent 实例的 PostgreSQL 连接池；填写 `1`–`100` 的整数，并按副本总数预留数据库连接预算。 |
| `AI_DATABASE_CONNECTION_TIMEOUT_MS` | `5000` | 限制等待空闲 PostgreSQL 连接的时间；填写 `100`–`30000` 的整数，单位为毫秒。 |
| `AI_DATABASE_STATEMENT_TIMEOUT_MS` | `15000` | 限制单条 Agent SQL 的执行时间；填写 `1000`–`120000` 的整数，单位为毫秒。 |

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
| `AI_CONTEXT_COMPRESSION_TRIGGER_RATIO`<sup>2</sup> | `0.9` | 设置何时依据上一次同模型官方 `prompt_tokens` 触发压缩；填写 `0.5`–`0.95`。 |
| `AI_CONTEXT_RECENT_TURN_COUNT` | `16` | 设置主动压缩时保留的近期对话轮数；填写 `1`–`32` 的整数。 |
| `AI_CONTEXT_MAX_HISTORY_PAYLOAD_K_BYTES` | `4096` | 限制一次编译通常携带的历史负载；填写 `64`–`16384` 的整数，单位为 KiB；为保证刚完成的超大 Turn 能在下一轮按原字节重放，最新完整 Turn 必要时可临时超过该总预算。 |
| `AI_CONTEXT_MAX_SUMMARY_PAYLOAD_K_BYTES` | `512` | 限制单次摘要请求的历史负载；填写 `16`–`4096` 的整数，单位为 KiB。 |
| `AI_CONTEXT_MAX_CONTINUATION_PAYLOAD_K_BYTES` | `1024` | 限制工具续跑消息负载；填写 `16`–`4096` 的整数，单位为 KiB。 |
| `AI_TOOLS_RESULT_PAYLOAD_K_BYTES` | `512` | 限制单次工具结果进入上下文的大小；填写 `4`–`4096` 的整数，单位为 KiB。 |

2. 说明：新会话没有官方用量时会直接请求 Provider；字节上限只保护传输和内存，不代表 Token 数。

### Docker Compose

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | 选择 Docker Compose 使用的 Agent 镜像版本；填写镜像标签。 |
