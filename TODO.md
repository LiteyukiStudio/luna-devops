# TODO

## 1. 文档与原型收口

- [x] 更新产品原型为文档式多页面线框。
- [x] 在原型中覆盖创建应用、构建、部署、镜像站、自定义域名、Access Token、配额页面。
- [x] 检查并移除文档中旧的 Actions / Kubernetes Job 构建路径表述，构建只保留平台 Builder。
- [x] 新增 Builder 真实构建验收复盘文档，记录问题、修复项、待优化项和构建镜像源方案。

## 2. 项目基础与前后端脚手架

- [x] 初始化 Go 服务目录。
- [x] 初始化 `cmd/api` 入口。
- [x] 初始化 `cmd/worker` 入口。
- [x] 接入 Gin。
- [x] 接入 PostgreSQL。
- [x] 接入 GORM。
- [x] 接入 golang-migrate。
- [x] 接入 Redis + Asynq。
- [x] 定义 API 与 Worker 的任务投递和状态回写约定。
- [x] 建立异步任务基础队列。
- [x] 定义 OpenAPI 基础结构。
- [x] 初始化 Vite + React + TypeScript。
- [x] 接入 Tailwind CSS。
- [x] 接入 shadcn/ui。
- [x] 接入 @antfu/eslint-config。
- [x] 统一 `web/src` 导入规则：共享模块使用 `@/` 根目录导入，ESLint 禁止跨目录相对导入共享模块。
- [x] 接入 i18next。
- [x] 抽离 `SessionProvider`，统一管理当前用户、登录、初始化、登出和语言更新。
- [x] 开发模式新增 Debug 悬浮窗，支持拖动记忆位置和前端角色视图切换，不再支持真实用户会话切换。
- [x] 将前端可见文本 i18n 规则提升为 MUST，并清理当前主要页面硬编码文案。
- [x] 复扫 `web/src/**/*.tsx`，清理内容区、placeholder、aria-label 和 Dialog 默认文本中的硬编码文案。
- [x] 增加 MUST 准则：潜在超过 100 条的列表 API 必须支持分页、排序字段和排序方向。
- [x] 增加 MUST 准则：第三方/外部平台能力必须由后端适配、聚合或反代，前端不得编排外部平台 API。
- [x] 增加列表交互准则：编辑、删除、测试、绑定等操作必须使用明确按钮或菜单入口，禁止整行隐式编辑。
- [x] 增加 MUST 准则：前端基础组件优先使用 shadcn/ui，有现成组件时禁止自造轮子。
- [x] 在 `web/SHADCN_COMPONENTS.md` 维护 shadcn/ui 官方组件清单和替换优先级。
- [x] 接入 Sonner toast。
- [x] 为自动关闭 toast 增加倒计时进度条。
- [x] 实现 light/dark/system 主题三态。
- [x] 将控制台默认主色调整为 Kubernetes 风格蓝，并同步品牌与原型文档。
- [x] 建立前端基础布局、路由和 API client。
- [x] 将用户信息和主题切换控件移动到侧边栏底部。
- [x] 引入前端轻量动效，覆盖页面切换、列表、弹窗和基础控件。
- [x] 将侧边栏导航改为二级分组结构，按 DevOps、个人工作区、系统管理分栏展示。
- [x] 将内容区顶部改为当前页面标题和说明，正文区域不再重复渲染页面标题。
- [x] 项目空间和应用详情页 topbar 使用资源类型前缀展示当前资源名称，并在应用列表提供进入应用详情的唯一入口。
- [x] 浏览器标签页标题统一为 `{page title} - {site title}`，其中 page title 复用内容区 topbar 标题。
- [x] 新增 shadcn Dialog 基础组件和可配置二次确认组件。
- [x] 新增内容区级 `ContentTabs` 组件，统一子 tab、右侧工具按钮布局和 hash 保持状态。
- [x] 统一前端状态展示：集群、构建、部署、网关、Webhook、扫描、启停和校验状态均使用有语义颜色的状态 Badge，并写入 AI 准则。
- [x] DevOps 导航新增“代码库”页，在镜像站上方集中管理 Git Provider 和 Git 凭据。
- [x] Git Provider 表单增加 GitHub/Gitea OAuth App 创建指引，Git 凭据仅在 OAuth App 完整配置后展示跳转授权入口。
- [x] Git Provider 类型选择和列表展示增加平台图标，优先使用实例 favicon，失败回退内置 GitHub/Gitea/GitLab 图标。
- [x] 项目仓库绑定页移除 Git Provider/Git 凭据创建入口和 secret ref 输入，只选择已有 Git 凭据绑定仓库。
- [x] Git Provider/Git 凭据 API 响应不再回显 secret ref，只返回 secret 是否已设置。
- [x] 抽离通用 `SegmentedControl`，主题切换和内容区横向单选 tab 共用同一套交互样式。
- [x] 主题切换改为纯图标 SegmentedControl，避免侧边栏窄宽度下文本省略。
- [x] 个人工作区导航改为“账号”，账号页用 `ContentTabs` 拆为个人资料、安全设置、个人令牌。
- [x] 将语言设置从布局底栏移入账号页个人资料表单。
- [x] 项目空间侧边栏支持展开、最多展示 10 个项目，并支持用户级固定项目。
- [x] 项目空间侧边栏子项目列表增加层级缩进，进入工作台后 topbar 显示项目名称。
- [x] 项目空间侧边栏展开/收起增加动画，图钉内嵌到项目条目右侧。
- [x] 侧边栏导航横条和项目条目统一为完全圆角风格。
- [x] 项目空间工作台用 `ContentTabs` 集中概览、应用、成员内容，仓库绑定改为挂靠到应用下面。
- [x] 项目空间工作台的新增应用、添加成员等 tab 主操作统一放入 `ContentTabs.tools`。
- [x] 应用行新增仓库绑定入口，支持用 Git 凭据搜索可见仓库并自动回填。
- [x] 应用编辑仓库来源移除 owner/repo/cloneUrl 自由输入，Dockerfile 与构建上下文改为基于仓库目录探测的候选选择。
- [x] 应用保存当前 `gitAccountId`，编辑时优先从 RepositoryBinding 恢复，绑定缺失时回退应用字段，避免重复选择 Git 凭据。
- [x] 将 Dockerfile/构建目录探测迁到后端 build-options 接口，优先使用 recursive tree API，前端不再逐目录串行探测。
- [x] Dockerfile 和构建上下文改为可输入候选建议，探测结果只作为建议，不阻止用户手动修正。
- [x] 新增统一搜索选择器，并将 Git 分支选择改为 search/limit 查询、短缓存筛选和前端最大展示。
- [x] 修复 Dialog 内搜索下拉撑高弹窗的问题，并将 Git 分支缓存改为分页拉取后再搜索。
- [x] 为 `web/src/components/common` 公共组件补充用途、适用场景和边界注释，方便后续 AI 复用。
- [x] 项目详情页之间切换时复用同一个页面动画容器，只让项目信息和 tab 内容轻量切换。
- [x] 全部项目列表只保留进入工作台入口，旧项目内列表路由重定向到工作台。
- [x] 第一批 CRUD 弹窗化：项目空间、应用、项目成员、用户、身份源、Access Token。
- [x] 将镜像站页面拆为镜像站、凭据、镜像、构建器四个子 tab，并将创建/编辑表单改为 Dialog。
- [x] 构建器作为镜像站页子资源管理，展示自动注册的 Builder Agent，并保留 global/project/user 三种作用域的 BuildProvider 配置项。
- [x] 镜像凭据 tab 默认展示全部镜像站凭据，并在凭据行展示所属镜像站名称。
- [x] 继续将仓库绑定等多表单页面改为创建/编辑 Dialog。
- [x] 使用浏览器验收前端启动、主题切换和基础路由。
- [x] 开发环境使用 Vite proxy 反代后端 API。
- [x] 为 api、worker、web 编写 Dockerfile。
- [x] 提供完整 docker compose 运行编排。
- [x] 拆分开发依赖和完整部署的 compose 边界：开发依赖使用独立项目名并暴露 PG/Redis，完整部署的 PG/Redis 仅走容器内网络，避免端口和容器项目名冲突。
- [x] 明确本地开发拓扑：前端、API、worker 优先在宿主机运行，PG/Redis 由 dev compose 提供；Builder 仅在真实构建联调或完整部署验收时启动。
- [x] 将 compose 场景收敛为两份：`docker-compose-dev.yaml` 启动 PG/Redis/worker/builder 用于开发联调，`docker-compose.yaml` 保留完整部署栈。
- [x] `docker-compose-dev.yaml` 的 worker / builder 读取 `.env.dev`，并通过服务级环境变量直接设置容器内连接地址。
- [x] `.env.example` / `.env.dev` 增加 scope 注释，区分宿主机进程、Builder、安全策略和前端公开入口变量；Compose 容器连接地址直接保留在 compose 文件内。

## 3. 认证、权限与登录

- [x] 实现本地账号登录。
- [x] 实现管理员创建、邀请或导入本地账号。
- [x] 接入通用 OIDC。
- [x] 支持 Casdoor OIDC 配置。
- [x] 实现 AuthProvider。
- [x] 实现 AuthAdmissionPolicy。
- [x] 支持后台配置多个 OIDC Provider。
- [x] 身份源页面拆为 OIDC Provider 和准入策略两个子 tab。
- [x] 实现 OIDC 外部身份绑定 ExternalIdentity。
- [x] 支持 OIDC 通过非空已验证邮箱绑定现有用户。
- [x] 支持登录态下绑定和解绑第三方登录。
- [x] 实现运行模式检测，开发模式支持开发账号快捷登录，生产模式禁用。
- [x] 收紧开发默认账号提示边界，仅 development 模式由后端下发并展示。
- [x] 登录页支持最近登录账号头像选择，浏览器本地最多持久化 3 个账号展示信息，不保存密码、Token 或 session cookie；直接恢复登录使用后端 HttpOnly remember cookie。
- [x] 受保护路由未登录直接跳转 `/login?redirect=...`，当前用户查询对未登录错误不重试，移除中间“需要登录”页面体验。
- [x] 封装统一用户头像组件，按平台头像、Gravatar 真实头像、字母头像顺序回退。
- [x] 实现生产模式首个平台管理员初始化流程。
- [x] 禁止开放自由注册。
- [x] 支持 OIDC 允许组白名单。
- [x] 支持可配置 OIDC group claim。
- [x] 支持邮箱域白名单和邀请邮箱白名单。
- [x] 支持 OIDC Client Secret 前端填写、后端加密保存、API 不回显。
- [x] 移除 OIDC Client Secret 引用输入，降低身份源配置复杂度。
- [x] 移除 Casdoor/OIDC 环境变量 bootstrap，身份源统一通过平台后台配置。
- [x] 开发模式打印 `ENV_FILE` 加载状态和文件路径，便于确认本地 `.env.*` 是否生效。
- [x] 开发模式未显式设置 `ENV_FILE` 时默认尝试读取 `.env.dev`。
- [x] 准入失败记录 AuditLog。
- [x] 实现统一 AuthErrorPage。
- [x] 实现统一 ForbiddenPage。
- [x] 实现 OIDC state 错误、组白名单不匹配、账号未邀请、权限不足的友好错误展示。
- [x] 建立 User、Project、ProjectMember。
- [x] 实现 Owner/Admin/Developer/Viewer 角色。
- [x] 实现权限点校验。
- [x] 实现 Access Token 创建、hash 存储和撤销。
- [x] 实现 Access Token scope 校验。
- [x] 收紧 Access Token scope：未知 API 默认拒绝，创建 scope 白名单化，普通用户只能创建读类 scope。
- [x] 增加 API CORS 白名单、Cookie 会话 Origin 防护和基础安全响应头。
- [x] 为本地登录和首个管理员初始化增加基础限流。
- [x] Access Token 列表隐藏已撤销 Token，时间列单行展示，有效期改为固定选项并支持 0 无限有效。
- [x] 实现登录页。
- [x] 实现当前用户和基础权限状态管理。
- [x] 实现用户语言偏好保存和前端 i18n 同步。
- [x] 实现项目成员权限状态管理。
- [x] 增加项目 owner 保护：admin 不能授予或修改 owner，禁止删除或降级最后一个 owner。
- [x] 为复杂表单字段补充 label 问询提示和统一校验交互。
- [x] 将用户列表改为统一列表组件展示，并接入后端分页查询。
- [x] 为用户列表 API 补充排序字段和排序方向参数。
- [x] 将 Access Token 管理合并到账号页，作为“个人令牌”子 tab。
- [x] 将 Access Token 列表改为统一列表组件展示，并接入后端分页查询。
- [x] 为 Access Token 列表 API 补充排序字段和排序方向参数。
- [x] 将统一列表分页替换为页码式分页控制器，支持页码、省略号、上一页和下一页。
- [x] 统一分页组件支持每页条数选择，统一列表滚动限制在表格区域内。
- [x] 项目空间列表接入分页与每页条数选择，侧边栏项目空间父入口默认进入列表并移除子级“全部项目”。
- [x] 侧边栏项目空间展开区限制为同时展示 6 个项目，其余项目在子列表内滚动。
- [x] 抽离统一分页组件，并将列表 API 改造为支持分页、排序、搜索和可选批量选择。
- [x] 使用浏览器验收本地登录、退出和 Access Token 创建/撤销流程。
- [x] 使用浏览器验收权限隐藏流程。

## 4. 项目、应用与前端主工作区

- [x] 实现 Project CRUD。
- [x] 修复 Project 软删除后 slug 唯一索引仍占用的问题，改为未删除项目唯一。
- [x] 明确并强化标识唯一约束：项目空间标识全局唯一，应用标识在同一项目空间内唯一，API 返回友好业务错误。
- [x] 实现 Project namespaceStrategy。
- [x] 实现 Application CRUD。
- [x] 支持 sourceType: repository。
- [x] 支持 sourceType: image。
- [x] 移除仓库声明配置文件的读取、解析和产品设计，后续如需此类能力再单独设计。
- [x] 实现项目页。
- [x] 前端展示命名从“项目”调整为“项目空间”，强化集合概念。
- [x] 增加表单准则：可搜索/可选择的资源优先选择，不让用户手填。
- [x] 将项目技术栈要求整合进 AGENTS.md，并删除独立 docs/02 文档。
- [x] 按 Skill 编写原则精简 AGENTS.md，保留核心 MUST 和渐进阅读入口。
- [x] 实现应用页。
- [x] 实现创建应用向导。
- [x] 实现应用配置页。
- [x] 将应用编辑从独立页面收回到应用列表 Dialog，与创建应用使用同一套弹窗表单。
- [x] 抽离可复用 PageHeader、EmptyState、ErrorState、StatusBadge。
- [x] 实现可复用 ConfirmDialog。
- [x] 实现公开站点配置 KV 读取接口。
- [x] 实现站点配置动态 KV 表单。
- [x] 支持自定义站点 title、logo、favicon、登录页副标题。
- [x] 修复站点设置保存结构化值时报 `cannot unmarshal object into string` 的兼容问题。
- [x] 使用浏览器验收站点设置保存、公开配置刷新和语言切换流程。
- [x] 使用浏览器验收项目页、应用页和 sourceType 切换流程。
- [x] 使用 PostgreSQL 集成环境验收项目创建和应用创建流程。

## 5. Git 集成

- [x] 实现 GitProvider 基础模型、迁移和 CRUD API。
- [x] 实现 GitAccount 基础模型、迁移和当前用户 CRUD API。
- [x] 支持 Gitea OAuth API。
- [x] 支持 GitHub OAuth API。
- [x] 实现 RepositoryBinding 基础模型、迁移和项目内 CRUD API。
- [x] 实现 GitProvider / GitAccount OAuth 回调和 token 刷新 API。
- [x] 统一 Git/OIDC OAuth state 运行时表名与迁移表名，并迁移历史 AutoMigrate 错误表数据。
- [x] Git Provider / Git 凭据表单作用域交互与镜像站一致：global/user 不展示具体项目，project 才选择所属项目。
- [x] 实现仓库列表、分支、文件读取和 Dockerfile/构建目录探测 API。
- [x] 创建 Git webhook API。
- [x] 校验 webhook 签名 API。
- [x] 处理 push/tag webhook 事件 API。
- [x] RepositoryBinding 列表排除软删除 GitProvider/GitAccount/Application，并展示 Git 凭据 owner 信息。
- [x] 删除 GitAccount 前检查 RepositoryBinding 引用，禁止删除仍被绑定引用的凭据。
- [x] Debug 角色预览状态下禁止触发 Git OAuth 授权，避免真实 session 归属混淆。
- [x] Git 上游接口错误对前端脱敏，不再透传上游响应体。
- [x] 收紧 Git 个人凭据访问：`personal` 凭据仅所有者可见可用，`provider` 凭据才按作用域共享。
- [x] 收紧普通业务列表中的用户空间资源：管理员不再混看他人的 user-scope Git Provider、镜像站、构建提供者和 personal Registry/Git 凭据，后续单独建设管理视图。
- [x] 全局 Git Provider、镜像站、集群对普通用户只返回可用资源摘要，不返回管理员维护的连接配置或密钥明文。
- [x] 将 Git OAuth 凭据自动刷新放入后期 Worker：扫描即将过期的 GitAccount，使用 refresh token 刷新 access token，失败时标记 expired 并记录审计事件。
- [x] 在 Git API 调用前增加兜底刷新：当 access token 已过期或即将过期时同步触发一次刷新，避免用户必须手动点击刷新。

## 5.1 异步任务与 Worker 后期增强

- [x] 以 Redis + Asynq 作为 Go 侧默认异步任务方案，承担类似 Celery 的队列、重试、延迟任务和定时任务能力。
- [x] 引入 Asynq Scheduler/PeriodicTaskManager 管理周期任务，包括 Git 凭据刷新、集群状态同步、证书续期检查、资源清理和失败任务补偿。

## 5.2 安全审计后续

- [x] 完成二次代码安全审计修复，覆盖后端权限、会话、OAuth、凭据、外部请求和前端调用链。
- [x] 收敛镜像站测试接口错误输出，避免向前端透出底层网络细节。
- [x] 收紧镜像记录创建权限：项目镜像记录要求项目写角色，未归属项目记录仅平台管理员可创建。
- [x] 抽离统一 SSRF/出站访问控制组件，并接入 Git、OIDC 和 Registry 外部请求链路。
- [x] SSRF/egress 组件接入管理员安全配置，支持域名黑名单、域名特许白名单、IP/CIDR 黑白名单和端口规则。
- [x] 封装统一错误响应层，开发模式返回调试细节，生产模式仅返回稳定错误码和业务化文案。
- [x] 持续补充业务错误码枚举和前端按错误码 i18n 展示。
- [x] 多实例部署时将登录限流从进程内存迁移到 Redis/Asynq 统一限流能力。
- [x] 为所有 Worker 任务定义统一 envelope：`taskId`、`taskType`、`dedupeKey`、`actorId`、`resourceRef`、`traceId`、`attempt`、`createdAt`。
- [x] 建立任务幂等规则：外部资源 apply、token 刷新、webhook 处理、构建触发都必须能按 `dedupeKey` 重放而不产生重复副作用。
- [x] 配置任务重试、超时、优先级队列和失败保留策略：构建/部署使用独立队列，Git/证书/同步使用轻量队列。
- [x] 增加任务状态表或事件表，记录 queued/running/succeeded/failed/canceled，供前端展示异步任务进度和失败原因。
- [x] 增加死信队列处理页面或后台命令，用于人工重放失败任务、忽略任务和查看失败上下文。
- [ ] 保留 Temporal 作为后期长流程备选：当部署流水线、人工审批、跨集群补偿和多日持久 workflow 复杂度升高时，再从 Asynq 迁移部分流程到 Temporal。

## 6. 镜像站

- [x] 实现 ArtifactRegistry。
- [x] 支持 global/project/user scope。
- [x] 实现 RegistryCredential 加密引用。
- [x] 明确 RegistryCredential 隔离：`scope` 表示 pull/push 用途，`accessScope` 表示 personal/跟随镜像站；global 镜像站凭据强制 personal。
- [x] Registry response 不再暴露默认 `credentialRef`，仅返回 `credentialSet`。
- [x] 项目级镜像站管理收紧为 Owner/Admin，Developer 不再能新增、修改或删除项目镜像站。
- [x] 实现 registry 凭据测试。
- [x] 修正镜像站测试接口：按使用权限开放，失败返回结构化结果，并支持 token-only Basic Auth 探测。
- [x] 实现默认镜像站选择优先级。
- [x] 镜像记录支持按镜像站搜索镜像仓库和读取 tag 建议，后端适配 DockerHub、Harbor 与通用 Registry，并加入限数、短缓存和用户级限流。
- [x] 实现 ContainerImage 记录。

## 7. 平台构建

### 7.1 构建 API/CRUD 优先

- [x] 实现 BuildProvider 模型、迁移和 CRUD API。
- [x] 实现 BuildRun 模型、迁移和列表/详情 API。
- [x] 实现 BuildJob 模型、迁移和列表/详情 API。
- [x] 实现手动触发构建 API，先创建 queued 状态的 BuildRun/BuildJob。
- [x] 实现构建触发器配置 API：manual、webhook、push branch、tag、API token。
- [x] 实现构建参数配置 API：Dockerfile 路径、构建上下文、目标镜像、目标镜像站凭据、构建目录。
- [x] 构建触发只允许用户填写目标镜像 Tag 模板；目标镜像名前缀由平台按镜像站和应用标识固定生成，DockerHub 不带域名前缀，其他镜像站强制带 registry domain；镜像站不再承载 repository namespace。
- [x] 构建触发表单按应用仓库绑定选择分支，Dockerfile 和构建上下文候选按本次构建分支探测；应用编辑仅保留默认分支和默认入口。
- [x] 新增 BuildVariableSet 构建变量集模型、迁移和 CRUD API，支持 global/project/user 作用域。
- [x] BuildRun 支持选择多个构建变量集，Builder 领取任务时按权限解析变量。
- [x] 为 BuildRun 预留 cache 配置字段，MVP 先不启用缓存。
- [x] 记录 image tag、digest、source commit 和构建产物归属。
- [x] 记录 CPU、内存和 credit 消耗字段，计费系统先不实现。

### 7.2 构建 Builder 执行链路

- [x] 构建执行链路收敛为平台 Builder Agent，移除 GitHub Actions、Gitea Actions 和 Kubernetes Job 构建执行支持。
- [x] API 只创建 queued 状态的 BuildRun / BuildJob，并投递到 Redis builder stream 由 Builder Agent 领取。
- [x] Builder Agent 注入 Git 和 registry 凭据到一次性 executor 容器。
- [x] Builder Agent 注入构建变量到一次性 executor 容器，并作为 BuildKit build-arg 传入。
- [x] Builder Agent 实时记录构建日志。
- [x] Builder Agent 回写构建状态、镜像引用、digest 和 source commit。
- [x] 前端构建页新增构建变量集管理、触发构建变量集选择、任务日志 Dialog 和列表自动刷新。

### 7.3 构建安全与网络策略

- [x] 移除 Kubernetes Build Job 专属 NetworkPolicy 执行链路。
- [ ] 为 Builder executor 增加独立出站访问控制策略。
- [x] 支持公开 Git、公开 registry、公开包管理源访问。
- [x] 支持内网 registry/镜像源 TCP 443 白名单访问。
- [x] 禁止私有网段非 443 端口访问。
- [x] 禁止元数据地址、Kubernetes API Server 和 Service CIDR 访问。
- [x] 为构建网络拒绝事件记录审计日志。

### 7.4 Builder 详细排期

#### 7.4.1 API 与队列

- [x] API 创建 BuildRun / BuildJob 后保存在数据库构建队列。
- [x] Redis Builder 事件支持 heartbeat、claim、logs、complete、fail。
- [x] 移除 `BUILD_DISPATCH_MODE` 和旧 Asynq `build:run`，BuildJob 始终作为 Redis stream 任务由 Builder Agent 领取。
- [x] BuildJob 增加 `builderId` 和 `leaseUntil`，支持任务认领和 lease。
- [x] BuildRun 创建时补充应用、仓库绑定、目标镜像站和凭据权限的强校验，避免 Worker 阶段才发现无权限。
- [ ] BuildRun 支持 cancel 请求和 canceled 状态，Builder Agent 收到后终止对应 executor 容器。

#### 7.4.2 Worker / Builder Controller

- [x] 新增 `cmd/builder`，Builder Agent 通过 Redis stream 领取任务并执行。
- [ ] Builder Agent 增加任务 watch/cancel 流程，实时同步 queued/running/succeeded/failed/canceled 状态。
- [ ] Builder Agent 增加并发控制：全局、项目、用户三个维度的构建并发额度。
- [ ] Builder Agent 增加超时处理：超过任务超时时间后终止 executor 并标记 failed。
- [ ] Builder Agent 增加失败重试策略，由平台任务队列控制 attempts 和重试窗口。

#### 7.4.3 Builder Image / Job 执行

- [x] 默认使用 BuildKit rootless 构建镜像。
- [x] Builder Agent 支持 Docker executor：每次构建通过 Docker CLI 启动独立 executor 容器。
- [x] Builder Docker executor 本地开发模式允许使用 privileged BuildKit 容器，解决嵌套构建 `/proc` 挂载权限问题。
- [x] Builder Git clone 增加浅克隆和重试，BuildKit 构建增加整体重试，缓解 GitHub/DockerHub 网络 EOF。
- [x] 修正 DockerHub 镜像引用和认证地址：镜像使用 `docker.io/...`，auth config 使用 DockerHub 兼容 key。
- [x] Builder 增加 `BUILDER_NPM_REGISTRY` 配置入口，默认值下沉到代码，compose 只保留必要连接和身份配置。
- [x] Builder 通信收敛为 Redis-only：API 投递任务到 Redis stream，Builder 消费任务并写事件，worker 消费事件落库。
- [x] Builder 使用 `BUILDER_AGENT_NAME` 作为唯一标识自动注册，worker 定时同步 agent online/offline 状态到数据库。
- [x] Builder 支持 `BUILDER_SCOPES` 和 `BUILDER_LABELS` 上报；API 触发构建时按应用构建标签、builder scope 和在线状态随机选择可用 Builder，并投递到 Builder 专属 Redis stream。
- [x] 构建器页面支持删除已注册 Builder Agent 记录，便于清理已移除或长期离线的构建器。
- [x] 构建运行支持重试，应用构建列表行操作菜单提供重试和右侧日志/日志流侧栏。
- [x] 修复容器内 Builder 使用宿主机 Docker socket 时 workspace host path 误指向容器内路径导致 Docker Desktop mounts denied 的问题，并验证 neo-blog 前端镜像成功构建推送。
- [ ] HTTP Builder 接入暂缓：后续如需外部 Builder，再设计平台生成 token、hash 存储、明文只展示一次、吊销/过期和 agent 绑定。
- [ ] 制作平台自有 builder 镜像，内置 git、ca-certificates、buildctl、shell、jq、基础诊断工具。
- [ ] 新增 Builder Profile：支持配置 executor 类型、executor image、是否 privileged、CPU/内存/超时/并发、适用项目范围和能力标签。
- [ ] Builder executor image 必须可配置，默认推荐 BuildKit，生产环境支持使用本地或内网镜像站中的 BuildKit 镜像。
- [ ] Builder Job 明确入口脚本：clone、checkout、registry login、buildctl build、push、输出 result。
- [ ] Builder Job 输出结构化 `result.json`，包含 `imageRef`、`imageDigest`、`sourceCommit`、`startedAt`、`finishedAt`。
- [x] Builder Job 不持有平台 API token，不直接回调 API；Builder Agent 仅通过 Redis stream 与平台交换任务和事件。

#### 7.4.4 隔离与安全

- [x] Builder Job 不挂载宿主机 Docker socket。
- [x] Builder Job 默认不使用 privileged。
- [x] 每个 BuildRun 使用独立 Kubernetes Secret 注入 Git/Registry 凭据。
- [x] build namespace 应用 restricted BuildNetworkPolicy。
- [x] Docker Compose 增加 builder 服务，MVP 可通过 Docker socket 跑 Docker executor。
- [ ] 每个 Builder Job 使用受限 ServiceAccount，默认不授予读取集群资源权限。
- [ ] Builder Job 完成后立即删除临时 Secret，Job/Pod 按 TTL 保留日志窗口后清理。
- [ ] 将构建出口网络拒绝事件接入审计或日志视图。

#### 7.4.5 日志、结果和前端展示

- [x] BuildJob 记录 `logRef`。
- [x] Builder Agent 实时采集 executor stdout/stderr，并按 1 秒或 8KB 批量追加到平台 BuildLog。
- [x] 增加 BuildLog 适配，避免只依赖 Kubernetes Pod 日志保留。
- [ ] 后续增加日志对象存储适配，用于大日志归档、检索和下载。
- [ ] 前端构建详情页展示实时日志、状态流转、镜像引用和 digest。
- [ ] 构建列表增加自动刷新、手动刷新和状态过期提示，避免 BuildRun 已完成但前端仍显示 queued。
- [ ] 构建成功后自动创建 ContainerImage，并与 BuildRun、Application、commit 关联。
- [ ] 发布页选择 BuildRun 时优先展示 succeeded 且存在镜像产物的构建记录。

#### 7.4.6 后续增强

- [ ] 接入 registry cache / BuildKit cache，MVP 先保留字段不启用。
- [ ] 新增 Mirror Policy：全局、项目和构建任务三级配置 DockerHub、GHCR、npm、PyPI、Go proxy、Maven、Gradle、Cargo 等镜像源。
- [ ] BuildKit 支持 registry mirror / pull-through cache 配置，优先支持 DockerHub、GHCR 和平台内网镜像站。
- [ ] 语言依赖工具链按生态注入镜像源，可选是否注入环境变量或配置文件：npm/pnpm/yarn、pip/poetry、GOPROXY、Maven settings、Gradle init、Cargo config 等。
- [ ] 镜像源凭据通过 BuildKit secret 或一次性文件注入，禁止进入最终镜像层。
- [ ] 支持远程 buildkitd pool，用于高并发或需要共享缓存的场景。
- [ ] 支持构建资源消耗统计，回写 CPU core seconds、memory MB seconds 和 creditCost。
- [ ] 支持构建队列优先级和项目级限额策略。
- [ ] 保留 External CI Provider 作为后期扩展，不作为 MVP 主路径。

## 8. 集群与部署

### 8.1 集群与部署 API/CRUD 优先

- [x] 实现 RuntimeCluster 模型、迁移和 CRUD API。
- [x] 支持设置默认集群。
- [x] RuntimeCluster 支持 global/project/user scope，只有 global 集群允许设为默认集群，列表按当前用户可访问范围返回。
- [x] 实现 kubeconfig 保存、按权限回显和测试连接 API：仅创建者本人或平台管理员可以查看和编辑 kubeconfig 明文。
- [x] 运行集群测试改为真实 Kubernetes API Server `/version` 连通性检测；无 kubeconfig、无效 kubeconfig 或网络不可达时写入失败状态并返回错误。
- [x] 集群前端接入改为 kubeconfig-only YAML 代码框，普通用户列表不展示无权维护集群的 endpoint 配置。
- [x] 约束 kubeconfig 代码框宽度，长行只在编辑器内部滚动，不撑大 Dialog 或表单布局。
- [x] 引入公共 `CodeEditor` 代码编辑框组件，集群 kubeconfig 使用 YAML 高亮、等宽字体和行号。
- [x] Kubernetes 和 K3s 接入选项合并为 Kubernetes / K3s，后端兼容旧 k3s 输入并归一到 kubernetes。
- [ ] 设计 Docker 运行时接入模型：支持 Docker host、Unix socket、TCP TLS CA/cert/key、连接测试、权限边界和部署适配，不复用 kubeconfig 字段。
- [x] 实现 Environment 模型、迁移和 CRUD API。
- [x] 实现 Release 模型、迁移和列表/详情 API。
- [x] 实现部署配置 API：镜像来源、环境变量、ConfigMap/Secret 引用、资源规格、副本数。
- [x] 实现手动部署 API，先创建 pending 状态 Release。
- [x] 发布表单以 BuildRun 为主输入，选择构建记录后自动带出应用和镜像；部署分支以 BuildRun 的 sourceBranch 为准。
- [x] 实现回滚请求 API，先创建 rollback 类型 Release。

### 8.2 部署 Worker/执行链路

- [x] 创建 Project namespace。
- [x] 实现 Deployment/Service/ConfigMap/Secret apply。
- [x] 实现 rollout 状态等待。
- [x] 实现 Release 状态回写。
- [x] 实现回滚到上一成功版本。

## 9. 网关与域名

### 9.1 网关与域名 API/CRUD 优先

- [x] 实现 GatewayRoute 模型、迁移和 CRUD API。
- [x] 实现默认域名生成规则 `{appSlug}-{projectSlug}.{stage}.{rootDomain}`。
- [x] 实现域名冲突检查 API。
- [x] 支持自定义域名创建和状态管理。
- [x] 生成 CNAME 目标并返回给前端展示。
- [x] 支持 HTTP-only 访问开关。
- [x] 实现证书状态字段：disabled、pending、issued、failed、expired。

### 9.2 网关 Worker/控制链路

- [x] 创建 Ingress。
- [x] 校验 DNS CNAME。
- [x] 支持 HTTP Challenge 证书申请。
- [x] 实现证书续期检查和状态回写。

## 10. 前端联调验收

- [x] 实现仓库绑定页，并与 Git 集成基础 CRUD 占位联调。
- [x] 仓库绑定页接入真实 OAuth、仓库列表、分支读取和 webhook 创建状态。
- [x] 实现镜像站页，并与 ArtifactRegistry 联调。

### 10.1 前端 CRUD 联调优先

- [x] 实现构建页，并与 BuildProvider、BuildRun、BuildJob CRUD 联调。
- [x] 实现部署环境页，并与 RuntimeCluster、Environment、Release CRUD 联调。
- [x] 实现网关域名页，并与 GatewayRoute CRUD 联调。
- [x] 将构建、部署、域名网关从顶层菜单收敛到应用详情页 `ContentTabs`，并新增独立集群页。
- [x] 侧边栏项目空间支持项目下钻应用快捷入口，应用详情页承载概览、构建、部署、域名网关和配置。
- [x] 侧边栏具体项目支持独立展开/收起，默认收起，展开后再加载并展示应用快捷入口。
- [x] 使用 Chrome 验收构建、部署、域名 CRUD 页面。

### 10.2 前端执行态联调

- [ ] 构建页接入 BuildRun 实时状态和日志查看。
- [ ] 部署环境页接入 Release 状态、rollout 进度和回滚结果。
- [ ] 网关域名页接入 DNS 校验、Ingress、证书申请和续期状态。
- [ ] 使用 Chrome 验收仓库绑定、构建、镜像站、部署、域名完整链路。

## 11. 安全与后端结构优化

- [x] 新增独立 `secret_values` 表，新写入的 Git/OIDC/Registry/Webhook 密钥在业务表只保存 `secret-id` 引用。
- [x] 移除 `literal:` 明文 secret ref 和未知前缀裸值解析；生产模式强制要求 `SECRET_ENCRYPTION_KEY`。
- [x] 生产模式 session cookie 设置 `Secure=true`，本地开发保持 `Secure=false`。
- [x] 为项目删除、应用删除、成员变更、Access Token、Git Provider/GitAccount、Git Webhook、Registry/RegistryCredential 和 Secret 写入补充 AuditLog。
- [x] 第一阶段后端结构拆分：`internal/model` 按用户、认证、项目、应用、Git、镜像站、审计、密钥拆分文件；`internal/api` 按认证用户、项目、应用、Access Token、会话、分页响应拆分 handler 和公共工具。
- [x] 第二阶段后端分层：新增 `internal/service` 承载 Access Token、Git、Registry 纯业务规则，新增 `internal/repository` 承载项目成员关系查询。
- [x] 继续瘦身 `internal/api`：Git 外部平台客户端迁入 `internal/provider/git`，Registry 连接检测和 SSRF 目标校验迁入 `internal/provider/registry`，密钥加密和 secret-id 存储迁入 `internal/secret`。
- [x] 将 Git API 大文件拆分为 Provider、Account、RepositoryBinding/Webhook 和 Helpers 多文件，避免单文件承载多个业务块。
- [x] 将 Registry API 拆分为镜像站、凭据、容器镜像、访问控制、连接测试和 DTO 多文件，`registries.go` 仅保留镜像站主 CRUD 与输入转换。
- [x] 新增 `tests` 自动化验证：API smoke 覆盖主要后端资源域，Browser smoke 使用 Playwright 登录并访问核心前端页面，`tests/run-all.sh` 串联 Go 测试、前端构建/lint、API 和浏览器验证。
- [ ] 后续按领域继续拆分后端：将 Git/Registry/Auth/Project handler 的复杂流程逐步沉入 service/repository/provider，handler 保持 HTTP 适配层。

## 100.优化需求

- [ ] 智能引导：例如用户在创建APP选择Git账号时发现没有账号，旁边用一个按钮引导去授权页面。这样的场景还有很多，不一定是Git账号，后续可以总结一批这样的场景进行统一优化。
