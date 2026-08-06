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
  <a href="https://github.com/LiteyukiStudio/luna-devops">GitHub</a>
  ·
  <a href="docs/docs/zh/start/install/kubernetes.md">Helm 部署</a>
  ·
  <a href="docs/docs/zh/start/install/docker-compose.md">Docker Compose 部署</a>
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
| AI Agent | Node.js 24、TypeScript、Fastify、LangGraph.js、PostgreSQL Checkpoint |
| 前端 | Vite、React、TypeScript、Tailwind CSS、shadcn/ui、TanStack Query |
| 表单与交互 | React Hook Form、Zod、i18next、react-i18next、Sonner |
| 交付 | Docker Compose、Helm、Kubernetes Job、BuildKit、Gateway API |
| CLI | TypeScript、Commander、Zod、i18next、npm / pnpm、Bun |
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
cd web
pnpm install
pnpm dev
```

Vite 开发服务器会将 `/api/v1` 代理到 `http://localhost:8080`。

## Luna CLI

Luna CLI 可在终端中管理 Luna DevOps，支持人类可读输出和面向自动化的 JSON 输出：

```bash
npm install --global @liteyuki/luna-cli
luna login
luna project get-projects
```

- [CLI 使用文档](https://luna-devops.liteyuki.org/download/installation)
- [CLI GitHub 仓库](https://github.com/LiteyukiStudio/luna-cli)
- [配套 Agent Skill](https://github.com/LiteyukiStudio/luna-cli/tree/main/skills/luna-devops)

## 部署

Luna DevOps 支持容器、Helm 和本地二进制部署。实际使用环境推荐采用容器化部署。

| 方式 | 适用场景 | 入口 |
| --- | --- | --- |
| Kubernetes / Helm | 生产级 Kubernetes 或 K3s 集群 | [`charts/luna-devops`](charts/luna-devops) |
| Docker Compose | 单机试用、小型实验室和发版验证 | [`docker-compose.yaml`](docker-compose.yaml) |
| 二进制 | 本地调试和源码开发 | [`cmd/api`](cmd/api)、[`cmd/worker`](cmd/worker) |

DockerHub 发布镜像统一为 `liteyukistudio/luna-devops`、`liteyukistudio/luna-worker` 和 `liteyukistudio/luna-agent`。

使用 Docker Compose 启动已发布的容器镜像：

```bash
cp .env.example .env
# 首次启动前请填写 SECRET_ENCRYPTION_KEY、BOOTSTRAP_TOKEN 和 REDIS_PASSWORD。
docker compose up -d
```

AI 助手默认关闭。为 API 与 Agent 配置同一个稳定的 `AI_INTERNAL_SECRET` 后，显式启用 AI profile：

```bash
AI_ASSISTANT_AVAILABLE=true docker compose --profile ai up -d
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

- [Kubernetes / Helm](docs/docs/zh/start/install/kubernetes.md)
- [Docker Compose](docs/docs/zh/start/install/docker-compose.md)
- [二进制部署](docs/docs/zh/start/install/binary.md)
- [配置参考](docs/docs/zh/reference/configuration.md)

## 配置说明

- `APP_ENV=development` 会启用本地开发便利功能。
- `APP_ENV=production` 会关闭开发默认值，并要求初始化管理员。
- 生产环境中的 `SECRET_ENCRYPTION_KEY` 必须保持稳定。它用于保护已保存的 Token、镜像站凭据、OAuth Secret 和其他敏感数据。
- Luna DevOps 位于反向代理之后时，`TRUSTED_PROXY_CIDRS` 应包含可信反向代理或 CDN 的出口网段。
- Worker 的构建网络可以单独配置。构建需要访问私有镜像站或镜像源时，建议使用受限出口并显式配置白名单。

完整的 API 与 Worker 配置请查看[配置参考](docs/docs/zh/reference/configuration.md)。

## 仓库结构

```text
cmd/api                 API 服务入口
cmd/worker              异步 Worker 入口
luna-agent/             独立 AI Agent、编排图、工具目录和持久运行时
internal/               后端业务域、Provider、Service 和数据模型
migrations/             PostgreSQL 数据库迁移
openapi/                OpenAPI 定义
web/                    Vite + React 控制台
web/public/             公共资源、标志、吉祥物和 favicon
docs/                   Rspress 文档站
docs-internal/                  内部开发文档（长期规范与方案记录）
charts/luna-devops      Helm Chart
```

本地可选的 `/cli/` 目录已被 Git 忽略，仅用于克隆独立 CLI 仓库进行联调。

## 开发

常用检查命令：

```bash
go test ./...
pnpm --dir web lint
pnpm --dir web build
```

项目约定：

- 前端依赖统一使用 `pnpm`。
- `web/`、`docs/`、`tests/` 和 `luna-agent/` 分别维护自己的依赖清单与 lockfile，不使用跨目录的根 pnpm workspace；需要 pnpm 项目配置时也只放在对应工作目录。
- Python 工具链统一使用 `uv`。
- 后端 Handler 保持精简，业务逻辑放入 Service，外部平台集成放入 Provider。
- 新功能必须按 [`docs-internal/14-可观测插桩与验收标准.md`](docs-internal/14-可观测插桩与验收标准.md) 补齐 Trace、关键结构化日志和低基数 Metrics，并保持跨服务 Context 连续。
- 所有用户可见的前端文案都放入 i18n 文件。
- 功能或行为变化时同步更新文档站。

## 品牌资源

- 标志 / favicon：[`web/public/luna-devops-logo.svg`](web/public/luna-devops-logo.svg)
- 吉祥物：[`web/public/brand/mascot-luna-devops.png`](web/public/brand/mascot-luna-devops.png)

## 文档

- 在线文档：[luna-devops.liteyuki.org](https://luna-devops.liteyuki.org/)
- 产品方案：[`docs-internal/01-产品与一体化方案.md`](docs-internal/01-产品与一体化方案.md)
- 内部开发文档索引：[`docs-internal/README.md`](docs-internal/README.md)
- 代码健康检查 SOP：[`docs-internal/07-代码健康检查SOP.md`](docs-internal/07-代码健康检查SOP.md)
- 开发计划：[`TODO.md`](TODO.md)
- AI 代理规范：[`AGENTS.md`](AGENTS.md)
- 贡献指南：[`CONTRIBUTING.md`](CONTRIBUTING.md)

## 许可证

Luna DevOps 采用 [MIT License](LICENSE) 开源。你可以使用、复制、修改、合并、发布和分发本项目，但在副本或项目的主要部分中需要保留原始版权与许可声明。

项目按“原样”提供，不附带任何明示或暗示的担保。第三方依赖、外部服务和第三方品牌资源仍分别受其自身条款约束；MIT License 不授予 Luna DevOps 或 Liteyuki Studio 名称与标志的商标使用权。详细说明见[许可说明](docs/docs/zh/reference/license.md)。
