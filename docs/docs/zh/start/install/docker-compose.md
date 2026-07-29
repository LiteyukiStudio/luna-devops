# Docker Compose 部署

Docker Compose 是最快的体验方式，适合个人服务器、测试环境和小团队试用。它会一次启动平台依赖的所有服务，不需要你分别安装 PostgreSQL 和 Redis。

如果你准备把平台本身部署到 Kubernetes，优先看 [Kubernetes (Helm) 部署](/start/install/kubernetes)。

## 开始前准备

你需要：

- 一台能运行 Docker 的机器。
- Docker Compose。
- 能拉取 DockerHub 镜像的网络。
- 宿主机 `8088` 端口空闲。

## 选择版本

仓库根目录的 `docker-compose.yaml` 默认拉取：

```text
liteyukistudio/devops-api:nightly
liteyukistudio/devops-worker:nightly
liteyukistudio/devops-agent:nightly（仅 AI profile）
```

验证指定版本时，在启动命令前设置镜像 tag：

```bash
DEVOPS_IMAGE_TAG=v0.1.0-rc.1 docker compose up -d
```

## 启动

先准备生产配置：

```bash
cp .env.example .env
```

编辑 `.env`，为 `SECRET_ENCRYPTION_KEY` 填写稳定随机密钥，并替换 `BOOTSTRAP_TOKEN` 和 `REDIS_PASSWORD` 中的占位值。Redis 密码请使用字母和数字等 URL-safe 字符；Compose 会直接用它启动内置 Redis，并自动为 API 和 Worker 组装完整连接 URI。完整 Compose 默认以生产模式启动，不会暴露固定开发管理员。

在仓库根目录执行：

```bash
docker compose up -d
```

这会启动 PostgreSQL、带密码认证的 Redis、API 和 Worker。API 会先完成数据库 migration；只有 `/healthz` 通过后 Compose 才启动 Worker，因此全新数据库不会被 Worker 提前访问。API 镜像已经内嵌前端页面，不需要单独启动 Vite。第一次进入时打开 `/bootstrap`，使用 `.env` 中的 `BOOTSTRAP_TOKEN` 创建首个管理员，完成后轮换或移除该一次性 Token。

### 启用 AI 助手

AI Agent 使用独立 profile，默认不会启动。先在 `.env` 中设置：

- `AI_AGENT_SERVICE_TOKEN` 与 `AI_ACTOR_CONTEXT_SIGNING_KEY`：至少 32 字符且彼此独立；
- `AI_AGENT_CALLBACK_SERVICE_TOKEN`：Agent 回调 Luna API 的独立服务凭据；
- `AI_RUN_ACTOR_GRANT_SIGNING_KEY` 与 `AI_DELEGATION_TOKEN_SIGNING_KEY`：Run Grant 与短时 Delegation 的独立签名键；
- `AI_RUN_GRANT_ENCRYPTION_KEY_BASE64`：32 个随机字节的 Base64，必须稳定保存。

然后启动：

```bash
AI_ASSISTANT_AVAILABLE=true docker compose --profile ai up -d
```

登录后在“全局设置 → AI 助手”配置 Provider、模型、访问范围和配额。Provider API Key 由平台 Secret Store 保存，不写入 `.env`。排障时可查看 `docker compose --profile ai logs -f agent`。

如果想从当前源码构建镜像：

```bash
docker compose -f docker-compose-build.yaml up -d --build
# 连同 Agent：
AI_ASSISTANT_AVAILABLE=true docker compose -f docker-compose-build.yaml --profile ai up -d --build
```

## 打开控制台

浏览器访问：

```text
http://localhost:8088
```

默认 Compose 只把 API 暴露到宿主机 `8088`。PostgreSQL 和 Redis 留在容器网络里，不占用宿主机 `5432` 和 `6379`。

## 检查状态

```bash
docker compose ps
docker compose logs -f api
docker compose logs -f worker
```

API 正常后就能打开控制台；Worker 正常后，构建、部署和状态同步才会工作。如果页面能打开但任务一直不执行，优先检查 Worker 日志。

## 下一步

1. 进入 [首次登录](/start/first-login)，创建管理员或登录。
2. 进入 [连接集群和镜像站](/start/connect-resources)，准备运行集群和镜像站。
3. 按 [部署上线一个 Web 项目](/start/first-project) 跑通第一条应用交付链路。

## 停止

```bash
docker compose down
```

连数据一起清理：

```bash
docker compose down -v
```

<div class="hint">
先跑起来，再慢慢配置。第一目标是进入控制台，不是一次性接好所有外部系统。
</div>
