# Worker 配置

## 基础配置

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ENV` | `production` | 选择 Worker 运行模式；可填 `production` 或 `development`。 |
| `DATABASE_URL` | 本地 PostgreSQL | 连接 PostgreSQL；填写 PostgreSQL 连接 URI。 |
| `REDIS_ADDR` | `redis://localhost:6379/0` | 连接 Redis 任务队列；填写 `redis://` 或 `rediss://` URI。 |
| `SECRET_ENCRYPTION_KEY`<sup>1</sup> | 空 | 解密平台保存的凭据；填写与 API 相同的稳定密钥。 |
| `PUBLIC_BASE_URL` | 空 | 设置任务通知中链接的平台根地址；填写 HTTP(S) URL。 |
| `LOG_FORMAT` | `auto` | 选择终端日志渲染；可填 `auto`、`console` 或 `json`，生产容器应使用 `json`。 |
| `LOG_COLOR` | `auto` | 控制 console 日志颜色；可填 `auto`、`always` 或 `never`，`NO_COLOR` 会强制关闭。 |
| `LOG_LEVEL` | `info` | 设置日志级别；可填 `debug`、`info`、`warn` 或 `error`。 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 空 | 设置遥测数据接收端；填写 Collector 的 OTLP/HTTP URL。 |
| `BUILD_EXECUTOR_IMAGE` | `moby/buildkit:v0.24.0-rootless` | 选择构建 Job 使用的 BuildKit；填写 OCI 镜像引用。 |
| `BUILD_EGRESS_MODE`<sup>2</sup> | `restricted` | 设置构建网络出口策略；可填 `restricted` 或 `permissive`。 |
| `BUILD_PRIVATE_EGRESS_CIDRS` | 空 | 放行构建访问的私网目标；填写逗号分隔的 CIDR。 |
| `DEPLOY_ROLLOUT_TIMEOUT_SECONDS` | `600` | 设置部署等待超时；填写正整数秒数。 |
| `CERT_MANAGER_CLUSTER_ISSUER` | `letsencrypt-http01` | 选择证书签发器；填写 Kubernetes ClusterIssuer 名称。 |

1. 说明：API 与 Worker 必须使用同一个 `SECRET_ENCRYPTION_KEY`。
2. 说明：`restricted` 默认阻止元数据地址和未放行的私网目标。

## 高级配置

### 运行与数据库

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `ENV_FILE` | `.env` | 指定 Worker 读取的环境文件；填写文件路径。 |
| `DB_MAX_OPEN_CONNS` | `20` | 限制数据库最大打开连接数；填写正整数。 |
| `DB_MAX_IDLE_CONNS` | `5` | 限制数据库最大空闲连接数；填写非负整数。 |
| `DB_CONN_MAX_LIFETIME` | `30m` | 限制数据库连接总寿命；填写 Go duration，如 `30m`。 |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | 限制数据库连接空闲时间；填写 Go duration，如 `5m`。 |

### 可观测

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `OTEL_RESOURCE_ATTRIBUTES` | 空 | 设置 OpenTelemetry 资源属性；填写逗号分隔的 `key=value`。 |
| `OTEL_EXPORTER_OTLP_HEADERS` | 空 | 鉴权 Collector 请求；填写逗号分隔的 `key=value` Header。 |

### 构建

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `BUILD_JOB_TIMEOUT_SECONDS` | `1800` | 设置构建 Job 的执行超时；填写正整数秒数。 |
| `BUILD_JOB_TTL_SECONDS` | `3600` | 设置完成后保留构建 Job 的时间；填写非负整数秒数。 |
| `BUILD_CACHE_ENABLED` | `false` | 控制是否读写 BuildKit Registry 缓存；可填 `true` 或 `false`。 |
| `BUILD_CACHE_TAG` | `buildcache` | 设置 Registry 缓存使用的标签；填写合法 OCI 镜像标签。 |
| `BUILD_PRIVATE_EGRESS_PORTS` | `443` | 限制构建访问私网目标的端口；填写逗号分隔的 `1`–`65535` 端口。 |
| `BUILD_BLOCKED_EGRESS_CIDRS` | 空 | 追加构建禁止访问的网段；填写逗号分隔的 CIDR。 |

### 数据卷导入与导出

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `VOLUME_TRANSFER_MAX_BYTES` | `100Gi` | 限制单次数据卷导入或导出大小；填写 `1Gi`–`5Ti` 容量值。 |
| `VOLUME_TRANSFER_JOB_IMAGE` | 空 | 选择数据卷传输 Pod 使用的程序版本；填写与 Worker 同版本的 OCI 镜像引用。 |

Helm 与 Docker Compose 默认复用当前 Worker 版本的镜像。源码或二进制部署需要导入导出时，应显式设置同版本 `VOLUME_TRANSFER_JOB_IMAGE`。数据流不经过对象存储。

### Docker Compose

| 配置项名称 | 默认值 | 说明 |
| --- | --- | --- |
| `DEVOPS_IMAGE_TAG` | `nightly` | 选择 Docker Compose 使用的 Worker 镜像版本；填写镜像标签。 |
