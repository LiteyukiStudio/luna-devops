# Builder 构建验收复盘

本文记录 2026-06-07 使用平台 Builder 从前端触发构建 `snowykami/neo-blog` 的 `web` 目录，并推送到 DockerHub `snowykami/neo-blog-front-test:latest` 的完整复盘。

## 验收目标

- 通过 Chrome 前端页面完成构建触发，不直接绕过平台业务接口。
- 使用已登录的 GitHub 凭据读取 `snowykami/neo-blog`。
- 使用已配置的 DockerHub 凭据推送 `snowykami/neo-blog-front-test:latest`。
- 验证平台 Builder 可以完成 clone、checkout、BuildKit 构建、实时日志上传、状态回写和镜像推送。

## 最终结果

最终构建运行成功：

- BuildRun: `bldr_e447f56631289e706ba32c1b`
- BuildJob: `bldj_59cdbf36908a5f1851093c3a`
- Source commit: `a1517025d580e175629f9cd6768e90f35b89464a`
- Image: `docker.io/snowykami/neo-blog-front-test:latest`
- Digest: `sha256:24c2abf166fa25652fad977f603b0e5b2fbcfda9cf88bec5a5b684e7a2ca339b`

外部校验命令：

```bash
docker buildx imagetools inspect docker.io/snowykami/neo-blog-front-test:latest
```

## 本次遇到的问题

### 1. 前端触发构建时缺少目标镜像站

早期构建触发表单没有显式选择目标镜像站，后端要求 `targetRegistryId` 和 `targetRepository`，导致构建请求不完整。

同时，选择应用后 Dockerfile 和构建上下文没有自动继承应用默认配置，仍停留在 `Dockerfile` 和 `.`。

### 2. 实时日志上传误判 204

Builder 向日志追加接口写入日志时，后端返回 `204 No Content` 表示成功；Builder client 原先把所有 204 都当成“没有任务”，导致日志上传报 `no builder task`。

### 3. GitHub clone 网络 EOF

第一次真实构建在 clone 阶段遇到 GitHub TLS / RPC EOF：

```text
fatal: early EOF
fatal: fetch-pack: invalid index-pack output
```

这是外部网络抖动，但 Builder 原先没有浅克隆和重试，失败概率较高。

### 4. 嵌套 BuildKit 权限不足

Docker executor 启动 rootless BuildKit 后，构建阶段执行 `RUN` 命令时报 `/proc` 挂载权限不足：

```text
runc run failed: error mounting "proc" to rootfs at "/proc": operation not permitted
```

本地 Docker executor 场景下，BuildKit 容器需要更明确的隔离构建权限。

### 5. DockerHub 镜像引用和认证 host 不一致

平台最初将 DockerHub endpoint `https://registry-1.docker.io` 直接拼进镜像引用：

```text
registry-1.docker.io/snowykami/neo-blog-front-test:latest
```

这会导致 DockerHub token scope 和镜像命名不匹配，push 报：

```text
insufficient_scope: authorization failed
```

DockerHub 镜像引用应使用 `docker.io/...`，auth config key 应使用 DockerHub 兼容地址。

### 6. npm 依赖下载极慢

`neo-blog/web/Dockerfile` 在 `pnpm install --frozen-lockfile` 阶段下载 768 个包。首次构建没有缓存，且 npmjs 网络多次低于 50 KiB/s，单次安装接近 10 到 12 分钟。

这不是平台逻辑错误，但会严重影响 Builder 首次构建体验。

### 7. DockerHub 基础镜像 metadata EOF

构建过程中拉取 `node:24-alpine` metadata 时出现 DockerHub EOF：

```text
failed to resolve source metadata for docker.io/library/node:24-alpine
```

这是基础镜像站网络问题。没有重试时会直接失败。

### 8. 前端构建列表状态不实时

BuildRun 已经变为 `failed` 或 `succeeded` 后，前端列表仍可能显示 `queued`。页面目前依赖用户刷新或重新请求，缺少实时轮询、SSE 或 WebSocket。

## 已修复内容

### 前端构建触发

- 构建触发表单增加目标镜像站选择。
- 打开触发构建 Dialog 时自动选择可用的默认镜像站。
- 构建触发 Dialog 只读展示应用的 Dockerfile 路径和构建上下文；实际 BuildRun 由后端强制继承应用配置，修改入口统一放在应用配置页。
- API client 错误处理优先展示后端 `detail`，方便开发定位。

### Builder 执行链路

- 日志上传 204 成功响应不再被误判为无任务。
- Git clone 改为浅克隆，并增加最多 3 次重试。
- BuildKit 构建命令增加最多 3 次整体重试。
- 本地 Docker executor 启动 BuildKit 容器时增加 `--privileged`，解决嵌套构建 `/proc` 挂载失败。

### DockerHub 兼容

- DockerHub 镜像引用从 `registry-1.docker.io/...` 修正为 `docker.io/...`。
- Docker auth config 对 DockerHub 使用兼容 key `https://index.docker.io/v1/`。
- 最终成功推送 `docker.io/snowykami/neo-blog-front-test:latest`。

### 构建加速配置入口

- Builder 增加 `BUILDER_NPM_REGISTRY` 配置。
- Docker Compose 开发环境默认设置为 `https://registry.npmmirror.com`。
- Builder 会尝试在构建上下文写入 `.npmrc`，作为初步加速入口。

注意：这只是临时能力。对于选择性 `COPY package.json pnpm-lock.yaml ./` 的 Dockerfile，构建上下文中的 `.npmrc` 不一定会被复制进安装层，因此不能作为长期唯一方案。

### 本轮闭环优化

- 新增 BuildVariableSet 构建变量集模型，支持 `global`、`project`、`user` 三种作用域。
- BuildRun 支持保存本次构建选择的变量集 ID，Builder 领取任务时按权限解析变量。
- Builder executor 会把变量注入一次性构建容器，并作为 BuildKit `build-arg` 传入。
- `BUILDER_NPM_REGISTRY` 改为默认变量来源之一，可被构建变量集覆盖。
- 前端构建页新增“构建变量集”子 tab，支持创建、编辑、删除和在触发构建时多选。
- 构建运行和构建任务列表增加自动刷新，构建任务支持打开日志 Dialog 查看实时日志。
- 构建运行列表的目标镜像展示优先使用完整 `imageRef`，避免 DockerHub host 显示不完整。

## 待优化点

### 1. Builder 基础镜像必须支持本地/内网镜像站

BuildKit 这类构建基础镜像会高频使用，必须支持用户自定义 builder executor image。

建议：

- 平台提供推荐默认值：`moby/buildkit:v0.24.0-rootless` 或平台自建 BuildKit 镜像。
- 支持配置内网镜像站版本，例如 `registry.internal/buildkit:v0.24.0-rootless`。
- 支持每个 Builder Agent 上报自身 executor image、版本和能力标签。
- 允许管理员在平台配置多个 Builder Profile：默认、内网加速、高权限、低权限、ARM、AMD64。

### 2. 基础镜像拉取要支持 mirror 配置

仅替换 BuildKit 镜像不够。用户 Dockerfile 中的 `FROM node:24-alpine`、`FROM golang:1.25` 仍然要访问 DockerHub。

建议：

- BuildKit daemon 支持 registry mirror 配置。
- Builder Profile 支持配置 DockerHub、GHCR、Quay、GCR 等 registry mirrors。
- 对 DockerHub 官方镜像可配置内网 pull-through cache。
- 允许项目或组织级覆盖镜像源，但默认使用平台全局镜像源。

### 3. 语言依赖镜像源按工具链注入

构建加速不能只靠 `.npmrc`，需要按语言和工具链提供标准注入方式，并允许用户选择是否启用。

建议支持：

| 生态 | 常见配置 | 注入方式 |
| --- | --- | --- |
| Node.js npm | `npm_config_registry`、`.npmrc` | 环境变量、secret/文件注入、构建参数 |
| pnpm | `npm_config_registry`、`.npmrc` | 环境变量、workspace `.npmrc`、BuildKit secret |
| Yarn | `YARN_NPM_REGISTRY_SERVER`、`.yarnrc.yml` | 环境变量、配置文件 |
| Python pip | `PIP_INDEX_URL`、`PIP_EXTRA_INDEX_URL` | 环境变量、`pip.conf` |
| Poetry | `POETRY_REPOSITORIES_*` | 环境变量、配置文件 |
| Go | `GOPROXY`、`GONOSUMDB`、`GOPRIVATE` | 环境变量 |
| Maven | `settings.xml` | BuildKit secret 文件 |
| Gradle | `init.gradle`、`gradle.properties` | 文件注入 |
| Cargo | `.cargo/config.toml` | 文件注入 |
| RubyGems | `BUNDLE_MIRROR__...` | 环境变量 |
| Docker/OCI | registry mirror | BuildKit daemon 配置 |
| GHCR | registry mirror / 凭据 | Docker auth config |

实现原则：

- 全局默认启用常用镜像源，项目可覆盖，构建任务可临时关闭。
- 环境变量注入适合 npm、pip、Go 等工具链。
- 文件注入适合 Maven、Gradle、Cargo、复杂 npmrc。
- 凭据必须通过 BuildKit secret 或一次性文件注入，不能写入最终镜像层。
- 用户 Dockerfile 不应被平台强行改写。

当前已完成 MVP：使用 BuildVariableSet 保存工具链环境变量，并在构建时注入容器环境和 BuildKit build-arg。后续仍需补齐按生态自动生成配置文件、BuildKit secret 文件注入和凭据型镜像源注入。

### 4. 构建缓存必须进入后续计划

本次 npm install 耗时说明 MVP 即使能跑通，也会因为无缓存体验很差。

建议分层做：

- 第一阶段：BuildKit local cache，挂载到 Builder Agent 本地目录。
- 第二阶段：registry cache，推送到平台镜像站或项目镜像站。
- 第三阶段：远程 buildkitd pool，共享缓存和并发构建能力。
- 第四阶段：按项目、应用、分支和 Dockerfile hash 生成 cache key。

### 5. 构建状态和日志展示需要产品化

目前前端只能看 BuildRun / BuildJob 列表，缺少详情页。

需要补齐：

- BuildRun 详情页。
- BuildJob 实时日志。
- 状态自动刷新。
- 失败原因摘要。
- 重试、取消、重新构建按钮。
- 成功镜像 digest、source commit、耗时、开始/结束时间。

当前已完成基础闭环：列表自动刷新和 BuildJob 日志 Dialog。独立详情页、取消、重试和失败摘要仍放在后续产品化阶段。

### 6. 任务取消和超时必须落地

本次测试中有长时间卡住的构建任务，只能人工停止临时 BuildKit 容器。

需要补齐：

- BuildRun cancel API。
- Builder Agent watch/cancel。
- executor 容器按 job id 命名，便于终止和清理。
- 任务超时后自动 fail。
- 重试策略应由平台控制，而不是无限运行。

### 7. 权限模型要区分本地开发和生产隔离

本地 Docker executor 为了跑通 BuildKit 使用了 `--privileged`。这适合开发环境，但生产环境要拆成安全档位。

建议：

- 开发档：Docker executor + privileged BuildKit，便于本地验证。
- 生产档：Kubernetes 或专用 worker 节点，受控网络和资源限制。
- 高安全档：rootless BuildKit + 网络白名单 + 无宿主 Docker socket。
- 高性能档：远程 buildkitd pool + 共享缓存。

## 后续推荐设计

### Builder Profile

新增 Builder Profile，用于把构建环境能力显式建模：

- 名称和描述。
- executor 类型：docker、remote-buildkit、kubernetes、podman。
- executor image：可使用公网或内网镜像。
- registry mirrors。
- language mirrors。
- 是否注入环境变量。
- 是否注入配置文件。
- 是否允许 privileged。
- CPU、内存、超时和并发限制。
- 适用项目或租户范围。

### Mirror Policy

新增 Mirror Policy，用于统一管理构建加速源：

- 全局默认策略。
- 项目覆盖策略。
- 用户临时构建覆盖。
- DockerHub、GHCR、npm、PyPI、Go proxy 等分类配置。
- 可选启用，不强制所有构建使用。
- 支持白名单和凭据。

### 构建执行流程补充

推荐流程：

1. API 创建 BuildRun / BuildJob。
2. Builder Agent 领取任务。
3. Builder Agent 加载 Builder Profile 和 Mirror Policy。
4. Builder Agent 准备一次性工作目录。
5. Builder Agent 写入 Git 凭据、Registry 凭据、语言工具链配置和 BuildKit 配置。
6. executor 容器 clone 仓库并 checkout。
7. executor 使用 BuildKit 构建并推送镜像。
8. Builder Agent 实时上传日志。
9. Builder Agent 回写 source commit、image ref、digest、耗时和状态。
10. 平台创建 ContainerImage 记录，供部署选择。

## 本次结论

平台 Builder 主链路已经可以跑通：前端触发、队列领取、源码拉取、BuildKit 构建、实时日志、状态回写、DockerHub 推送均已验证。

但要让它成为可靠产品，还需要继续补齐：

- 构建镜像源和语言依赖镜像源策略的产品化管理。
- BuildKit / DockerHub / GHCR 等 registry mirror。
- 构建缓存。
- 构建详情、取消、重试和失败摘要 UI。
- 任务取消、超时和重试策略。
- Builder Profile 管理。
