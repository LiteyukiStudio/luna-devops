# Liteyuki DevOps

面向个人开发者和小团队的 DevOps 应用交付平台。

平台目标是把代码仓库、镜像站、构建、部署、网关和域名打通，让用户只需要维护代码、`Dockerfile` 和少量配置，就可以把应用部署成一个可访问的服务。

## 核心能力

- 项目与应用管理
- 本地账号与 OIDC 登录准入
- Gitea / GitHub 仓库绑定
- Git webhook 触发平台构建
- Kubernetes Job + BuildKit rootless 构建镜像
- Harbor / Gitea Registry / DockerHub 镜像站接入
- Kubernetes / K3s 部署
- Ingress / Traefik 网关与域名
- 自定义域名与 HTTP Challenge 证书
- 发布记录与回滚
- 公开站点配置：title、logo、favicon 等元数据可由后台配置
- 多主题：亮色、暗色、跟随系统

## 技术栈

后端：

- Go
- Gin
- GORM
- PostgreSQL
- Redis + Asynq
- client-go
- OpenAPI

前端：

- Vite
- React
- TypeScript
- Tailwind CSS
- shadcn/ui
- TanStack Query
- React Hook Form + Zod
- i18next
- Sonner toast
- pnpm

Python 工具链：

- uv

## Monorepo 结构

- Go 后端位于仓库根目录。
- 前端位于 `web/` 目录。
- 本地数据库和队列使用 `docker-compose-dev.yaml` 启动 PostgreSQL 与 Redis。

## 本地开发

启动开发所需组件：

```bash
docker compose -f docker-compose-dev.yaml up -d
```

启动后端 API：

```bash
go run ./cmd/api
```

运行模式：

- `APP_ENV=development`：启用开发默认管理员，并由后端下发登录页开发账号提示。
- `APP_ENV=production`：禁用开发默认管理员；如果没有平台管理员，需要先访问 `/bootstrap` 初始化首个管理员。
- 生产模式不会返回或显示开发默认账号、默认密码等调试提示。
- 未设置 `APP_ENV` 时，`go run` 会按开发模式处理，普通二进制和容器默认按生产模式处理。

使用本地 `.env.*` 配置文件启动：

```bash
cp .env.example .env.local
ENV_FILE=.env.local go run ./cmd/api
```

启动 worker：

```bash
go run ./cmd/worker
```

使用同一份本地配置启动 worker：

```bash
ENV_FILE=.env.local go run ./cmd/worker
```

启动前端：

```bash
cd web
pnpm install
pnpm dev
```

开发环境前端请求 `/api/v1`，由 Vite proxy 反代到 `http://localhost:8080`。

## 容器运行

构建并启动完整平台：

```bash
docker compose up --build
```

访问前端：

```text
http://localhost:8088
```

容器链路：

```text
browser
  -> web nginx :80
  -> /api/* proxy
  -> api :8080
  -> postgres / redis

worker
  -> postgres / redis
```

各组件镜像：

- `api`: 根目录 `Dockerfile`，`TARGET=api`。
- `worker`: 根目录 `Dockerfile`，`TARGET=worker`。
- `web`: `web/Dockerfile`，Nginx 承载静态资源并反代 `/api/`。

生产环境可以继续将 `web` 服务放到 Traefik/Ingress 后面。

## 文档

阅读顺序：

1. [产品与一体化方案](docs/01-产品与一体化方案.md)
2. [项目技术栈要求](docs/02-项目技术栈要求.md)
3. [产品原型](docs/03-产品原型.html)
4. [AI 能力提案](docs/04-AI能力提案.md)
5. [TODO](TODO.md)

## 开发约定

- 前端必须使用 `pnpm`。
- Python 必须使用 `uv`。
- Go 后端使用 `Gin + GORM`。
- 平台构建主路径使用 `Kubernetes Job + BuildKit rootless`。
- Gitea/GitHub Actions 仅作为可选 BuildProvider。
- 部署由平台统一执行和记录。
