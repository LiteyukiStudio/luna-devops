# 环境变量参考

API 和 Worker 都通过环境变量读取运行配置。使用 Docker Compose、Helm 或其他容器平台时，把这些值注入对应容器即可。

第一次部署只需要处理“基本”项。平台正常运行后，再按实际需求调整“进阶”项，不必一开始逐个配置。

## API 配置项

| 类型 | 配置项 | 默认值 | 用途与修改时机 |
| --- | --- | --- | --- |
| 基本 | `APP_ENV` | `production` | 运行模式；只有本地开发时显式改为 `development`。 |
| 基本 | `SECRET_ENCRYPTION_KEY` | 空 | 加密密钥；生产环境必须设置稳定随机值。 |
| 基本 | `DATABASE_URL` | `postgres://devops:devops@postgres:5432/devops?sslmode=disable` | PostgreSQL 连接串；换数据库或账号时改。 |
| 基本 | `REDIS_ADDR` | `redis://localhost:6379/0` | 完整 Redis URI，格式为 `redis://用户名:密码@域名:端口/数据库`；TLS 连接使用 `rediss://`。不再拆分配置用户名、密码或 DB。 |
| 基本 | `PUBLIC_BASE_URL` | `http://localhost:8088` | 平台外部地址；有公网域名、HTTPS、反代时改。OIDC Redirect URI 会按 `{PUBLIC_BASE_URL}/api/v1/auth/oidc/callback` 生成。 |
| 进阶 | `API_ADDR` | `:8080` | API 容器监听地址；自定义端口时改。 |
| 进阶 | `APP_CORS_ORIGINS` | `http://localhost:8088` | 允许访问 API 的前端 Origin；前后端不同域时改。 |
| 进阶 | `TRUSTED_PROXY_CIDRS` | 空 | 允许提供真实客户端地址的反向代理 CIDR，多个值用逗号分隔。默认不信任任何代理；只有确认请求必经受控代理时才配置，避免伪造 `X-Forwarded-For` 绕过按 IP 限流。 |
| 进阶 | `LOG_LEVEL` | `info` | 日志级别；本地排障时可临时改为 `debug`。 |
| 进阶 | `DB_MAX_OPEN_CONNS` | `20` | 当前 API 进程最多打开的 PostgreSQL 连接数；多副本部署时按总副本数一起控制，避免打满数据库。 |
| 进阶 | `DB_MAX_IDLE_CONNS` | `5` | 当前 API 进程保留的空闲 PostgreSQL 连接数；连接紧张时调小。 |
| 进阶 | `DB_CONN_MAX_LIFETIME` | `30m` | 单个数据库连接最长复用时间；负载均衡、连接代理或数据库滚动维护时可适当调短。 |
| 进阶 | `DB_CONN_MAX_IDLE_TIME` | `5m` | 空闲数据库连接保留时间；连接紧张时调短。 |
| 进阶 | `DB_CONNECT_RETRY_ATTEMPTS` | `12` | 启动时连接 PostgreSQL 的重试次数；数据库启动慢或短暂满连接时调大。 |
| 进阶 | `DB_CONNECT_RETRY_INTERVAL` | `5s` | 启动连接重试间隔，支持 `5s`、`1m` 或纯数字秒数。 |
| 进阶 | `METRICS_ENABLED` | `false` | 是否启用独立 Prometheus metrics listener；默认关闭。设为 `true` 后 API 会使用默认监听地址 `:9090`。 |
| 进阶 | `METRICS_ADDR` | `:9090` | metrics 监听地址；只在需要调整 API metrics 端口或绑定地址时修改。 |
| 进阶 | `METRICS_PATH` | `/metrics` | Prometheus 抓取路径；只注册在独立 metrics listener 上。 |

启用 metrics 后，API 会暴露 HTTP 请求量、延迟、错误响应、PostgreSQL 连接池和 PostgreSQL/Redis 健康检查指标。Grafana dashboard JSON 位于 `grafana/dashboards/`，需要时直接导入 Grafana。

OIDC 身份源的 Redirect URI 由 `PUBLIC_BASE_URL` 生成，后台“身份源”表单会直接展示可复制地址。准入策略默认要求 OIDC 返回非空邮箱且 `email_verified=true`；如果接入的是可信内部身份源，但无法返回标准 `email_verified`，可以在准入策略里关闭“要求 OIDC 邮箱已验证”，平台仍会要求邮箱非空。

前端未登录时会按浏览器首选语言顺序选择界面语言，目前支持 `zh-CN` 和 `en-US`。登录后以账号设置里的语言偏好为准，并写入本地缓存，方便下次打开时立即使用同一语言。

访问入口的可用域名后缀、外层访问协议、外层访问端口和 Gateway API 默认值在“运行集群”里维护。不同集群可以配置不同的网关域名后缀、GatewayClass 和共享 Gateway；同一集群也可以配置多个域名后缀。部署配置绑定到哪个集群，创建访问入口时就只能从该集群的后缀里选择一个，默认域名、短前缀补全和控制台访问链接也按该选择生成。外层 CDN 或反向代理已经提供 HTTPS 时，可以把对应集群的外层访问协议改为 `https`；它只影响控制台展示和跳转链接，不会修改集群内部 listener，也不会触发证书申请。

## 前端构建配置项

| 类型 | 配置项 | 默认值 | 用途与修改时机 |
| --- | --- | --- | --- |
| 进阶 | `VITE_DOCS_BASE_URL` | `https://luna-devops.liteyuki.org` | 文档站基础地址；账单页等帮助链接会基于它生成。文档站独立域名或路径变化时，在构建前设置。 |

## Worker 配置项

| 类型 | 配置项 | 默认值 | 用途与修改时机 |
| --- | --- | --- | --- |
| 基本 | `APP_ENV` | `production` | 运行模式；和 API 保持一致。 |
| 基本 | `SECRET_ENCRYPTION_KEY` | 空 | 解密平台密钥；必须和 API 一致。 |
| 基本 | `DATABASE_URL` | `postgres://devops:devops@postgres:5432/devops?sslmode=disable` | PostgreSQL 连接串；和 API 指向同库。 |
| 基本 | `REDIS_ADDR` | `redis://localhost:6379/0` | 完整 Redis URI；必须和 API 使用同一连接串。TLS 连接使用 `rediss://`。 |
| 基本 | `BUILD_EXECUTOR_IMAGE` | `moby/buildkit:v0.24.0-rootless` | BuildKit 镜像；构建集群拉不到默认镜像时改。 |
| 进阶 | `LOG_LEVEL` | `info` | 日志级别；本地排障时可临时改为 `debug`。 |
| 进阶 | `DB_MAX_OPEN_CONNS` | `20` | 当前 Worker 进程最多打开的 PostgreSQL 连接数；多副本部署时按总副本数一起控制，避免打满数据库。 |
| 进阶 | `DB_MAX_IDLE_CONNS` | `5` | 当前 Worker 进程保留的空闲 PostgreSQL 连接数；连接紧张时调小。 |
| 进阶 | `DB_CONN_MAX_LIFETIME` | `30m` | 单个数据库连接最长复用时间；负载均衡、连接代理或数据库滚动维护时可适当调短。 |
| 进阶 | `DB_CONN_MAX_IDLE_TIME` | `5m` | 空闲数据库连接保留时间；连接紧张时调短。 |
| 进阶 | `DB_CONNECT_RETRY_ATTEMPTS` | `12` | 启动时连接 PostgreSQL 的重试次数；数据库启动慢或短暂满连接时调大。 |
| 进阶 | `DB_CONNECT_RETRY_INTERVAL` | `5s` | 启动连接重试间隔，支持 `5s`、`1m` 或纯数字秒数。 |
| 进阶 | `METRICS_ENABLED` | `false` | 是否启用独立 Prometheus metrics listener；默认关闭。设为 `true` 后 Worker 会使用默认监听地址 `:9091`。 |
| 进阶 | `METRICS_ADDR` | `:9091` | metrics 监听地址；只在需要调整 Worker metrics 端口或绑定地址时修改。 |
| 进阶 | `METRICS_PATH` | `/metrics` | Prometheus 抓取路径；只注册在独立 metrics listener 上。 |
| 进阶 | `DEPLOY_ROLLOUT_TIMEOUT_SECONDS` | `600` | 发布等待超时；应用启动慢时调大。 |
| 进阶 | `CERT_MANAGER_CLUSTER_ISSUER` | `letsencrypt-http01` | 证书 Issuer 名称；集群名称不同时改。 |
| 进阶 | `BUILD_EGRESS_MODE` | `permissive` | 构建出站模式；需要强隔离时改为 `restricted`。 |
| 进阶 | `BUILD_JOB_TIMEOUT_SECONDS` | `1800` | 构建超时兜底；部署配置未设置超时时使用。大型项目构建慢时调大。 |
| 进阶 | `BUILD_JOB_TTL_SECONDS` | `3600` | 构建 Pod 保留时间；想看更久日志时调大。 |
| 进阶 | `BUILD_CACHE_ENABLED` | `false` | 构建缓存开关；需要加速重复构建时开启。 |
| 进阶 | `BUILD_CACHE_TAG` | `buildcache` | 构建缓存 tag；需要隔离缓存时改。 |
| 进阶 | `BUILD_NPM_REGISTRY` | 空 | npm 镜像源；需要内部源时设置。 |
| 进阶 | `BUILD_PRIVATE_EGRESS_CIDRS` | 空 | `restricted` 模式下额外允许的内网 CIDR。 |
| 进阶 | `BUILD_PRIVATE_EGRESS_PORTS` | `443` | `restricted` 模式下的内网白名单端口；非标镜像站常用 `5000`、`8081`。 |
| 进阶 | `BUILD_BLOCKED_EGRESS_CIDRS` | 空 | `restricted` 模式下额外禁止的 CIDR。 |

启用 metrics 后会暴露 worker 任务、重试、队列深度、队列延迟、构建/发布结果与耗时、运行副本、网关同步和依赖健康指标。Grafana dashboard JSON 位于 `grafana/dashboards/`，需要时直接导入 Grafana。
