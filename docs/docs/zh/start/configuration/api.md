# API 配置

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
| `BOOTSTRAP_TOKEN`<sup>2</sup> | 空 | 初始化首个管理员；填写一次性 Token 字符串。 |
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
2. 说明：初始化完成后移除或轮换 `BOOTSTRAP_TOKEN`。
3. 说明：API 与 Agent 必须使用同一个 `AI_INTERNAL_SECRET`。

## 高级配置

### 运行与开发

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `ENV_FILE` | `.env` | 指定 API 读取的环境文件；填写文件路径。 |
| `APP_ENABLE_HSTS` | 生产为 `true` | 控制 API 是否发送 HSTS Header；可填 `true` 或 `false`。 |
| `APP_VERSION` | 构建版本 | 设置 API 对外报告的版本；填写版本字符串。 |
| `LOCAL_ADMIN_EMAIL` | `admin@luna.dev` | 设置开发模式管理员邮箱；填写有效邮箱地址。 |
| `LOCAL_ADMIN_PASSWORD` | `devops` | 设置开发模式管理员密码；填写非空字符串。 |
| `LOCAL_ADMIN_NAME` | `Platform Admin` | 设置开发模式管理员名称；填写显示名称。 |
| `LOCAL_ADMIN_FREE_QUOTA_CREDITS` | `1000` | 设置开发管理员的免费额度；填写非负 Credits 数值。 |

### 数据库

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `DB_MAX_OPEN_CONNS` | `20` | 限制数据库最大打开连接数；填写正整数。 |
| `DB_MAX_IDLE_CONNS` | `5` | 限制数据库最大空闲连接数；填写非负整数。 |
| `DB_CONN_MAX_LIFETIME` | `30m` | 限制数据库连接总寿命；填写 Go duration，如 `30m`。 |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | 限制数据库连接空闲时间；填写 Go duration，如 `5m`。 |

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

### AI 助手

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `AI_AGENT_ADDR` | 空 | 设置 API 访问 Agent 的备用地址；填写 HTTP(S) URL。 |

### Docker Compose 与 Web 构建

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | 选择 Docker Compose 使用的 API 和 Web 镜像版本；填写镜像标签。 |
| `API_METRICS_ENABLED` | 继承 `METRICS_ENABLED` | 覆盖 Compose API 的指标开关；可填 `true` 或 `false`。 |
| `API_METRICS_ADDR` | `:9090` | 覆盖 Compose API 的指标监听地址；填写 `IP:端口` 或 `:端口`。 |
| `API_METRICS_PATH` | 继承 `METRICS_PATH` | 覆盖 Compose API 的指标接口路径；填写以 `/` 开头的 HTTP 路径。 |
| `VITE_DOCS_BASE_URL` | 官方文档站 | 设置 Web 中的文档入口；填写构建时可用的 HTTP(S) URL。 |
