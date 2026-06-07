# AGENTS.md

本文件是给 AI 编码代理的项目级开发规范。保持简短、可执行、少歧义；细节优先从 `docs/` 和现有代码中渐进读取。

## 0. 开工前必读

按需阅读，但开始实现前至少确认这些文件：

1. `README.md`
2. `TODO.md`
3. `docs/01-产品与一体化方案.md`
4. `docs/03-产品原型.html`
5. `docs/04-AI能力提案.md`

## 1. Hard MUST

- 先读现有代码和文档，再修改。
- 不主动执行 `git commit`、`git push`、创建/切换分支等 Git 操作，除非用户明确要求。
- 对一次可完成的小任务采用“一个目标一轮推进”的节奏：每完成一个可独立验收的事项（如一次功能点、文档修订、定位与修复闭环），要形成可追溯记录并与该事项绑定。
- 每次需求新增或变更，必须同步更新 `docs/` 下相关产品文档；影响计划、验收或状态时也必须更新 `TODO.md`。
- 完成实现后按改动规模选择验证：小功能改动只做针对性检查（相关 Go 包测试、TypeScript 类型检查或局部 smoke），不强制全量 lint/build/浏览器验收。
- 当一次改动满足任一条件时，必须执行完整验证并优先用浏览器验收前端交互：修改文件数超过 8 个、同时跨 3 个及以上业务域、涉及认证/权限/Secret/SSRF/数据库迁移/构建部署运行时、或用户明确要求验收。验收通过后再把 `TODO.md` 对应项标记完成。
- **MUST i18n**：前端任何用户可见文本常量必须走 `i18next/react-i18next`，不可硬编码。包括标题、描述、按钮、菜单、表单 label、hint、placeholder、toast、错误/空状态、确认弹窗、aria-label、schema 校验文案和状态 badge。产品名、文件名、API enum 原始值、URL/slug 示例可以保留为数据或示例；只要作为 UI 文案展示，就必须用 i18n label。
- **MUST 后端适配外部平台**：涉及 GitHub、Gitea、GitLab、Harbor、DockerHub、OIDC、Kubernetes、Traefik、AI Provider 等第三方/外部平台的读取、探测、搜索、状态同步和写操作，必须由后端 provider/service/API 适配、聚合或反代。前端只调用平台后端 API，不允许在前端编排第三方平台 API、暴露底层外部平台能力，或用多个底层代理接口拼出业务流程。
- Secret、Token、Registry Credential 不允许明文落业务表；密钥类字段不回显给前端。

## 2. 技术栈

后端：

- Go + Gin + GORM
- PostgreSQL，不使用 SQLite
- Redis + Asynq
- golang-migrate
- Kubernetes/client-go
- OpenAPI

前端：

- Vite + React + TypeScript
- Tailwind CSS + shadcn/ui
- TanStack Query + React Router
- React Hook Form + Zod
- i18next + react-i18next
- Sonner toast
- @antfu/eslint-config
- 包管理器必须使用 pnpm

Python：

- 必须使用 uv，不直接用 pip 管理项目依赖。

## 3. 目录边界

- 仓库是 monorepo。
- Go 后端在仓库根目录。
- 前端在 `web/`。
- 本地开发依赖放 `docker-compose-dev.yaml`，只包含开发需要的 PostgreSQL、Redis 等组件。
- `.env.*` 不提交；`.env.example` 可提交。
- 后端配置默认读环境变量，也支持 `ENV_FILE=.env.local go run ./cmd/api` 加载本地 `.env.*`。

推荐模块：

```text
cmd/api
cmd/worker
internal/auth
internal/project
internal/application
internal/repository
internal/registry
internal/build
internal/cluster
internal/deployment
internal/gateway
internal/config
internal/secret
web/src/pages
web/src/components/ui
web/src/components/common
web/src/i18n
```

## 4. 后端准则

- 第一阶段采用模块化单体 + 多进程部署。
- `cmd/api` 负责 HTTP API、Webhook、OAuth 回调、CRUD、权限校验和任务投递。
- `cmd/worker` 负责构建、部署、状态同步、证书申请、资源清理等异步任务。
- 长耗时任务进入 worker，不在 HTTP 请求里同步执行。
- Handler 只做参数解析、权限入口和响应；业务逻辑放 service；数据访问放 repository；外部系统调用放 provider。
- 构建/部署阶段的用户配置字符串默认允许使用 GitHub Actions 风格变量；最终执行前必须通过后端统一变量渲染组件处理，禁止在各业务里手写零散替换逻辑。
- 权限由后端最终判断，前端隐藏按钮只做体验优化。
- 危险操作必须写 AuditLog。

## 5. 前端准则

- 页面按 `web/src/pages/<module>` 组织。
- `web/src` 下共享模块必须使用 `@/` 根目录导入；公共组件、API、app context、layout、lib、i18n 和跨页面引用都必须用 `@/`。相对导入只用于当前页面/组件目录内的私有文件。
- **MUST shadcn/ui**：前端基础 UI 必须优先使用 shadcn/ui。凡 shadcn/ui 已提供的基础组件、布局组件、表单组件、反馈组件、表格/分页组件，不允许手写同类轮子；只能在业务组合层做薄封装。
- shadcn/ui 基础组件放 `web/src/components/ui`，组件清单见 `web/SHADCN_COMPONENTS.md`。
- 两个及以上页面稳定复用的业务组件必须抽到 `web/src/components/common` 或更合适共享目录。
- 表单统一使用 React Hook Form + Zod。
- 必填项使用主题色 `*`，不可用红色强警告风格。
- 未满足要求前提交按钮保持 disabled/弱化；字段错误在对应字段附近展示。
- 复杂字段必须提供可 hover/focus 的说明图标。
- 能搜索/选择的资源不要让用户手填。
- 密钥字段允许前端填写，但编辑时不展示原值；留空表示不修改。
- 列表类数据必须优先使用统一列表组件；管理台列表默认用表格/行列表并向上对齐，不用等宽卡片流冒充列表。
- 管理台资源列表默认复用构建页的 `DataList` 视觉和交互：固定表头、行内垂直居中、明确操作按钮、底部分页栏；不要为相同列表场景自造表格样式。
- 列表中的编辑、删除、测试、绑定等操作必须使用明确按钮或菜单入口；不要把整行或整张展示卡片做成编辑入口，避免误触和语义混乱。
- 涉及状态的展示必须使用有语义颜色的 `StatusValueBadge` 或带 `tone` 的 `StatusBadge`，包括集群健康状态、镜像站/外部连接健康状态、构建/部署/网关任务状态、Webhook/DNS/证书/扫描状态、启用/禁用和校验状态；不要在列表、详情或卡片中直接显示纯文本状态。
- **MUST 列表 API**：任何返回列表/批量对象的接口，只要未来数据量可能超过 100，就必须支持分页和排序参数，返回 `items/page/pageSize/sortBy/sortOrder/total/totalPages`。排序字段必须做后端白名单映射，排序方向只允许 `asc` 或 `desc`。OIDC Provider、少量系统配置定义等明确不太可能超过 10 条的小规模配置列表可以例外。
- 错误页面必须用户友好，并复用 `ErrorState`、`AuthErrorPage`、`ForbiddenPage` 等公共组件。
- 主题必须支持 light、dark、system 三态，并监听系统主题变化。
- 前端展示“Project”时统一称为“项目空间”，强调集合概念。

## 6. 集成与安全边界

- 平台构建主路径是平台 Builder + BuildKit rootless；GitHub/Gitea 只作为代码源、Webhook 和授权来源。
- 部署由平台执行并记录。
- 构建 Job 不挂载宿主机 Docker socket，不默认 privileged。
- 构建网络默认 restricted egress。
- 默认禁止访问元数据地址、Kubernetes API Server、Service CIDR、私有网段非 443 端口。
- 内网 registry/镜像源可通过白名单或私有网段 TCP 443 放行。
- Webhook 必须校验签名，只接受已绑定仓库事件。
- OIDC Provider 在平台后台配置，不通过环境变量 bootstrap。
- 内部平台不开放自由注册；本地账号由管理员创建、邀请或导入。

## 7. 常用命令

```bash
# dev deps
docker compose -f docker-compose-dev.yaml up -d

# backend
go test ./...
go run ./cmd/api
ENV_FILE=.env.local go run ./cmd/api

# frontend
pnpm --dir web install
pnpm --dir web dev
pnpm --dir web lint
pnpm --dir web build

# python
uv sync
uv add <package>
uv run <script>
```

## 8. Git 提交消息

- 不主动提交；只有用户明确要求 `git commit` 时才应用本节。
- 提交消息必须使用 gitmoji，格式为：`<type> <gitmoji>: <summary>`。
- `type` 使用常见 Conventional Commits 类型：`feat`、`fix`、`docs`、`style`、`refactor`、`perf`、`test`、`build`、`ci`、`chore`、`revert`。
- `summary` 使用简短中文或英文，说明本次提交的用户可见变化或工程变化；不加句号。
- 示例：`feat ✨: 新增项目空间管理页面`、`fix 🐛: 修复 Access Token 分页错位`。

常用 gitmoji：

- `✨` feat：新增功能
- `🐛` fix：修复缺陷
- `📝` docs：文档变更
- `🎨` style：代码风格、UI 细节或格式调整
- `♻️` refactor：重构且不改变行为
- `⚡️` perf：性能优化
- `✅` test：新增或修复测试
- `🚀` ci/build/release：部署、发布或流水线相关
- `🔧` chore：配置、脚手架、工具链调整
- `🔒️` security：安全加固
- `🌐` i18n：国际化文案
- `💄` ui：视觉样式或交互 polish
- `🗃️` db：数据库 schema 或迁移
- `🔥` remove：移除代码或功能
- `🚨` lint：修复 lint 或类型检查问题
- `⏪️` revert：回滚变更

## 9. 不要做

- 不擅自引入未讨论的新框架，可以推荐给人类。
- 不为 MVP 预先实现完整计费、持久构建缓存、Service Mesh。
- 不把 Gitea/GitHub Actions 作为唯一构建路径。
- 不在 handler 中散落 GORM 查询。
- 不直接展示后端原始异常、OIDC 原始错误或技术堆栈给用户。
- 不提交本地环境文件、构建产物、依赖目录或临时日志。
