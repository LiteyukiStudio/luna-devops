<p align="center">
  <img src="web/public/luna-devops-logo.svg" width="132" alt="Luna DevOps 标志" />
</p>

<h1 align="center">Luna DevOps</h1>

<p align="center">
  面向小型团队、企业与独立开发者的轻量级应用交付平台。
</p>

<p align="center">
  <strong>简体中文</strong> · <a href="README_EN.md">English</a>
</p>

<p align="center">
  <img src="web/public/images/luna-devops-banner-v4.png" alt="Luna DevOps 自动化交付流水线" />
</p>

<p align="center">
  <a href="https://luna-devops.liteyuki.org/">文档站</a>
  ·
  <a href="https://github.com/LiteyukiStudio/devops">GitHub</a>
  ·
  <a href="docs/docs/zh/guide/deployment/kubernetes-helm.md">Helm 部署</a>
  ·
  <a href="docs/docs/zh/guide/deployment/docker-compose.md">Docker Compose 部署</a>
</p>

## Luna DevOps 是什么？

Luna DevOps 将代码仓库、镜像站、BuildKit、Kubernetes、访问入口、证书、发布和计费能力串联成一条完整的应用交付流程。

目标很简单：让产品团队专注于代码，只需轻松几步即可部署自己的项目，平台负责重复而繁琐的构建与交付工作。

```text
代码仓库
  -> 构建镜像
  -> 推送镜像产物
  -> 部署到 Kubernetes / K3s
  -> 创建访问入口
  -> 跟踪状态、日志、发布历史与资源用量
```

## 主要功能

| 领域 | 已支持能力 |
| --- | --- |
| 工作空间 | 项目空间、应用、成员、角色和带审计记录的管理操作 |
| 代码仓库 | GitHub 与 Gitea 账号接入、仓库绑定和 Webhook 入口 |
| 构建 | Worker 管理的 Kubernetes Job、Rootless BuildKit、镜像标签、日志和构建记录 |
| 镜像站 | Harbor、Gitea Registry、DockerHub 和通用 OCI 镜像站 |
| 部署 | Kubernetes / K3s 工作负载、发布记录、状态同步和支持回滚的历史记录 |
| 访问入口 | Gateway API / HTTPRoute、域名、访问入口和证书自动化 |
| 平台运营 | 事件、通知、应用市场、计费和站点设置 |
| 用户体验 | React 控制台、国际化、浅色 / 深色 / 跟随系统主题和内嵌生产前端 |

## 技术栈

| 层级 | 技术栈 |
| --- | --- |
| 后端 | Go、Gin、GORM、PostgreSQL、Redis、Asynq、client-go |
| 前端 | Vite、React、TypeScript、Tailwind CSS、shadcn/ui、TanStack Query |
| 表单与交互 | React Hook Form、Zod、i18next、react-i18next、Sonner |
| 交付 | Docker Compose、Helm、Kubernetes Job、BuildKit、Gateway API |
| 工具链 | pnpm、uv、golang-migrate、OpenAPI |

## 快速开始

启动本地开发依赖：

```bash
docker compose -f docker-compose-dev.yaml up -d
```

创建本地配置：

```bash
cp .env.example .env
```

运行后端：

```bash
go run ./cmd/api
go run ./cmd/worker
```

运行前端：

```bash
pnpm --dir web install
pnpm --dir web dev
```

Vite 开发服务器会将 `/api/v1` 代理到 `http://localhost:8080`。

## 部署

Luna DevOps 支持容器、Helm 和本地二进制部署。实际使用环境推荐采用容器化部署。

| 方式 | 适用场景 | 入口 |
| --- | --- | --- |
| Kubernetes / Helm | 生产级 Kubernetes 或 K3s 集群 | [`charts/luna-devops`](charts/luna-devops) |
| Docker Compose | 单机试用、小型实验室和发版验证 | [`docker-compose.yaml`](docker-compose.yaml) |
| 二进制 | 本地调试和源码开发 | [`cmd/api`](cmd/api)、[`cmd/worker`](cmd/worker) |

使用 Docker Compose 启动已发布的容器镜像：

```bash
cp .env.example .env
# 首次启动前请填写 SECRET_ENCRYPTION_KEY、BOOTSTRAP_TOKEN 和 REDIS_PASSWORD。
docker compose up -d
```

从当前源码构建并启动完整服务：

```bash
docker compose -f docker-compose-build.yaml up -d --build
```

使用 Helm 安装：

```bash
helm install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace
```

更多部署说明：

- [Kubernetes / Helm](docs/docs/zh/guide/deployment/kubernetes-helm.md)
- [Docker Compose](docs/docs/zh/guide/deployment/docker-compose.md)
- [二进制部署](docs/docs/zh/guide/deployment/binary.md)
- [配置参考](docs/docs/zh/guide/configuration-reference.md)

## 配置说明

- `APP_ENV=development` 会启用本地开发便利功能。
- `APP_ENV=production` 会关闭开发默认值，并要求初始化管理员。
- 生产环境中的 `SECRET_ENCRYPTION_KEY` 必须保持稳定。它用于保护已保存的 Token、镜像站凭据、OAuth Secret 和其他敏感数据。
- Luna DevOps 位于反向代理之后时，`TRUSTED_PROXY_CIDRS` 应包含可信反向代理或 CDN 的出口网段。
- Worker 的构建网络可以单独配置。构建需要访问私有镜像站或镜像源时，建议使用受限出口并显式配置白名单。

完整的 API 与 Worker 配置请查看[配置参考](docs/docs/zh/guide/configuration-reference.md)。

## 仓库结构

```text
cmd/api                 API 服务入口
cmd/worker              异步 Worker 入口
internal/               后端业务域、Provider、Service 和数据模型
migrations/             PostgreSQL 数据库迁移
openapi/                OpenAPI 定义
web/                    Vite + React 控制台
web/public/             公共资源、标志、吉祥物和 favicon
docs/                   Rspress 文档站
notes/                  产品方案、工程笔记和 SOP
charts/luna-devops      Helm Chart
```

## 开发

常用检查命令：

```bash
go test ./...
pnpm --dir web lint
pnpm --dir web build
```

项目约定：

- 前端依赖统一使用 `pnpm`。
- Python 工具链统一使用 `uv`。
- 后端 Handler 保持精简，业务逻辑放入 Service，外部平台集成放入 Provider。
- 所有用户可见的前端文案都放入 i18n 文件。
- 功能或行为变化时同步更新文档站。

## 品牌资源

- 标志 / favicon：[`web/public/luna-devops-logo.svg`](web/public/luna-devops-logo.svg)
- 吉祥物：[`web/public/brand/mascot-luna-devops.png`](web/public/brand/mascot-luna-devops.png)

## 文档

- 在线文档：[luna-devops.liteyuki.org](https://luna-devops.liteyuki.org/)
- 产品方案：[`notes/01-产品与一体化方案.md`](notes/01-产品与一体化方案.md)
- 代码健康检查 SOP：[`notes/07-代码健康检查SOP.md`](notes/07-代码健康检查SOP.md)
- 开发计划：[`TODO.md`](TODO.md)
- Agent 与贡献规范：[`AGENTS.md`](AGENTS.md)
