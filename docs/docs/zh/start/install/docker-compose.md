# Docker Compose 部署

Docker Compose 是最快的体验方式，适合个人服务器、测试环境和小团队试用。它会一次启动平台依赖的所有服务，不需要你分别安装 PostgreSQL 和 Redis。

如果你准备把平台本身部署到 Kubernetes，优先看 [Kubernetes (Helm) 部署](/start/install/kubernetes)。

## 开始前准备

你需要：

- 一台能运行 Docker 的机器。
- Docker Compose。
- 能拉取 DockerHub 镜像的网络。

## 选择版本

仓库根目录的 `docker-compose.yaml` 默认使用 `nightly` 镜像。

验证指定版本时，在启动命令前设置镜像 tag：

```bash
DEVOPS_IMAGE_TAG=v0.1.0-rc.1 docker compose up -d
```

## 启动

先准备生产配置：

```bash
cp .env.example .env
```

根目录 `.env` 是 Compose 的唯一配置填写入口。为 `SECRET_ENCRYPTION_KEY` 填写稳定随机密钥，并替换 `REDIS_PASSWORD` 中的占位值。数据库全新时还要设置 `INITIAL_ADMIN_EMAIL`、`INITIAL_ADMIN_PASSWORD`；已有有效管理员时可将它们留空。管理员名称和语言可通过 `INITIAL_ADMIN_NAME`、`INITIAL_ADMIN_LANGUAGE` 设置。把 `PUBLIC_BASE_URL` 改成用户实际访问平台的 HTTP(S) 根地址；仅在本机使用时填写 `http://localhost:8088`。Redis 密码请使用字母和数字等 URL-safe 字符；Compose 会直接用它启动内置 Redis，并自动为 API 和 Worker 组装完整连接 URI。完整 Compose 固定以生产模式启动，不包含固定管理员凭据。

Compose 会按消费者白名单下发配置：日志和通用 OpenTelemetry 配置进入 API、Worker、Agent；`PUBLIC_BASE_URL`、数据卷传输上限和传输镜像只进入 API、Worker；首个管理员、CORS、指标和 AI Client 只进入 API；构建与部署策略只进入 Worker；Agent 数据库池和诊断开关只进入 Agent。API、Worker 的连接池分别使用 `API_DB_*`、`WORKER_DB_*`，不会再因共用一组值而互相挤占预算。根 `.env` 中面向源码联调的 `AI_AGENT_BASE_URL` 不会进入 Compose；API 在容器内固定通过 `http://agent:8091` 访问 Agent。

在仓库根目录执行：

```bash
docker compose up -d
```

这会启动平台及其 PostgreSQL 和 Redis。API 会在空数据库中用 `INITIAL_ADMIN_*` 创建首个管理员；健康检查通过后打开 `/login` 登录。创建完成后，可以清空这些变量；修改它们也不会覆盖已有管理员账号或密码。Compose 始终只把这些可选值透传给 API，由 API 根据数据库状态决定是否校验。

### 启用 AI 助手

AI Agent 使用独立 profile，默认不会启动。只需生成一个稳定的内部根密钥并写入 `.env`：

```bash
printf 'AI_INTERNAL_SECRET=%s\n' "$(openssl rand -hex 32)" >> .env
```

平台会隔离并保护 AI 助手的内部凭据。请保持该密钥稳定，并且不要与其他加密密钥共用。

再把同一份 `.env` 中的 `AI_ASSISTANT_AVAILABLE` 改为 `true`，然后启动。Compose 会固定使用容器内 Agent 地址，不需要改动源码联调使用的 `AI_AGENT_BASE_URL`：

```bash
docker compose --profile ai up -d
```

登录后在“全局设置 → AI 助手”配置 Provider、模型目录、访问范围和配额。Provider API Key 由平台 Secret Store 保存，不写入 `.env`。排障时可查看 `docker compose --profile ai logs -f agent`。

## 按需暴露控制台

部署完成后，再根据实际访问范围配置端口、反向代理、域名和 TLS。默认本机验证地址为：

```text
http://localhost:8088
```

Compose 会把 `.env` 中唯一一份 `PUBLIC_BASE_URL` 同时传给 API 和 Worker，用于 OAuth、Webhook 以及通知详情链接；Agent 不消费这个值。修改后需要重新创建 API 和 Worker。控制台跨域部署时再按 [API 配置](/start/configuration/api)设置 `APP_CORS_ORIGINS`。PostgreSQL 和 Redis 保留在容器网络中，不需要对外暴露。

## 检查状态

```bash
docker compose ps
docker compose logs -f api
docker compose logs -f worker
```

API 正常后就能打开控制台；Worker 正常后，构建、部署和状态同步才会工作。如果页面能打开但任务一直不执行，优先检查 Worker 日志。

## 下一步

1. 进入[首次登录](/start/first-login)，使用已配置的管理员账号登录。
2. 进入[添加基础资源](/start/connect-resources)，准备运行集群、镜像站和 Git Provider OAuth。
3. 按[日常交付](/use/workflow)创建并部署应用。

## 停止

```bash
docker compose down
```

如果确定不再需要当前数据，可以连同数据卷一起清理：

```bash
docker compose down -v
```

此操作会永久删除内置 PostgreSQL 和 Redis 数据，请先确认已有备份。

<div class="hint">
先跑起来，再慢慢配置。第一目标是进入控制台，不是一次性接好所有外部系统。
</div>
