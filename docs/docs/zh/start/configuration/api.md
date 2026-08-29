# API 配置

源码运行时，API 从根目录 `.env` 读取这些变量；Docker Compose 也只读取这一份 `.env`，但会按消费者白名单仅把 API 所需变量注入 API 容器。`PUBLIC_BASE_URL`、日志、OpenTelemetry 和数据卷传输上限等公共值只填写一次，再由 Compose 同步给实际消费者。

## 基础配置

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ENV` | `production` | 选择 API 运行模式；可填 `production` 或 `development`。 |
| `API_ADDR` | `:8080` | 设置 API 监听地址；填写 `IP:端口` 或 `:端口`。 |
| `PUBLIC_BASE_URL` | 空 | 设置用户访问平台的根地址；填写 HTTP(S) URL。 |
| `DATABASE_URL` | 本地 PostgreSQL | 连接 PostgreSQL；填写 PostgreSQL 连接 URI。 |
| `REDIS_ADDR` | `redis://localhost:6379/0` | 连接 Redis；填写 `redis://` 或 `rediss://` URI。 |
| `REDIS_PASSWORD` | 空 | 设置 Docker Compose 内置 Redis 的密码；填写密码字符串，未启用认证时留空。 |
| `SECRET_ENCRYPTION_KEY`<sup>1</sup> | 空 | 加密平台保存的凭据；填写稳定的非空密钥。 |
| `INITIAL_ADMIN_EMAIL`<sup>2</sup> | 空 | 在全新数据库创建首个管理员；填写有效的纯邮箱地址。 |
| `INITIAL_ADMIN_PASSWORD`<sup>2</sup> | 空 | 设置首个管理员的初始密码；填写 8–72 字节的强密码并通过 Secret 提供。 |
| `INITIAL_ADMIN_NAME`<sup>2</sup> | 空 | 设置首个管理员的显示名称；填写名称或留空以使用邮箱。 |
| `INITIAL_ADMIN_LANGUAGE`<sup>2</sup> | `zh-CN` | 设置首个管理员的语言；可填 `zh-CN` 或 `en-US`。 |
| `APP_CORS_ORIGINS` | 空 | 允许浏览器跨域访问；填写逗号分隔的 HTTP(S) Origin。 |
| `TRUSTED_PROXY_CIDRS` | 空 | 识别可信反向代理；填写逗号分隔的 CIDR。 |
| `LOG_FORMAT` | `auto` | 选择终端日志渲染；可填 `auto`、`console` 或 `json`，生产容器应使用 `json`。 |
| `LOG_COLOR` | `auto` | 控制 console 日志颜色；可填 `auto`、`always` 或 `never`，`NO_COLOR` 会强制关闭。 |
| `LOG_LEVEL` | `info` | 设置日志级别；可填 `debug`、`info`、`warn` 或 `error`。 |
| `AI_ASSISTANT_AVAILABLE` | `false` | 控制 AI 助手是否可用；可填 `true` 或 `false`。 |
| `AI_AGENT_BASE_URL` | 空 | 设置 API 访问 Agent 的地址；填写 HTTP(S) URL。 |
| `AI_AGENT_TIMEOUT` | `10s` | 限制 API 对 Agent 的非流式请求时长；填写 Go duration（如 `10s` 或 `1m`），SSE 不受此项限制。 |
| `AI_INTERNAL_SECRET`<sup>3</sup> | 空 | 鉴权 API 与 Agent 的内部请求；填写至少 32 字节的密钥。 |

1. 说明：生产环境必须设置 `SECRET_ENCRYPTION_KEY`；更换后已有加密凭据将无法读取。
2. 说明：这些配置只在数据库从未存在用户时创建首个管理员；已有有效管理员时不会覆盖账号或密码，已有用户但没有有效管理员时 API 会拒绝启动并要求先恢复管理员。
3. 说明：API 与 Agent 必须使用同一个 `AI_INTERNAL_SECRET`。

## 高级配置

### 运行与开发

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `ENV_FILE` | `.env` | 指定 API 读取的环境文件；填写文件路径。 |
| `APP_ENABLE_HSTS` | 生产为 `true` | 控制 API 是否发送 HSTS Header；可填 `true` 或 `false`。 |
| `APP_VERSION` | 构建版本 | 设置 API 对外报告的版本；填写版本字符串。 |
| `LOCAL_ADMIN_FREE_QUOTA_CREDITS` | `1000` | 设置开发模式下首个管理员创建时发放的免费额度；填写非负 Credits 数值。 |

### 数据库

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `API_DB_MAX_OPEN_CONNS` | `20` | 限制每个 API 副本的数据库最大打开连接数；填写正整数。 |
| `API_DB_MAX_IDLE_CONNS` | `5` | 限制每个 API 副本的数据库最大空闲连接数；填写非负整数。 |
| `API_DB_CONN_MAX_LIFETIME` | `30m` | 限制 API 数据库连接总寿命；填写 Go duration，如 `30m`。 |
| `API_DB_CONN_MAX_IDLE_TIME` | `5m` | 限制 API 数据库连接空闲时间；填写 Go duration，如 `5m`。 |

### 指标与可观测

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `METRICS_ENABLED` | `false` | 控制是否暴露 Prometheus 指标；可填 `true` 或 `false`。 |
| `METRICS_ADDR` | `:9090` | 设置指标服务监听地址；填写 `IP:端口` 或 `:端口`。 |
| `METRICS_PATH` | `/metrics` | 设置指标接口路径；填写以 `/` 开头的 HTTP 路径。 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 空 | 设置遥测数据接收端；填写 Collector 的 OTLP/HTTP URL。 |
| `OTEL_RESOURCE_ATTRIBUTES` | 空 | 设置 OpenTelemetry 资源属性；填写逗号分隔的 `key=value`。 |
| `OTEL_EXPORTER_OTLP_HEADERS` | 空 | 鉴权 Collector 请求；填写逗号分隔的 `key=value` Header。 |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | 空 | 设置浏览器 Trace Relay 的接收端；填写 HTTP(S) URL。 |
| `OTEL_EXPORTER_OTLP_TRACES_HEADERS` | 空 | 鉴权浏览器 Trace Relay 请求；填写逗号分隔的 `key=value` Header。 |

### 数据卷导入与导出

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `VOLUME_TRANSFER_MAX_BYTES` | `100Gi` | 限制单次数据卷导入或导出大小；填写 `1Gi`–`5Ti` 容量值。 |

数据卷内容在客户端、API 和运行集群 Transfer Pod 之间直接流式传输，不需要对象存储、回调地址或 API 本地完整暂存。

### Docker Compose 与 Web 构建

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | 选择 Docker Compose 使用的 API 和 Web 镜像版本；填写镜像标签。 |
| `VITE_DOCS_BASE_URL` | 官方文档站 | 设置 Web 中的文档入口；填写构建时可用的 HTTP(S) URL。 |
