# Docker Compose 部署

Docker Compose 是最快的体验方式，适合个人服务器、测试环境和小团队试用。它会一次启动平台依赖的所有服务，不需要你分别安装 PostgreSQL 和 Redis。

如果你准备把平台本身部署到 Kubernetes，优先看 [Kubernetes (Helm) 部署](/guide/deployment/kubernetes-helm)。

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
```

验证指定版本时，在启动命令前设置镜像 tag：

```bash
DEVOPS_IMAGE_TAG=v0.1.0-rc.1 docker compose up -d
```

## 启动

先准备生产配置：

```bash
cp .env.production.example .env
```

编辑 `.env`，替换 `SECRET_ENCRYPTION_KEY`、`BOOTSTRAP_TOKEN` 和 `REDIS_PASSWORD` 的占位值。完整 Compose 默认以生产模式启动，不会暴露固定开发管理员；Redis 也会强制校验密码。

在仓库根目录执行：

```bash
docker compose up -d
```

这会启动 PostgreSQL、带密码认证的 Redis、API 和 Worker。API 会先完成数据库 migration；只有 `/healthz` 通过后 Compose 才启动 Worker，因此全新数据库不会被 Worker 提前访问。API 镜像已经内嵌前端页面，不需要单独启动 Vite。第一次进入时打开 `/bootstrap`，使用 `.env` 中的 `BOOTSTRAP_TOKEN` 创建首个管理员，完成后轮换或移除该一次性 Token。

如果想从当前源码构建镜像：

```bash
docker compose -f docker-compose-build.yaml up -d --build
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

1. 进入 [初始化控制台](/guide/product)，创建管理员或登录。
2. 进入 [连接集群和镜像站](/guide/workspace)，准备运行集群和镜像站。
3. 按 [部署上线一个 Web 项目](/operations/deploy-web-project) 跑通第一条应用交付链路。

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
