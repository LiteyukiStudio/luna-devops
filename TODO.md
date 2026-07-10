# TODO

## 1. 文档与原型收口

- [x] 更新产品原型为文档式多页面线框。
- [x] 在原型中覆盖创建应用、构建、部署、镜像站、自定义域名、Access Token、配额页面。
- [x] 检查并移除文档中旧的 Actions / 常驻 Builder 构建路径表述，构建主路径收敛为 Worker 调度部署集群 Kubernetes Job。
- [x] 新增旧 Builder 真实构建验收复盘文档，记录问题、修复项、待优化项和构建镜像源方案。
- [x] 新增代码健康检查 SOP，沉淀 AI 长期协作项目的定期检查、重构触发条件和健康记录模板。
- [x] 清理已被 `AGENTS.md` 吸收的旧版技术栈文档 `docs/02-项目技术栈要求.md`。
- [x] 删除过期产品原型和暂不启用的 AI 能力提案文档，README 与 AGENTS 只保留当前有效文档入口。
- [x] 删除独立品牌说明和旧 Builder 构建验收复盘文档；构建后续计划已由 TODO 中的细分条目承接。
- [x] 将旧 `docs/` 内容迁移到 `notes/`，并使用 Rspress 建立支持中英双语、响应式和多主题的文档站。
- [x] 补充外部组件兼容矩阵文档，覆盖 GitHub/Gitea、镜像仓库、Kubernetes/Gateway API、OIDC、PostgreSQL、Redis、BuildKit、Prometheus/Grafana 和通知适配器的接口与版本范围。

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
- [x] 后端提供 Swagger UI 调试入口：运行中的 API 暴露 `/openapi.yaml` 和 `/swagger`，便于查看接口规格和发起开发调试请求。
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
- [x] 统一列表行内操作按钮为无边框 ghost 风格，减少同一列表内边框按钮和纯文本按钮混用。
- [x] 增加 MUST 准则：前端基础组件优先使用 shadcn/ui，有现成组件时禁止自造轮子。
- [x] 在 `web/SHADCN_COMPONENTS.md` 维护 shadcn/ui 官方组件清单和替换优先级。
- [x] 接入 Sonner toast。
- [x] 为自动关闭 toast 增加倒计时进度条。
- [x] 前端 API client 增加统一请求/响应错误封装，按后端稳定 code 做 i18n 映射，并对本地网络、代理和 VPN 连接失败给出友好提示。
- [x] 实现 light/dark/system 主题三态。
- [x] 将控制台默认主色调整为 Kubernetes 风格蓝，并同步品牌与原型文档。
- [x] 建立前端基础布局、路由和 API client。
- [x] 将用户信息和主题切换控件移动到侧边栏底部。
- [x] 引入前端轻量动效，覆盖页面切换、列表、弹窗和基础控件。
- [x] 按前端体验优化报告完成部署主路径减负：部署配置、部署列表、发布弹窗、应用概览、镜像站凭据、代码源 Provider 和 Docker 构建上下文均完成收口。
- [x] 将侧边栏导航改为二级分组结构，按 DevOps、个人工作区、系统管理分栏展示。
- [x] 新增 `/dashboard` 看板页，登录后默认进入看板，聚合项目空间、应用、近期构建、镜像站和集群状态；最近构建固定 4 条高度并支持最多 20 条滚动查看，按“项目空间 · 应用”展示并可一键直达具体页面。
- [x] 看板常用项目空间改为一行横向滚动展示，最多 16 个；固定项目空间优先，其余按当前用户使用频次和最近使用排序。
- [x] 项目空间列表支持最近使用、使用频次、创建时间、更新时间和名称排序；最近使用与使用频次写入 `project_members.last_used_at/use_count`，不复用项目更新时间。
- [x] 将内容区顶部改为当前页面标题和说明，正文区域不再重复渲染页面标题。
- [x] 项目空间和应用详情页 topbar 使用资源类型前缀展示当前资源名称，应用详情页按“项目空间 / 应用”展示，并在应用列表提供进入应用详情的唯一入口。
- [x] 项目空间、应用、部署配置详情页 topbar 名称链路可点击：项目名进项目面板，应用名进应用面板，模块名进模块面板。
- [x] 移动端基础适配：桌面侧边栏在小屏切换为带滑入滑出动画的可折叠侧边抽屉，`DataList` 小屏保留表格并支持左右滑动，`ContentTabs` 小屏改为当前 tab 下拉选择，代码库/镜像站/身份源等配置表单小屏单列化。
- [x] 浏览器标签页标题统一为 `{page title} - {site title}`，其中 page title 复用内容区 topbar 标题。
- [x] 前端首次加载支持浏览器语言自动检测；未登录按浏览器语言，登录后按用户偏好语言覆盖并缓存。
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
- [x] 项目空间列表增加范围筛选：默认展示“与我相关”，平台管理员可手动切换到“全部项目空间”。
- [x] 抽离通用 `HoverText` 单行省略组件，删除失败原因展示在状态 badge 右侧，hover/focus 展示完整原文，避免错误日志撑开列表。
- [x] 项目空间工作台用 `ContentTabs` 集中概览、应用、成员内容，仓库绑定改为挂靠到应用下面。
- [x] 应用详情新增“仓库”tab，按当前应用过滤 RepositoryBinding；仓库新建入口收敛到部署配置表单，保存后自动选中到当前部署配置。
- [x] 项目空间概览改为项目级 dashboard，聚合应用、最近构建、发布健康、访问入口、变量密钥和成员摘要；粒度保持项目级，避免与应用概览重复。
- [x] 项目页面命名统一：仓库绑定显示为仓库，变量和密钥显示为构建变量，运行配置集显示为配置。
- [x] 移除项目空间工作台残留的仓库 tab 和概览仓库入口，旧项目仓库 URL 重定向到应用 tab，仓库绑定入口继续向应用详情收敛。
- [x] 项目空间工作台的新增应用、添加成员等 tab 主操作统一放入 `ContentTabs.tools`。
- [x] 应用行新增仓库绑定入口，支持用 Git 凭据搜索可见仓库并自动回填。
- [x] 应用编辑仓库来源移除 owner/repo/cloneUrl 自由输入，Dockerfile 与构建上下文改为在部署配置中维护，避免应用信息表单和部署配置重复。
- [x] 应用保存当前 `gitAccountId`，编辑时优先从 RepositoryBinding 恢复，绑定缺失时回退应用字段，避免重复选择 Git 凭据。
- [x] 应用仓库绑定增加同应用同 Git Provider、同 owner/repo 的重复校验；后端返回冲突，前端保存前提示并禁用重复提交。
- [x] 将 Dockerfile/构建目录探测迁到后端 build-options 接口，优先使用 recursive tree API，前端不再逐目录串行探测。
- [x] Dockerfile 和构建上下文改为可输入候选建议，探测结果只作为建议，不阻止用户手动修正。
- [x] 新增统一搜索选择器，并将 Git 分支选择改为 search/limit 查询、短缓存筛选和前端最大展示。
- [x] 修复 Dialog 内搜索下拉撑高弹窗的问题，并将 Git 分支缓存改为分页拉取后再搜索。
- [x] 补齐管理类列表分页：Git Provider/Git Account、应用、仓库绑定、项目成员、镜像站/单镜像站凭据、构建变量集、运行配置集、部署配置和访问入口支持 `items/page/pageSize/total/totalPages`，前端主要管理页接入 `DataList` 分页控件。
- [x] 为 `web/src/components/common` 公共组件补充用途、适用场景和边界注释，方便后续 AI 复用。
- [x] 项目详情页之间切换时复用同一个页面动画容器，只让项目信息和 tab 内容轻量切换。
- [x] 全部项目列表只保留进入工作台入口，旧项目内列表路由重定向到工作台。
- [x] 第一批 CRUD 弹窗化：项目空间、应用、项目成员、用户、身份源、Access Token。
- [x] 项目空间添加成员改为用户候选搜索与多选提交：Owner/Admin 按用户名或邮箱搜索平台用户，只能添加已选中的真实用户，避免自由输入邮箱误提交。
- [x] 将镜像站页面拆为镜像站、凭据和镜像子 tab，并将创建/编辑表单改为 Dialog。
- [x] 移除构建器 / Builder Token 主路径：构建由 Worker 在部署集群创建 Kubernetes Job，不再需要用户维护构建器接入。
- [x] 应用支持多个部署配置 `DeploymentTarget`，BuildRun 记录 `deploymentTargetId`，触发构建时选择部署配置并从配置继承 Dockerfile、构建上下文和目标镜像；项目空间变量和密钥默认注入构建容器。
- [x] 创建应用和应用配置表单不再填写部署配置；部署配置没有默认属性，触发构建和部署链路必须显式引用具体部署配置。
- [x] 应用基础信息不再维护服务端口；服务端口改由部署配置维护，部署生成 Service 使用部署配置端口，访问入口默认继承部署配置端口。
- [x] 部署配置支持多个服务端口：DeploymentTarget 维护端口列表，Kubernetes Service/容器端口按列表生成，访问入口只能选择该部署配置已暴露的端口。
- [x] 部署配置删除级联清理绑定的访问入口：API 同步标记关联 GatewayRoute 删除中，worker 先删除入口再清理工作负载，访问入口遇到部署配置已不存在时不再卡死。
- [x] 新增部署配置 `DeploymentTarget`，部署配置直接绑定环境并配置自动部署策略；构建成功后按策略自动创建 Release 并投递部署任务，部署页展示部署配置、最新发布状态、日志、回滚和手动发布。
- [x] 新增部署配置时自动部署默认启用，用户可在部署配置表单中手动禁用。
- [x] 镜像站凭据支持可编辑镜像仓库模板和 Tag 模板：创建部署配置时按项目空间、应用和 `stage` 生成默认仓库，触发构建时按分支、tag、commit 等变量渲染最终 tag。
- [x] 应用访问入口创建域名时直接选择部署配置，GatewayRoute 由 DeploymentTarget 推导应用、环境和目标 Service，留空域名时沿用平台默认域名生成规则。
- [x] 镜像凭据 tab 默认展示全部镜像站凭据，并在凭据行展示所属镜像站名称。
- [x] 构建 running 阶段展示当前步骤，日志抽屉标题同步显示状态 Badge，并修复构建运行操作菜单长文案溢出。
- [x] 构建运行时间展示优化：10 小时内显示相对时间，超过后显示带年份绝对时间，并展示构建耗时。
- [x] 构建时间与耗时展示走 i18n，中文时间单位统一使用 `时 / 分 / 秒`。
- [x] 抽离通用时间格式化工具，构建任务复用同一套 10 小时内相对时间、超过后绝对时间策略。
- [x] 应用构建运行列表接入后端分页，并支持 Event、Status、Branch、Actor 筛选；全局分页组件调整为 GitHub 风格页码。
- [x] 构建运行行压缩信息密度：代码来源和运行/镜像状态分别用 badge 分组展示，并区分平台触发人与 Git 提交者。
- [x] 构建运行 badge 支持操作：仓库/提交跳转到 Git commit，Git 提交者跳转 Git 平台主页，镜像标签点击复制并 toast 提示。
- [x] 部署配置支持构建超时时间：默认 30 分钟，创建 BuildRun 时快照到构建记录，Worker 创建 Kubernetes Job 时写入 `activeDeadlineSeconds` 并按该值终止超时构建。
- [x] 构建页操作收敛到 ContentTabs 工具区：触发构建按钮提升到页签工具栏，触发表单继续使用 Dialog；构建记录支持删除终态记录并清理关联任务日志和 HookRun 记录。
- [x] 构建运行进度改为后端回传原始 key、前端 i18n 展示，并移除右侧时间区域的阶段文案。
- [x] 构建日志接入 SSE stream，支持按 offset / Last-Event-ID 从已落库日志继续读取；前端日志抽屉从轮询改为 EventSource。
- [x] 修复应用构建页外层构建运行和构建任务查询不轮询导致列表与阶段进度不实时刷新的问题。
- [x] 应用详情页移除旧交付详情入口，将部署配置维护收敛到应用概览、构建、部署和访问工作流。
- [x] 统一构建和部署列表状态短轮询间隔，应用构建页、应用部署页和看板近期构建复用同一常量刷新运行态数据。
- [x] 应用部署页目标镜像和部署进度保持常规截断，hover 展示完整内容，点击文本可复制完整值并提示复制成功。
- [x] 为 Project/Application 关键命名片段增加长度防呆，提示用户使用短 slug；DeploymentTarget 名称改为纯展示名，Kubernetes 资源名由内部 ID 派生。
- [x] 修正 Kubernetes 资源名生成：部署配置不再使用用户填写名称生成运行态资源名，避免中文名称或长名称影响 Deployment/Service；前后端允许部署配置名称作为可读展示名自由填写。
- [x] Kubernetes 运行态资源命名改为内部 ID 派生：namespace 使用 `ns-{projectIdShort}`，Deployment/Service/ConfigMap/Secret 使用 `dplt-{deploymentTargetIdShort}`，平台识别依赖稳定 ID labels。
- [x] 部署配置支持维护运行时 ConfigMap/Secret 覆盖项：入口挂靠到模块的部署配置，按“环境默认 + 部署配置覆盖”生成运行态资源，Secret 不回显原文。
- [x] 增加 i18n 边界准则：能在前端本地化的内容由前端按 key 映射，后端只返回稳定 key、原始枚举和必要备注。
- [x] 继续将仓库绑定等多表单页面改为创建/编辑 Dialog。
- [x] 使用浏览器验收前端启动、主题切换和基础路由。
- [x] 开发环境使用 Vite proxy 反代后端 API。
- [x] 为 api、worker 编写 Dockerfile；api 支持 `embed_web` 构建内嵌前端 SPA，前端不再维护独立 Dockerfile。
- [x] 提供完整 docker compose 运行编排。
- [x] 拆分开发依赖和完整部署的 compose 边界：开发依赖使用独立项目名并暴露 PG/Redis，完整部署的 PG/Redis 仅走容器内网络，避免端口和容器项目名冲突。
- [x] 明确本地开发拓扑：前端、API、worker 优先在宿主机运行，PG/Redis 由 dev compose 提供；构建联调使用部署配置所选运行集群的 Kubernetes Job。
- [x] 将 compose 场景收敛为三份：`docker-compose-dev.yaml` 启动 PG/Redis/worker 用于开发联调，`docker-compose.yaml` 使用 DockerHub 镜像启动完整部署栈，`docker-compose-build.yaml` 从当前源码构建完整部署栈。
- [x] 新增 Helm Chart：支持一键在 Kubernetes / K3s 中部署 API、worker、PostgreSQL 和 Redis，并支持切换外部数据库、外部 Redis、Ingress 和固定镜像版本。
- [x] 环境文件按运行边界拆分：`.env` 只保留基础模式开关，`.env.development` 面向宿主机进程，`.env.worker` 面向 worker 容器，并提供对应 `.example` 模板。
- [x] API/Worker 数据库连接增强：启动时对 PostgreSQL 做可配置重试，统一限制每进程连接池大小、空闲连接和连接生命周期，避免多副本部署或数据库短暂满连接时直接崩溃。
- [x] `docker-compose.yaml` 和 `docker-compose-build.yaml` 内联 API / worker 运行环境变量，生产密钥、域名和镜像 tag 通过宿主机环境变量覆盖；`docker-compose-dev.yaml` 继续使用 `.env.worker` 服务开发联调。
- [x] 新增 GitHub Actions 容器发布工作流：仅构建 `linux/amd64` 容器镜像，发布 DockerHub `liteyukistudio/devops-api`、`liteyukistudio/devops-worker`；分支发布 `nightly`，`v*` tag 发布版本 tag，稳定版本 tag 额外发布 `latest`；`devops-api` 使用 `embed_web` 内嵌前端静态文件，不额外构建或上传 GitHub Release 二进制产物。
- [x] 修复内嵌 SPA 根路径和 fallback 被 Go FileServer 重定向到 `./` 的问题：`index.html` 改为直接返回，避免服务端根路径出现不必要 301。

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
- [x] 开发环境放宽登录、首个管理员初始化和镜像搜索限流到 `10000/分钟`，Redis 临时不可用时不阻断本地初始化和调试；生产环境仍保持 Redis 限流失败即拒绝。
- [x] 身份源表单展示后端 `PUBLIC_BASE_URL` 生成的 OIDC Redirect URI，并提供复制入口，避免管理员在 OIDC Provider 应用里配置错回调地址。
- [x] 准入策略支持配置“要求 OIDC 邮箱已验证”：默认开启，可信内部身份源无法返回 `email_verified=true` 时可关闭，但仍要求非空邮箱。
- [x] 支持 OIDC 通过准入策略校验后的非空邮箱绑定现有用户。
- [x] 登录页对 OIDC 回调错误补齐细分文案，并同时展示页面错误块和 toast，增强失败反馈。
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
- [x] 建立 `internal/authz` 轻量授权中心：集中定义权限 action、项目角色矩阵和 Access Token scope 规则，保留现有角色体验并为后续 RBAC 收敛提供统一入口。
- [x] 实现 Access Token 创建、hash 存储和撤销。
- [x] 实现 Access Token scope 校验。
- [x] 收紧 Access Token scope：未知 API 默认拒绝，创建 scope 白名单化，普通用户只能创建读类和明确自动化触发类 scope。
- [x] 增加 API CORS 白名单、Cookie 会话 Origin 防护和基础安全响应头。
- [x] 补齐生产安全响应头：API 增加默认 CSP，HSTS 按 `APP_ENV=production` 或 `APP_ENABLE_HSTS=true` 启用，本地 development 不强制 HSTS。
- [x] 为本地登录和首个管理员初始化增加基础限流。
- [x] Access Token 列表隐藏已撤销 Token，时间列单行展示，有效期改为固定选项并支持 0 无限有效。
- [x] 实现登录页。
- [x] 实现当前用户和基础权限状态管理。
- [x] 实现用户语言偏好保存和前端 i18n 同步。
- [x] 新用户创建时自动生成一个以用户名命名的默认项目空间，并将该用户设为项目空间 owner；默认项目名称按用户语言本地化。
- [x] 实现项目成员权限状态管理。
- [x] 增加项目 owner 保护：admin 不能授予或修改 owner，禁止删除或降级最后一个 owner。
- [x] 为复杂表单字段补充 label 问询提示和统一校验交互。
- [x] 将用户列表改为统一列表组件展示，并接入后端分页查询。
- [x] 为用户列表 API 补充排序字段和排序方向参数。
- [x] 将 Access Token 管理合并到账号页，作为“个人令牌”子 tab。
- [x] 将 Access Token 列表改为统一列表组件展示，并接入后端分页查询。
- [x] 为 Access Token 列表 API 补充排序字段和排序方向参数。
- [x] 个人令牌支持后端同步权限目录和多选 scope 创建，权限粒度细化到项目空间、应用、部署、构建、访问入口、配置密钥、集群、代码库、镜像站、账单和系统管理。
- [x] 将统一列表分页替换为页码式分页控制器，支持页码、省略号、上一页和下一页。
- [x] 统一分页组件支持每页条数选择，统一列表滚动限制在表格区域内。
- [x] 项目空间列表接入分页与每页条数选择，侧边栏项目空间父入口默认进入列表并移除子级“全部项目”。
- [x] 侧边栏项目空间展开区限制为同时展示 6 个项目，其余项目在子列表内滚动。
- [x] 修复项目空间列表 total 与可见行不一致：浏览器场景中空状态显示“还没有项目空间”但分页仍显示有记录，新建后只显示 1 行却显示“共 2 条”；排查软删除、权限过滤和分页 total 统计口径。
- [x] 抽离统一分页组件，并将列表 API 改造为支持分页、排序、搜索和可选批量选择。
- [x] 使用浏览器验收本地登录、退出和 Access Token 创建/撤销流程。
- [x] 使用浏览器验收权限隐藏流程。
- [ ] 设计并实现敏感操作 Step-up MFA：管理员可在全局安全设置中开启/关闭敏感操作二次验证，并配置验证有效期、无操作重新验证时间和最大持续有效期。
- [ ] 用户账号安全页支持绑定离线 TOTP 身份验证器：后端生成加密存储的 TOTP secret，前端展示二维码和手动密钥，用户输入 OTP 校验成功后才正式启用。
- [ ] 为用户生成一次性恢复码：恢复码只展示一次、仅存 hash、单个恢复码只能使用一次，支持用户重新生成并作废旧码。
- [ ] 敏感操作 API 增加 MFA step-up 校验：Web Console、Secret/Registry Credential 查看或修改、kubeconfig/数据导出、删除资源、修改 OIDC/Git Provider/站点安全设置、管理员账号变更和高风险部署操作在策略开启后必须先完成 MFA。
- [ ] MFA challenge/assertion 状态后端化：按 user/session/purpose 记录验证状态，支持 Redis 或数据库存储，多副本部署下共享；敏感操作通过后刷新 last activity，超过无操作时间或绝对有效期后重新要求 OTP。
- [x] Step-up MFA 后端第一阶段：新增 `security.stepUpMfa.enabled` 开关、`step_up_assertions` 共享表和 `requireStepUp` 统一检查点；已接入 runtime exec/terminal、数据导出、Secret/Registry Credential 写入、kubeconfig 更新、Auth Provider 更新和管理员用户变更，未通过时返回 `mfa_required`。
- [ ] 前端统一 MFA Dialog：敏感操作遇到 `mfa_required` 后弹出 OTP/恢复码输入框，验证通过后自动继续原操作，所有文案走 i18n。
- [ ] MFA 安全控制：OTP 校验允许小时间漂移，验证接口按用户和 IP 限流，OTP/恢复码不写日志，密码重置、禁用账号、角色变化和 MFA 重置后清理已有 assertion。
- [ ] MFA 管理与审计：用户解绑 MFA、重新生成恢复码、使用恢复码、管理员重置用户 MFA、敏感操作 MFA 通过/失败均写入 AuditLog；管理员不可查看用户 TOTP secret 或恢复码明文。

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
- [x] 应用模型新增预设图标字段，创建/编辑和应用配置页可通过悬浮图标选择器修改，并在列表与概览展示。
- [x] 应用配置页新增目标镜像模板变量参考表，宽屏在配置表单右侧展示，窄屏自动落到表单下方。
- [x] 应用配置表单内展示当前 Webhook 配置和状态，并提供重新配置 Webhook 操作。
- [x] Webhook 配置展示增加 PUBLIC_BASE_URL 防呆提示，发现空回调、相对地址或 localhost/127.0.0.1 时提醒用户外部 Git 平台无法访问。
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

## 4.1 应用最小单元与模块退场重构

- [x] 开发准则：平台尚未上线且本地数据无兼容价值，本轮允许破坏式重构；数据库迁移、API、前端状态和文档不做前向兼容，旧模块数据可直接删除或重建。
- [x] 产品口径调整：废弃“模块”作为用户侧主概念，明确“应用”是最小构建、部署、发布、访问和运行时配置归属单元；monorepo 通过多个应用绑定同一仓库、不同构建上下文和 Dockerfile 支持。
- [x] 重塑交付模型：将部署配置 `DeploymentTarget` 提升为应用下的主交付配置，承载环境、触发规则、构建配置、运行时配置、自动部署、审批策略和访问入口绑定。
- [x] 后端模型迁移：移除旧模块主模型和模块 API，构建、触发、部署和 Hook 职责迁移到应用下的部署配置；不保留兼容读写和旧数据迁移脚本。
- [x] 构建模型调整：`BuildRun` 直接绑定 `applicationId` 和 `deploymentTargetId`，记录本次构建面向的环境/交付配置，避免通过旧模块层级间接判断 dev/prod 构建归属。
- [x] 发布模型调整：`Release` 直接绑定 `deploymentTargetId`，发布 Worker、回滚、部署日志和 Kubernetes 资源查询都以部署配置为目标，不再通过应用 + 环境 + 模块组合反查。
- [x] 触发边界收敛：Webhook 事件进入平台后按应用下的部署配置匹配 branch/tag 规则；触发与调度策略只归属部署配置，避免模块和部署配置同时匹配造成职责重复。
- [x] 支持镜像直部署：部署配置支持 `sourceType=image`，允许应用不绑定源码构建，直接选择镜像站镜像或填写镜像引用后创建 Release；源码构建路径使用 `sourceType=repository`。
- [x] 前端信息架构调整：应用详情移除“模块”页签，改为“概览 / 构建 / 部署 / 访问”等围绕应用和部署配置的工作流；应用 topbar 不再出现模块层级。
- [x] 文档与原型同步：更新产品方案、原型和术语表，明确“应用是最小交付单元”，补充“一个应用一个可部署服务”的 monorepo 使用示例。
- [x] Chrome 场景模拟记录：已按 `docs/场景模拟.md` 跑本地核心 DevOps 路径，并记录主路径阻塞与体验卡点。
- [x] 修复应用创建失败：移除 `applications.source_type` 等旧来源字段的数据库非空约束或模型残留，使创建应用只提交名称、标识和图标即可成功。
- [x] 补齐部署配置维护入口：应用详情页需要提供创建、编辑、删除 DeploymentTarget 的明确入口，并覆盖 repository / image 两类来源、环境、构建参数、运行配置和访问绑定。
- [x] 修正发布创建入口状态：没有可发布的、已绑定部署配置的 BuildRun 时，顶部“创建发布”应禁用或给出明确空状态；旧构建记录不能作为可选项混入。
- [x] 清理应用与看板 UI 残留：创建应用弹窗描述改为新模型口径；看板图钉按钮补齐 i18n；应用概览合并重复的部署配置指标。
- [x] 本地验收数据重建：通过浏览器 UI 清理旧项目空间数据和集群残留资源后继续重建，确保验证数据全部使用 `deploymentTargetId` 新模型。
- [x] 补齐仓库绑定入口：清库后用户必须能在项目空间或应用详情内通过 UI 重新绑定 Git 仓库，否则 repository 构建场景无法从浏览器跑通。
- [x] 补齐集群资源清理入口：平台管理的 Deployment/Service/ConfigMap/Secret 出现无部署配置归属或错误发布时，用户需要能通过 UI 删除。
- [x] 部署失败诊断第一阶段：集群工作负载页 Pod 摘要展示容器 waiting/terminated reason 和 Pod condition，避免 rollout stuck 时只能看到 `ready=0`。
- [x] 补齐部署运行日志入口：应用部署页“查看日志”支持在部署日志和当前运行 Pod 容器日志之间切换。
- [x] Web Console MVP：应用部署页支持对当前发布对应的运行中 Pod 执行一次性命令，后端通过 Kubernetes exec 执行并写入审计日志。
- [x] 代码健康修复：项目空间异步删除按项目环境覆盖的集群逐个清理 namespace；项目、部署配置、访问入口和运行配置写入口统一阻止 deleting 状态；启动清理移除旧 `applications.service_port` 字段；资源清理状态机拆出 `internal/worker/resource_cleanup.go` 并补单测。
- [x] 热点大文件第一轮拆分：`deployment_handlers.go` 拆出运行集群 handler，`web/src/api/client.ts` 拆出 DTO 类型，i18n locale 按语言和 namespace 拆分，`ApplicationConfigPage.tsx` 拆出 Overview 面板与页面工具函数。
- [ ] 补齐部署失败诊断第二阶段：发布日志和集群工作负载页继续补 Pod events、重新同步入口和镜像架构提示。
- [ ] 破坏式重建验收：主人确认 `docs/场景模拟.md` 后，清空本地旧库并按场景完成应用创建、部署配置创建、repository 构建、image 直部署、自动部署、域名、ConfigMap/Secret 覆盖、权限、Access Token、回滚和集群资源归属 smoke。
- [x] Neo Blog 镜像直部署验收：清库后通过浏览器创建项目空间、环境、数据库/后端/前端应用、镜像来源部署配置和 `blogtest.local` 访问入口；Postgres、后端、前端均 rollout 成功，Ingress 通过 `http://blogtest.local:64111/` 返回 200。
- [x] Browser 镜像直部署运行配置闭环：通过 UI 创建 nginx 镜像应用、部署配置、ConfigMap 环境变量、ConfigMap 文件、Release、运行日志、Web Console 和 `nginx-loop.local` 访问入口；`curl --resolve` 返回 `liteyuki browser loop ok`。
- [x] 收紧运行集群和环境删除关系：运行集群被环境引用时禁止删除；环境被部署配置、访问入口或发布记录引用时禁止删除，避免父资源删除后子链路不可维护。
- [x] 删除运行配置集成功后从部署配置中移除对应引用，避免部署配置继续指向已删除配置集。
- [ ] 镜像架构与本地集群提示：镜像部署失败时在发布日志或资源详情中提示 `no matching manifest for linux/arm64` 等架构不匹配原因，并引导改用支持当前集群架构的镜像或平台构建产物。

## 4.2 应用模板市场

目标：提供轻量应用市场 / 应用模板能力，优先覆盖 Redis、PostgreSQL、MySQL、MariaDB、MongoDB、Garage、RabbitMQ、监控、工具和轻量协作应用。用户在项目空间内选择模板，填写应用名称、短名、密码、容量等少量字段后，一键生成应用、部署配置、运行配置、Secret、数据卷和可选访问入口。第一版不做复杂商店和在线安装，模板数量少，使用 JSON 目录即可；后续兼容第三方模板市场时继续沿用同一份 JSON schema。

- [x] 定义应用模板 JSON schema：MVP 包含 `id/slug/name/description/category/icon/popularityWeight/image/version/servicePort/defaultResources/env/secretEnv/configFiles/secretFiles/values`；图标缺失或加载失败时前端 fallback 到 `/app-templates/icons/fallback.svg`。
- [x] 设计模板来源加载策略：内置模板从仓库内 JSON 读取；后续第三方模板市场只需要提供同 schema 的远程 JSON 列表，平台后端负责拉取、校验、缓存和禁用不可信字段。
- [x] 建立模板安全边界：MVP 只暴露后端内置模板；普通用户只能安装已内置模板；模板不允许用户自由写命令、宿主机挂载或特权字段；密钥类输入只能进入 Secret，不回显。
- [x] 实现模板渲染服务：按用户输入渲染镜像、端口、资源规格、数据卷、ConfigMap 和 Secret；所有变量替换统一走后端模板渲染组件，避免业务里散落字符串替换。
- [x] 实现模板安装事务：安装时一次性创建 Application、DeploymentTarget、Secret、数据卷配置和安装记录；任一步失败回滚业务数据，不在失败时留下半个应用。
- [x] 模板安装后自动发布：支持选择是否立即部署；默认创建发布并投递部署任务，安装记录保存安装与部署投递状态。
- [x] 内置 Redis 模板：镜像 `redis:7-alpine`，端口 `6379`，默认 `500m/512Mi/1`，默认数据卷 `/data`。
- [x] 内置 PostgreSQL 模板：镜像 `postgres:16-alpine`，端口 `5432`，默认数据卷 `/var/lib/postgresql/data`，输入 `POSTGRES_DB/POSTGRES_USER/POSTGRES_PASSWORD`。
- [x] 内置 MySQL 模板：镜像 `mysql:8.4`，端口 `3306`，输入数据库名、用户名、root 密码和用户密码，数据卷挂载到 MySQL 数据目录。
- [x] 内置 MariaDB 模板：镜像 `mariadb:11.4`，端口 `3306`，输入数据库名、用户名、root 密码和用户密码，数据卷挂载到 MariaDB 数据目录。
- [x] 内置 Garage 模板：镜像 `dxflrs/garage:v1.1.0`，端口 `3900`，单节点轻量对象存储，敏感配置通过 Secret 文件挂载到 `/etc/garage.toml`。
- [x] 内置 RabbitMQ 模板：镜像 `rabbitmq:3-management-alpine`，输入默认用户和密码；公网管理入口默认关闭。
- [x] 扩充非 PHP 容器应用模板：参考 1Panel 应用商店收录范围，新增 MongoDB、Valkey、Memcached、pgAdmin4、Meilisearch、Grafana、Uptime Kuma、Memos、IT-Tools、Excalidraw、Verdaccio、Docker Registry 和 Bytebase。
- [x] 新增 Prometheus 应用模板：镜像 `prom/prometheus:v3.12.0`，端口 `9090`，默认持久化 `/prometheus`，内置抓取自身 `/metrics` 的最小配置，并在文档中说明 Grafana/Prometheus 暂不自动联动。
- [x] 前端新增“应用市场”入口：支持查看模板、展示图标、描述、完整镜像名称、默认资源和持久化容量；支持分类筛选、按热度权重或名称排序，并支持顺序/倒序；安装时默认预填模板镜像，并允许用户替换为 Harbor、DockerHub 代理或私有镜像地址；安装后的应用默认使用模板图标，应用图标字段兼容内置图标名、站内资源路径和 `http(s)` 图片地址；模板图标缺失或加载失败时展示无图标 fallback。
- [ ] 项目空间内新增“从模板安装”入口：安装弹窗只展示模板 schema 里的必要字段，默认短名按 `{templateSlug}-{随机字符}` 生成，密码支持自动生成和复制。
- [ ] 安装完成页展示连接信息：按模板 outputs 展示内网服务域名、端口和建议环境变量，敏感字段默认隐藏并提供复制按钮。
- [x] 为模板市场补充 i18n：模板市场页面、安装表单、安装状态和模板描述走前端 i18n。

## 5. Git 集成

- [x] 实现 GitProvider 基础模型、迁移和 CRUD API。
- [x] 实现 GitAccount 基础模型、迁移和当前用户 CRUD API。
- [x] 支持 Gitea OAuth API。
- [x] 支持 GitHub OAuth API。
- [x] 实现 RepositoryBinding 基础模型、迁移和项目内 CRUD API。
- [x] 实现 GitProvider / GitAccount OAuth 回调和 token 刷新 API。
- [x] 统一 Git/OIDC OAuth state 运行时表名与迁移表名，并迁移历史 AutoMigrate 错误表数据。
- [x] Git Provider / Git 凭据表单作用域交互与镜像站一致：global/user 不展示具体项目，project 支持选择多个项目空间。
- [x] 统一 scoped 资源项目空间绑定模型：Git Provider、Git 凭据、镜像站、运行集群、BuildProvider 和 BuildVariableSet 的 project scope 使用 `scoped_resource_project_bindings`，不再把单个项目 ID 塞进 `ownerRef`。
- [x] 实现仓库列表、分支、文件读取和 Dockerfile/构建目录探测 API。
- [x] 应用/仓库绑定选择仓库时优先搜索当前 Git 凭据可访问仓库；无匹配且有搜索词时由平台后端搜索公开仓库，并继续复用后端结构探测 API。
- [x] 创建 Git webhook API。
- [x] RepositoryBinding 保存时支持默认自动配置 Git webhook，失败不阻塞保存并记录 failed 状态。
- [x] 提供 RepositoryBinding Webhook 重新配置 API，复用后端 provider 创建逻辑和权限校验。
- [x] 校验 webhook 签名 API。
- [x] 处理 push/tag webhook 事件 API。
- [x] 调整 Webhook 与构建触发边界：Webhook 直接绑定应用仓库，Git 事件统一进入平台；平台按应用下的部署配置触发规则判断要创建哪些 BuildRun/部署任务，部署配置不单独拥有外部 Git webhook。
- [x] RepositoryBinding 列表排除软删除 GitProvider/GitAccount/Application，并展示 Git 凭据 owner 信息。
- [x] 删除 GitAccount 前检查 RepositoryBinding 引用，禁止删除仍被绑定引用的凭据。
- [x] Debug 角色预览状态下禁止触发 Git OAuth 授权，避免真实 session 归属混淆。
- [x] Git 上游接口错误对前端脱敏，不再透传上游响应体。
- [x] Git webhook 创建失败按上游状态和 validation 细节映射友好错误码，明确提示 PUBLIC_BASE_URL 不可公网访问、权限不足、仓库不存在、重复 webhook 和平台限流等原因。
- [ ] Webhook 状态改为实时检测/同步：仓库绑定列表不只展示本地 `webhookStatus`，后端按 Git Provider 适配 GitHub/Gitea 查询当前仓库 Webhook 是否存在、回调 URL 是否匹配、是否启用，并将检测结果回写或作为实时状态返回；前端提供“检测 Webhook”入口，避免用户在上游手动删除后平台仍显示成功。
- [x] Git 外部请求网络失败映射为稳定错误码 `git.network_failed`，提示检查服务端网络、代理/VPN、DNS 解析或 FakeIP 设置。
- [x] 收紧 Git 个人凭据访问：`personal` 凭据仅所有者可见可用，`provider` 凭据才按作用域共享。
- [x] 收紧普通业务列表中的用户空间资源：管理员不再混看他人的 user-scope Git Provider、镜像站和 personal Registry/Git 凭据，后续单独建设管理视图。
- [x] 全局 Git Provider、镜像站、集群对普通用户只返回可用资源摘要，不返回管理员维护的连接配置或密钥明文。
- [x] 将 Git OAuth 凭据自动刷新放入后期 Worker：扫描即将过期的 GitAccount，使用 refresh token 刷新 access token，失败时标记 expired 并记录审计事件。
- [x] 在 Git API 调用前增加兜底刷新：当 access token 已过期或即将过期时同步触发一次刷新，避免用户必须手动点击刷新。
- [x] 删除 Git Provider 时同步软删除其 Git 凭据，并在前端确认弹窗提示级联删除，避免父资源删除后凭据残留且无法维护。

## 5.1 异步任务与 Worker 后期增强

- [x] 以 Redis + Asynq 作为 Go 侧默认异步任务方案，承担类似 Celery 的队列、重试、延迟任务和定时任务能力。
- [x] 引入 Asynq Scheduler/PeriodicTaskManager 管理周期任务，包括 Git 凭据刷新、集群状态同步、证书续期检查、资源清理和失败任务补偿。

## 5.1.1 删除与 Kubernetes 资源最终一致性

开发原则：

- 不追求数据库删除和 Kubernetes 删除的强原子事务；Kubernetes API 不参与数据库事务，平台语义统一定义为“用户删除平台托管对象，平台负责最终清理对应运行态资源”。
- 不引入复杂 Saga 框架或多层补偿 DSL；保持一个资源族一个清理入口、一个幂等 worker、一个可观测状态字段，避免系统越来越难维护。
- 删除操作必须幂等：重复删除不存在的 Kubernetes 资源视为成功；权限、归属和 managed labels 校验失败才视为失败。
- 用户侧不隐藏中间态：删除中、删除失败、残留资源待清理要明确展示，并提供重试或手动清理入口。

待做：

- [x] 统一删除状态字段和状态机：Project/Application/DeploymentTarget/GatewayRoute/ProjectRuntimeConfigSet 等资源使用 `active/deleting/delete_failed/deleted` 或等价最小状态，避免每个 handler 自造删除语义。
- [x] 统一清理任务 payload：按资源族定义 `resource:cleanup`，通过 `resourceType` 区分 `project`、`deployment_target`、`gateway_route`、`runtime_config`，payload 只携带稳定 ID、actor、dedupeKey，具体 Kubernetes 资源名由后端按当前命名规则推导。
- [x] Project 删除采用两阶段：先标记项目空间删除中并阻断新建应用、环境、发布和访问入口；worker 删除该项目 namespace 内平台托管资源或整个 namespace，成功后软删除业务数据，失败则保留 `delete_failed` 和错误摘要。
- [x] 项目空间删除前端防误触：删除时必须输入项目空间名称后才允许确认。
- [x] Application 删除保持两阶段：先阻断新的构建、发布、网关和运行态变更；worker 按应用归属 labels 删除 Deployment/Service/ConfigMap/Secret/HTTPRoute 等托管资源，托管数据卷按数据保留策略处理。
- [x] DeploymentTarget 删除只清理该部署配置派生资源：Deployment、Service、ConfigMap、Secret、PVC 可按保留策略处理；不误删同一应用其他部署配置的资源。
- [x] GatewayRoute 删除不删除 Service，只删除对应 HTTPRoute 等访问入口资源；访问入口删除和部署运行态删除职责分离。
- [x] ProjectRuntimeConfigSet 删除或变更不直接删除正在运行的 ConfigMap/Secret；只更新引用关系并提示受影响部署配置重新部署，真正运行态清理由部署配置发布/清理任务负责。
- [x] 增加资源清理审计和日志：清理任务复用 WorkerTaskEvent 记录开始、成功、失败原因和重试上下文，业务资源保留 `delete_failed` 错误摘要供前端展示。
- [x] 增加周期 reconciler：复用 `sync:status` 周期任务扫描 `deleting/delete_failed` 业务资源并重放幂等清理，避免任务投递或 worker 短暂故障导致资源永久卡住。
- [x] 前端删除体验统一：列表/详情页展示删除中、删除失败并禁用重复操作，删除失败时展示错误摘要。
- [ ] 集群孤儿资源自动发现：周期扫描平台 managed labels 下但业务对象已不存在的 Kubernetes 资源，生成残留资源提示或清理建议；当前阶段仍可通过集群资源页手动清理。

## 5.2 安全审计后续

- [x] 完成二次代码安全审计修复，覆盖后端权限、会话、OAuth、凭据、外部请求和前端调用链。
- [x] 收敛镜像站测试接口错误输出，避免向前端透出底层网络细节。
- [x] 收紧镜像记录创建权限：项目镜像记录要求项目写角色，未归属项目记录仅平台管理员可创建。
- [x] 抽离统一 SSRF/出站访问控制组件，并接入 Git、OIDC 和 Registry 外部请求链路。
- [x] SSRF/egress 组件接入管理员安全配置，支持域名黑名单、域名特许白名单、IP/CIDR 黑白名单和端口规则。
- [x] 修复 egress CIDR 地址族匹配，避免 `::ffff:0:0/96` 误拦截 GitHub 等普通公网 IPv4。
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
- [x] 项目 scope 镜像站支持绑定多个项目空间，项目默认镜像站按绑定项目空间分别保存。
- [x] 实现 RegistryCredential 加密引用。
- [x] 明确 RegistryCredential 隔离：`scope` 表示 pull/push 用途，`accessScope` 表示 personal/跟随镜像站；global 镜像站凭据强制 personal。
- [x] Registry response 不再暴露默认 `credentialRef`，仅返回 `credentialSet`。
- [x] 项目级镜像站管理收紧为 Owner/Admin，Developer 不再能新增、修改或删除项目镜像站。
- [x] 实现 registry 凭据测试。
- [x] 修正镜像站测试接口：按使用权限开放，失败返回结构化结果，并支持 token-only Basic Auth 探测。
- [x] 实现默认镜像站选择优先级。
- [x] 镜像记录支持按镜像站搜索镜像仓库和读取 tag 建议，后端适配 DockerHub、Harbor 与通用 Registry，并加入限数、短缓存和用户级限流。
- [x] 新增通用 OCI / Docker Registry 镜像站类型：保存为 `generic-oci`，使用标准 `/v2/`、`_catalog` 和 `tags/list` 能力，便于接入 GitLab Registry、GHCR、Quay、Nexus、JFrog Artifactory 等兼容 registry。
- [x] 实现 ContainerImage 记录。
- [x] 镜像站“镜像”列表接入后端分页、排序和搜索，前端复用统一分页控件。
- [x] 删除镜像站时同步软删除其凭据，并在前端确认弹窗提示级联删除，避免镜像站删除后凭据残留且无法维护。

## 7. 平台构建

### 7.1 构建 API/CRUD 优先

- [x] 实现 BuildProvider 模型、迁移和 CRUD API。
- [x] 实现 BuildRun 模型、迁移和列表/详情 API。
- [x] 实现 BuildJob 模型、迁移和列表/详情 API。
- [x] 实现手动触发构建 API，先创建 queued 状态的 BuildRun/BuildJob。
- [x] 实现构建触发器配置 API：manual、webhook、push branch、tag、API token。
- [x] 实现构建参数配置 API：Dockerfile 路径、构建上下文、Dockerfile Build Args、目标镜像、目标镜像站凭据、构建目录。
- [x] 构建触发只允许用户填写目标镜像 Tag 模板；目标镜像名前缀由平台按镜像站和应用标识固定生成，DockerHub 不带域名前缀，其他镜像站强制带 registry domain；镜像站不再承载 repository namespace。
- [ ] 支持平台 Dockerfile 模板构建：部署配置可选择“仓库 Dockerfile”或“平台模板”，模板支持 Node/Go/静态站点等预设及运行参数；Kubernetes 构建 Job 克隆仓库后，将平台渲染出的临时 Dockerfile 写入指定构建目录，并按部署配置中的构建上下文执行 BuildKit 构建；前端在仓库探测不到 Dockerfile 或用户主动切换时引导选择模板。
- [x] 部署配置表单维护代码仓库、Dockerfile、构建上下文、目标镜像站、镜像引用模板和构建策略；应用配置只保留名称、标识、图标和服务默认端口，不再维护单一仓库或镜像来源。
- [x] 创建/编辑应用弹窗只保留名称、标识和图标，仓库、镜像、端口、Webhook 等来源与交付入口统一由部署配置维护，避免应用被误解为只能绑定一个仓库。
- [x] 应用详情概览改为看板式运行摘要，展示部署配置、构建、部署和访问入口关键状态；应用基础模型移除来源类型、仓库、镜像和构建字段，交付配置统一归属部署配置和访问配置。
- [x] 项目应用列表移除来源类型和服务端口列，只展示应用基础摘要和操作入口。
- [x] 部署配置表单选择应用下已绑定的 RepositoryBinding，支持就地绑定新仓库并自动选中；Dockerfile、构建上下文和构建路径按选中仓库自动探测，构建提供者不再作为用户表单项展示。
- [x] 修复部署配置表单仓库结构探测断链：选择仓库绑定后重新调用后端 build-options，恢复 Dockerfile、构建上下文和构建目录候选及 Dockerfile 目录联动。
- [x] 自动部署直接按 DeploymentTarget 的 branch/tag pattern 匹配；DeploymentTarget 同时表达构建触发范围和发布到具体环境的策略。
- [x] 应用部署配置表单按配置工作流重排为基础信息、代码来源、构建产物、触发与调度、部署配置和访问入口分区，弹窗正文内部滚动、底部保存操作固定，降低长表单配置负担。
- [x] 部署配置表单分区支持折叠展开；基础信息、代码来源、构建产物、触发与调度、部署配置和访问入口默认展开，部署配置钩子默认收起；部署配置钩子改为按需添加阶段，再在阶段内选择 Hook 并拖拽排序，避免阶段和 Hook 全量铺开。
- [x] 部署配置表单增加智能默认值：选择 Dockerfile 时自动填充构建上下文和构建目录为 Dockerfile 所在目录；目标镜像引用模板在用户未手动编辑前按镜像站命名空间、应用标识和部署配置标识自动生成。
- [x] 部署配置支持显式 Dockerfile Build Args：按 `KEY=value` 配置非密钥 ARG，触发构建时快照到 BuildRun，并和项目空间构建变量一起作为 BuildKit `build-arg` 传入；同名时部署配置 Build Args 覆盖变量集默认值。
- [x] 构建变量和密钥提升为项目空间级资源，在项目工作台集中维护；构建时默认注入项目空间启用的变量和密钥。
- [x] 实现项目空间级构建/部署钩子 MVP：项目 Hook 页面只维护通用脚本库，不维护阶段、启停状态或执行顺序；部署配置把通用 Hook 绑定到构建/部署阶段并在部署配置内拖拽排序；运行时按部署配置绑定快照执行，并支持脚本快照、超时、失败策略、运行记录、日志和审计。
- [x] 扩展部署配置阶段 Hook：支持 `prePull` / `postPull`（仓库拉取前后）、`preBuild` / `postBuild`（镜像构建前后）、`prePush` / `postPush`（镜像推送前后）以及 `preDeployment` / `postDeployment`；部署配置的 Hook 选择与排序 UI、Build Job 阶段调用点、Worker 部署阶段调用点、HookRun 展示和文档同步覆盖这些阶段。
- [x] 补齐部署配置 Hook 消费入口：应用部署配置弹窗可加载项目空间 Hook，显式启用/停用 Hook，按阶段绑定脚本并调整执行顺序，提交前规范化 `DeploymentTargetHookBinding`。
- [x] 部署阶段 Hook Job 增加 TTL 清理：成功 Hook Job/Pod 保留 5 分钟，失败 Hook Job/Pod 保留 24 小时；脚本 ConfigMap 绑定 Job ownerReference，随 Job TTL GC 清理。
- [x] 收敛构建模板变量命名：同一个值只保留一个变量名，前端说明、后端预览渲染和 Build Job executor 渲染保持一致。
- [x] 部署配置支持同配置并行策略：默认同一部署配置排队，不同部署配置可并行；配置为并行时同一部署配置也允许多个构建任务同时运行。
- [x] Kubernetes 构建 Job 支持 BuildKit registry 镜像层缓存：通过 `BUILD_CACHE_ENABLED` / `BUILD_CACHE_TAG` 统一控制，启用后按目标镜像同仓库默认推送 `:buildcache`，executor 自动 import/export cache。
- [ ] 构建高级参数后续补齐：Dockerfile target stage、目标平台 platform、多镜像 tag、BuildKit secret mount、BuildKit SSH mount、cache import/export 细粒度策略和构建网络策略可视化，保持在高级折叠区渐进开放。
- [x] 新增 BuildVariableSet 构建变量和密钥模型、迁移和 CRUD API，支持 global/project/user 作用域。
- [x] BuildRun 默认注入项目空间启用的变量和密钥，Worker 调度构建 Job 前按权限解析变量并从后端 Secret Store 解析密钥。
- [x] 为 BuildRun 预留 cache 配置字段，MVP 先不启用缓存。
- [x] 记录 image tag、digest、source commit 和构建产物归属。
- [x] 记录 CPU、内存和 credit 消耗字段，计费系统先不实现。

### 7.1 计费系统 MVP

- [x] 计费主体统一为用户钱包：项目空间记录当前计费归属人，构建、运行、访问、存储等消耗在结算时扣到当时的归属人；项目空间转移后只影响新费用，历史流水不迁移。
- [x] 全平台只使用 `credits` 作为内部货币，底层金额使用高精度 decimal 存储并支持小数，禁止使用 `float32/float64` 表示金额；展示单位由站点管理员在站点设置中配置，平台本体不内置支付渠道。
- [x] 费用相关写操作必须使用数据库事务保证原子性：账本流水、钱包余额、用量结算状态和补偿/退款流水必须在同一个事务内完成，任一步失败都整体回滚。
- [x] 建立 append-only 账本：所有扣费、充值、赠送、补偿和退款都写入不可变流水，历史流水不修改；修正通过反向流水完成。
- [x] 建立用量记录与账本分离模型：先记录原始用量，再按当时的计费规则快照生成 credits 流水，避免后续调价影响历史账单。
- [ ] 计费采用事件 + 周期采样 + 批量聚合：构建完成按事件结算（已接入），容器运行和存储按分钟采样并按小时聚合，访问次数按时间窗口聚合后再生成账本流水，不在请求路径逐次扣费。
- [x] 构建 Job 计费：无论成功、失败、取消还是超时，只要 Kubernetes Job 已实际开始运行就按资源占用计费；排队未开始不计费，平台内部错误可生成补偿流水。
- [x] 构建 Job 粗略计量系数先按 decimal 估算：`1 vCPU-minute = 10 credits`，`1 GiB-memory-minute = 2 credits`，最小计费粒度 1 分钟；最终系数不写死，放入站点配置或计费规则表。
- [x] 容器运行计费：按项目空间内已发布部署配置的 `replicas * cpu_request * duration`、`replicas * memory_request * duration` 累计 credits；数据卷容量和访问次数独立实现。
- [x] 容器运行粗略计量系数先按 decimal 估算：`1 vCPU-hour = 30 credits`，`1 GiB-memory-hour = 6 credits`；MVP 按整点小时窗口聚合并对首个窗口按发布时间截断。
- [x] 访问计费改为按平台访问入口的响应出站流量聚合：新增 `gateway.egress_gib` meter 和网关用量上报接口；请求次数 `gateway.requests_1000` 默认关闭并保留为观测/防滥用扩展。
- [x] 存储计费按声明容量计费，不按实际使用量计费；数据卷保留期间持续计费。导出数据卷作为一次性操作账单项后续再补。
- [x] 站点管理员可配置计费规则启停、credits 单位展示名、免费额度、计费归属人欠费宽限期和是否允许欠费继续运行。
- [x] 用户侧账单与环境选择页面使用 `billing.creditsDisplayName` 展示货币单位，并在运行环境、构建环境表单中按当前计费规则前端估算单价。
- [x] 计费原子粒度统一为部署级：CPU、内存、存储按部署配置窗口结算，网关按路由窗口结算，构建按 BuildRun 结算；账单分析按项目空间 / 应用 / 部署配置聚合，应用只作为展示汇总层，不再保留应用级费用组兼容。
- [x] 站点管理员可在账单页为指定用户账户写入充值和补偿流水；余额更新和账本记录必须在同一个事务内完成。
- [x] 对外提供幂等充值 API：允许受信任第三方支付/运营系统按用户账户写入充值、补偿或扣减流水，支持按用户钱包和幂等键防止重复入账；平台本体不对接支付宝、微信、Stripe 等支付渠道。
- [x] 余额不足风控：按站点开关禁止新构建、新发布、回滚发布和部署配置变更；运行中资源先进入欠费/宽限状态，不默认立即删除数据，避免误伤用户业务。
- [x] 前端提供账单视图：展示用户余额、今日花费、本月花费、近期待扣、构建/运行/访问/存储分类消耗、低余额提示、账本流水和用量记录，并支持按项目空间筛选费用来源。
- [x] 账单视图支持周期筛选：预设本周、近 7 天、本月、近 30 天、本年、去年，并支持自定义日期范围；费用分析、账单流水、用量记录和周期分类消耗按所选周期过滤。
- [x] 平台管理员账单视图支持按用户账户切换查看；默认优先显示管理员自己的账单，手动选择全部用户或指定用户后，余额、费用分析、账单流水、用量记录和项目空间筛选同步切换到对应范围。
- [x] 项目空间概览展示计费归属账户，包含头像、名称和邮箱，便于确认当前项目空间后续扣费关联到哪个用户钱包。

### 7.2 平台系统项目空间与集群探针

- [x] 设计平台系统项目空间模型：新增或约定 `platform-system` 系统项目空间，用于承载平台自有探针、采集器和诊断组件；仅平台管理员可见，不参与普通用户账单，不允许普通用户删除或修改；平台管理员可以像维护普通项目空间一样进入查看应用、部署、发布和运行态。
- [x] 将 Gateway Traffic Probe 从“特殊系统组件安装”迁移为平台系统项目空间下的普通应用：创建 Application、DeploymentTarget、Release 和运行态资源，复用正常部署、日志、Web Console、资源列表和审计链路；系统组件安装记录仅作为应用市场安装入口、状态索引或兼容迁移层。
- [x] 为平台系统项目空间增加保护规则：禁止删除平台系统项目空间；探针相关运行费用不进入普通用户账单，系统组件安装记录仅保留安装索引，最近 heartbeat / 上报窗口改为 Redis 或进程内短 TTL 运行态。
- [ ] 为平台组件实例增加卸载/重装二次确认：平台管理员删除或重装探针应用/部署配置时需要明确确认，并同步清理或更新系统组件安装记录。
- [x] 为系统组件资源建立统一标记：平台自有集群组件写入 `liteyuki.devops/system=true`、组件类型、运行集群 ID 和版本等 labels/annotations，便于集群资源页过滤、审计和后续升级。
- [x] 设计系统组件部署 API/Worker：由平台按运行集群下发或升级探针组件，支持版本记录、部署状态回写和失败原因展示；组件部署失败不能影响用户业务发布。
- [x] 网络流量计费探针改为可选平台组件：默认不安装；账单页在没有可用探针时显示“访问流量不可用”，平台管理员可直接跳转应用市场安装 Gateway Traffic Probe。
- [x] 应用市场支持平台组件模板安装：新增系统组件安装记录、独立上报 Token，安装到平台系统项目空间并复用普通应用部署链路；Worker 仅额外确保探针所需 Kubernetes RBAC。
- [x] 实现网关流量采集器 MVP：优先采集平台创建的 GatewayRoute/HTTPRoute 对应的网关访问日志或 metrics，按 `routeId + windowStart + windowEnd` 聚合响应出站字节、请求数和状态码摘要。
- [x] 实现探针到平台的受限上报链路：采集器使用系统组件 Bearer Token 调用平台网关用量上报 API，后端按探针所属运行集群校验 GatewayRoute，避免跨集群伪造用量；窗口幂等继续复用账单服务去重。
- [x] 将访问计费接入采集器聚合结果：默认按响应出站流量 `gateway.egress_gib` 结算，请求次数只作为观测和防滥用字段保留；集群内服务互访、镜像拉取和数据库访问不计入公网访问费用。
- [ ] 前端增加系统组件状态视图：在运行集群详情或集群资源页展示探针安装状态、最近上报时间、采集窗口延迟和错误摘要，方便诊断“为什么流量账单没更新”。
- [ ] 补充平台系统项目空间验收：创建运行集群后可部署探针；创建访问入口并产生请求后能看到用量记录和账本流水；禁用探针后不再产生新的访问用量但不影响应用访问。

### 7.3 Worker + Kubernetes Job 构建执行链路

- [x] 构建执行链路收敛为 Worker 调度部署集群 Kubernetes Job，移除常驻 Builder Agent 和宿主机 Docker socket 主路径。
- [x] API 创建 queued 状态的 BuildRun / BuildJob 后投递 `build:run` 异步任务，由 Worker 负责调度和回写。
- [x] Worker 按 DeploymentTarget 所属环境解析运行集群，在项目命名空间创建一次性 Kubernetes Job。
- [x] 部署配置拆分运行环境与构建环境；构建环境改为选择项目空间已有环境，构建触发时按环境 CPU/内存写入 BuildRun 快照，手动触发构建默认继承部署配置并可临时切换。
- [x] Worker 通过一次性 Kubernetes Secret 注入 Git、registry 凭据和构建变量，并作为 BuildKit build-arg 传入。
- [x] Worker 实时采集 Pod 日志并写入 BuildLog，同时解析 Hook 控制行和 BuildKit 阶段进度。
- [x] Worker 回写构建状态、镜像引用、digest、source commit，并按自动部署策略创建 Release。
- [x] 前端构建页新增构建变量和密钥管理、触发构建变量和密钥选择、任务日志 Dialog 和列表自动刷新。

### 7.4 构建安全与网络策略

- [x] 构建主路径恢复 Kubernetes Build Job 执行链路。
- [x] 为 Kubernetes 构建 Job 增加独立出站访问控制策略。
- [x] 支持公开 Git、公开 registry、公开包管理源访问。
- [x] 支持内网 registry/镜像源 TCP 443 白名单访问。
- [x] 禁止私有网段非 443 端口访问。
- [x] 禁止元数据地址、Kubernetes API Server 和 Service CIDR 访问。
- [x] 为构建网络拒绝事件记录审计日志。
- [ ] Web Console 增加项目/部署配置级开关：真实终端默认面向可信开发者，公开 RC 前需要能按项目或部署配置关闭，并在权限说明中明确审计范围。
- [ ] 细化 Access Token scope：将应用、部署配置、发布、构建和网关接口从粗粒度 `project:read/write` 拆到稳定业务 scope，避免自动化授权语义模糊。
- [x] 构建日志和 Hook 日志统一脱敏验收：Git Token、Registry 密码/Token、构建密钥、常见 Authorization header 和 URL 内 token 不应落入 BuildLog/HookRunLog。
- [x] Kubernetes 构建 Job 默认不使用 privileged，不挂载宿主机 Docker socket；rootless BuildKit executor 使用非 root UID/GID、`--oci-worker-no-process-sandbox`、允许 `newuidmap/newgidmap` 所需的 privilege escalation，并放开 seccomp/AppArmor 兼容 Kubernetes 用户命名空间限制。
- [x] 构建变量集拆分“可使用”和“可查看明文”权限：列表仍展示可用配置摘要和变量数量，但普通项目成员不再收到 variables 明文，只有平台管理员、个人所有者或项目空间 Owner/Admin 可查看和编辑。
- [x] 目标镜像默认模板在镜像站 namespace 为空时 fallback 到当前项目空间 slug，避免生成裸仓库名。
- [x] 运行时 exec 审计收敛验收：AuditLog 只记录命令摘要、长度、容器和退出码，不记录原始命令文本。

### 7.5 Kubernetes 构建 Job 详细排期

#### 7.4.1 API 与队列

- [x] API 创建 BuildRun / BuildJob 后保存在数据库构建队列。
- [x] 移除常驻 Builder claim 主路径，恢复 Asynq `build:run` 任务作为构建调度入口。
- [x] BuildJob 记录 Kubernetes Job executor 标识、logRef、运行心跳和超时兜底。
- [x] BuildRun 创建时补充应用、仓库绑定、目标镜像站和凭据权限的强校验，避免 Worker 阶段才发现无权限。
- [x] BuildRun 支持 cancel 请求和 canceled 状态，Worker 发现取消后删除对应 Kubernetes Job。

#### 7.4.2 Worker Build Controller

- [x] 移除 `cmd/builder` 独立入口，Worker 直接处理构建任务。
- [x] 清理旧 Builder 本地配置残留：删除本地 `.env.builder`、旧 `.local/builder-workspace` 和空壳 `internal/api/builder_handlers.go`。
- [x] Worker 创建 Kubernetes Job 后实时同步 queued/running/succeeded/failed/canceled 状态。
- [x] Worker 定时扫描运行中 BuildJob 的超时状态，避免 executor 或 Pod 异常后长期卡在 running。
- [x] Worker 增加并发控制：项目空间与运行集群两级构建并发额度，默认项目空间 2、运行集群 4，超额构建保持 queued 并短延迟重试。
- [x] Worker 增加超时处理：超过任务超时时间后删除 Job 并标记 failed。
- [x] Worker 构建失败重试策略由平台任务队列控制 attempts 和重试窗口，默认避免重复推送过多次。

#### 7.4.3 Executor Image / Job 执行

- [x] 默认使用 BuildKit rootless 构建镜像。
- [x] Worker 支持 Kubernetes Job executor：每次构建在部署集群里启动独立 Pod。
- [x] Git clone 增加浅克隆和重试，BuildKit 构建增加整体重试，缓解 GitHub/DockerHub 网络 EOF。
- [x] 修正 DockerHub 镜像引用和认证地址：镜像使用 `docker.io/...`，auth config 使用 DockerHub 兼容 key。
- [x] Worker 增加 `BUILD_NPM_REGISTRY` 配置入口，默认值下沉到代码，compose 只保留必要连接和构建 Job 配置。
- [x] 构建通信收敛为 Worker 内部调度：API 维护构建记录并投递队列，Worker 直接写回日志、进度和结果。
- [x] 移除构建器注册、Builder Token 和 Agent scope 主路径，构建隔离由项目空间、部署配置环境和运行集群权限决定。
- [x] 构建运行支持重试，应用构建列表行操作菜单提供重试和右侧日志/日志流侧栏。
- [ ] 制作平台自有 executor 镜像，内置 git、ca-certificates、buildctl、shell、jq、基础诊断工具。
- [ ] 新增 Build Job Profile：支持配置 executor image、CPU/内存/超时/并发、适用项目范围和能力标签。
- [x] Executor image 可配置，默认推荐 BuildKit rootless，生产环境支持使用本地或内网镜像站中的 BuildKit 镜像。
- [x] 删除构建变量集时清理部署配置中的引用并删除 scoped 项目绑定，避免部署配置保留不可用变量集 ID。
- [x] Kubernetes Build Job 按官方 rootless BuildKit 兼容方式启动：设置 `BUILDKITD_FLAGS=--oci-worker-no-process-sandbox`，并为 executor 容器放开 seccomp/AppArmor 与 privilege escalation，但仍保持非 privileged、无 ServiceAccount token。
- [x] Build Job 明确入口脚本：clone、checkout、registry login、buildctl build、push、输出 result。
- [x] Build Job 输出结构化 `result.json`，并通过日志 marker 回传 `imageRef`、`imageDigest`、`sourceCommit` 等结果。
- [x] Build Job 不持有平台 API token，不直接回调 API；Worker 通过 Kubernetes API 采集日志和结果。
- [x] Build Job 名称统一为 `build-{buildJobId}`，方便集群内排查和日志关联。
- [x] 构建结束由 Worker 根据 Kubernetes Job 状态回写结果；若 Job/Pod 丢失或超时，Worker 回写失败原因。

#### 7.4.4 隔离与安全

- [x] Build Job 不挂载宿主机 Docker socket。
- [x] Build Job 默认不使用 privileged。
- [x] 每个 BuildRun 使用独立 Kubernetes Secret 注入 Git/Registry 凭据。
- [x] build namespace 应用 restricted BuildNetworkPolicy。
- [x] Docker Compose 移除 builder 服务，构建只依赖 worker 和部署集群 Kubernetes Job。
- [x] 每个 Build Job 使用受限 ServiceAccount，默认不授予读取集群资源权限。
- [x] Build Job 完成后立即删除临时 Secret，Job/Pod 按 TTL 保留日志窗口后清理。
- [ ] 将构建出口网络拒绝事件接入审计或日志视图。

#### 7.4.5 日志、结果和前端展示

- [x] BuildJob 记录 `logRef`。
- [x] Worker 实时采集 executor stdout/stderr，并按行追加到平台 BuildLog。
- [x] BuildKit build 阶段使用 `--progress=plain` 保留可读实时日志，Worker 文本日志解析继续作为阶段识别兜底。
- [x] 构建运行支持用户主动终止：queued/running 可取消，平台标记 BuildRun/BuildJob 为 `canceled`，Worker 删除当前 Kubernetes Job。
- [x] BuildKit plain 日志解析为结构化进度并由 Worker 直接回写，保留普通日志展示且不依赖 rawjson。
- [x] Hook 控制行不再原样写入普通构建日志；Worker 将 Hook 日志协议转换为 `[phase: name] message` 可读文本，同时继续写入独立 HookRunLog。
- [x] 修复 Kubernetes Build Job 日志跟随过早失败：等待 executor 容器进入 running/terminated 后再读取日志，避免 `ContainerCreating` 被误判失败并提前删除临时 Secret。
- [x] 修复 Kubernetes Build Job executor Secret 投影权限问题：脚本文件对非 root 容器可读，并且启动命令只复制显式脚本和 hooks，避免递归复制 `..data/..timestamp` 内部目录导致 Permission denied。
- [x] 前端构建状态补充 `lost` / `timeout` 展示和筛选，失联与超时按需要关注状态统计。
- [x] 增加 BuildLog 适配，避免只依赖 Kubernetes Pod 日志保留。
- [ ] 后续增加日志对象存储适配，用于大日志归档、检索和下载。
- [ ] 前端构建详情页展示实时日志、状态流转、镜像引用和 digest。
- [ ] 构建列表增加手动刷新和状态过期提示，避免 BuildRun 已完成但前端仍显示 queued。
- [ ] 构建成功后自动创建 ContainerImage，并与 BuildRun、Application、commit 关联。
- [x] 发布页选择 BuildRun 时优先展示 succeeded 且存在镜像产物的构建记录。
- [x] 发布页选择构建产物时按 imageRef 去重，同一 tag 只展示最新构建，并显示镜像 digest 摘要。
- [x] 部署 Worker 增加 ReleaseLog，前端发布列表和应用部署页支持查看部署日志。
- [x] 部署状态同步接入 `sync:status` 周期任务：对最近 pending/running/succeeded Release 反查 Kubernetes Deployment，资源缺失时标记 failed 并追加运行态漂移日志。
- [ ] 支持构建成功后按应用环境策略自动创建 Release 并投递部署任务。

#### 7.4.6 后续增强

- [x] 接入 registry cache / BuildKit cache，MVP 由 Worker `BUILD_*` 环境变量统一控制，使用目标镜像同仓库 cache tag 作为共享缓存。
- [ ] BuildKit 支持 registry mirror / pull-through cache 配置，优先支持 DockerHub、GHCR 和平台内网镜像站。
- [ ] 语言依赖工具链按生态注入镜像源，可选是否注入环境变量或配置文件：npm/pnpm/yarn、pip/poetry、GOPROXY、Maven settings、Gradle init、Cargo config 等。
- [ ] 镜像源凭据通过 BuildKit secret 或一次性文件注入，禁止进入最终镜像层。
- [ ] 支持远程 buildkitd pool，用于高并发或需要共享缓存的场景。
- [ ] 支持构建资源消耗统计，回写 CPU core seconds、memory MB seconds 和 creditCost。
- [ ] 支持构建队列优先级和项目级限额策略。
- [ ] 保留 External CI Provider 作为后期扩展，不作为 MVP 主路径。

## 8. 集群与部署

### 8.1 集群与部署 API/CRUD 优先

- [x] 部署信息架构拆分：集群页只管理平台运行能力，项目空间管理环境，应用详情管理部署意图和发布入口，用户侧不直接暴露 Deployment/Service/HTTPRoute/ConfigMap 等 Kubernetes 资源名。
- [x] 项目空间新增“环境”页签，支持创建/编辑/删除环境，环境引用可用集群并配置副本数和资源规格；部署统一使用项目空间命名空间。
- [x] 创建项目空间时自动附带默认 `prod` 环境：1 副本、`500m` CPU、`512Mi` 内存，降低首次部署前置配置成本。
- [x] 环境资源规格输入改为数值和单位同胶囊分栏选择，数值区只允许数字，CPU 支持 `m`/核，内存支持 `Mi`/`Gi`，避免用户手写 Kubernetes quantity。
- [x] 抽象带单位数值输入组件，环境 CPU/内存和部署配置数据容量统一使用“数值 + 单位”胶囊输入。
- [x] 优化带单位数值输入组件默认宽度，至少容纳四五位数字和单位，避免数据容量等字段输入区过窄。
- [x] 运行配置支持项目空间级复用配置集，并支持普通配置、配置文件、密钥配置和密钥文件；部署配置可引用公共配置集并覆盖同名键或同路径文件。
- [x] 运行配置引用支持 live/snapshot 策略：跟随引用的部署配置在公共配置更新后提示重新部署；使用快照的部署配置保存时冻结公共配置内容，后续公共配置更新不影响该部署配置。
- [x] 应用详情新增“部署”页签，展示当前应用在各环境的发布状态，并支持从成功构建产物一键发布到环境。
- [x] 部署配置编辑检测运行态字段变更，提示运行中副本需重新部署后生效，并提供“保存并重新部署”入口复用上一条发布镜像创建新 Release。
- [x] 发布列表补充应用、环境、revision、rollout message、开始/结束时间和失败原因展示，回滚仅对成功发布开放。
- [x] 后端 Release 创建增加应用/环境/构建产物归属与状态校验，避免部署到错误项目、错误应用或未成功构建产物。
- [ ] 部署错误输出友好化：Kubernetes apply/rollout 错误返回稳定错误码，前端按 i18n 展示，避免直接暴露底层异常。
- [x] 实现 RuntimeCluster 模型、迁移和 CRUD API。
- [x] 支持设置默认集群。
- [x] RuntimeCluster 支持 global/project/user scope，project scope 支持多个项目空间绑定；只有 global 集群允许设为默认集群，列表按当前用户可访问范围返回。
- [x] 收紧运行集群使用授权：创建/更新部署配置、环境和模板安装时统一校验集群 scope、项目空间绑定、用户归属和平台管理员旁路，避免通过构造 `clusterId` 越权部署或读取运行态资源。
- [x] 实现 kubeconfig 保存、替换和测试连接 API：仅创建者本人或平台管理员可以替换 kubeconfig，接口不回显 kubeconfig 明文。
- [x] 运行集群测试改为真实 Kubernetes API Server `/version` 连通性检测；无 kubeconfig、无效 kubeconfig 或网络不可达时写入失败状态并返回错误。
- [x] 集群前端接入改为 kubeconfig-only YAML 代码框，普通用户列表不展示无权维护集群的 endpoint 配置。
- [x] 约束 kubeconfig 代码框宽度，长行只在编辑器内部滚动，不撑大 Dialog 或表单布局。
- [x] 修复集群 kubeconfig 代码框超长行反撑表单宽度的问题，CodeMirror 顶层固定宽度，横向滚动限制在编辑器内部。
- [x] 引入公共 `CodeEditor` 代码编辑框组件，集群 kubeconfig 使用 YAML 高亮、等宽字体和行号。
- [x] 集群创建/编辑前端支持多 context kubeconfig 选择：粘贴多 context 配置时必须选择一个 context，提交前只保留该 context 及其引用的 cluster/user。
- [x] Kubernetes 和 K3s 接入选项合并为 Kubernetes / K3s，后端兼容旧 k3s 输入并归一到 kubernetes。
- [ ] 设计 Docker 运行时接入模型：支持 Docker host、Unix socket、TCP TLS CA/cert/key、连接测试、权限边界和部署适配，不复用 kubeconfig 字段。
- [x] 实现早期 Environment 模型、迁移和 CRUD API；后续已将用户侧运行/构建规格收敛到 DeploymentTarget。
- [x] 实现 Release 模型、迁移和列表/详情 API。
- [x] 实现部署配置 API：镜像来源、环境变量、ConfigMap/Secret 引用、资源规格、副本数。
- [x] 实现手动部署 API，先创建 pending 状态 Release。
- [x] 发布表单以 BuildRun 为主输入，选择构建记录后自动带出应用和镜像；部署分支以 BuildRun 的 sourceBranch 为准。
- [x] 实现回滚请求 API，先创建 rollback 类型 Release。

### 8.2 部署 Worker/执行链路

- [x] 部署时自动创建 Project namespace；当前阶段固定一个项目空间一个 Kubernetes namespace。
- [x] 部署 Worker 对引用本地证书文件的 kubeconfig 返回友好错误，提示重新保存已内联证书的 kubeconfig。
- [x] 本地 minikube 联调统一预留 `dev.minikube.local` 域名，compose 容器内解析到宿主机网关，kubeconfig 使用 flatten 后的内联证书。
- [x] 实现 Deployment/Service/ConfigMap/Secret apply。
- [x] P4 多工作负载主路径：部署配置增加 `workloadType`，支持 `Deployment` / `StatefulSet`；发布渲染、HPA targetRef、rollout 状态同步、重启、删除清理和集群资源聚合均按实际工作负载处理。普通用户默认仍使用 Deployment，StatefulSet 放在高级配置中按需启用。
- [x] 简化应用侧存储体验：部署配置只暴露运行数据保留、多个容器内数据卷、容量调整和数据导出，底层 PVC 由平台托管且不向普通用户展示。
- [x] 渐进式开放 Kubernetes P0/P1 高级运行时配置：部署配置基础表单继续只展示镜像、端口、副本、资源和数据卷；高级配置默认折叠，已支持 `command/args`、`imagePullPolicy`、`readinessProbe`、`livenessProbe`、`startupProbe`。
- [x] 渐进式开放容器与 Pod 安全上下文：部署配置高级区支持 `runAsUser`、`runAsGroup`、`fsGroup`、`fsGroupChangePolicy`、`readOnlyRootFilesystem`、`allowPrivilegeEscalation`、`capabilities` 等常用字段；默认不启用，后端做基础格式校验，用于解决 OpenList 等镜像的数据目录权限问题。
- [x] 渐进式开放调度配置：高级区支持 `nodeSelector`、`tolerations`、基础 `affinity`、`topologySpreadConstraints` 和 `priorityClassName`；普通用户优先使用平台默认调度。
- [x] 渐进式开放运行态资源 limit：运行态资源从仅支持 request 扩展到 request/limit 分离；默认仍不设置 limit，避免用户必须理解 limits 和峰值。
- [x] 渐进式开放 Service 与 PVC P0/P1 高级参数：Service 默认继续使用 ClusterIP，多端口保持简单；高级区支持 `service.type`、annotations、`sessionAffinity`、`externalTrafficPolicy`，PVC 支持 `storageClassName` 和 `accessModes`。
- [x] 继续渐进式开放 P2/P3 Kubernetes 能力：支持容器 `lifecycle`、HPA/自动伸缩、Service `appProtocol`、PVC `volumeMode`、existingClaim、emptyDir 临时卷、initContainer 和 sidecar。
- [x] 渐进式开放初始化与多容器能力：先支持 initContainer 用于权限初始化、数据库迁移和等待依赖，同时支持 sidecar 辅助容器；表单层不暴露完整 Pod YAML，而是使用受控 Container JSON 数组并由后端裁剪危险字段。
- [x] P4 自动伸缩高级行为：部署配置支持 HPA `behavior` JSON，后端校验 `autoscaling/v2 HorizontalPodAutoscalerBehavior`，Worker 下发到 HPA，用于控制扩缩容速度和稳定窗口。
- [ ] P5 继续评估更完整的高级编排：DaemonSet 是否仅作为平台探针/集群插件能力提供、原生多主容器可视化编辑、容器级独立配置引用、HPA custom metrics、自定义 metrics provider 接入和更严格的策略权限。
- [x] 部署配置运行配置区支持就地新建和编辑项目空间公共配置；新建后自动加入当前部署配置选择，编辑公共配置后后端返回受影响部署配置数量，前端提示需重新部署并支持对当前应用受影响资源一键重新部署。
- [x] 修复部署配置与公共配置编辑弹窗保存按钮被 React Hook Form 初始校验状态误禁用的问题；保存按钮只按文件路径校验和提交状态禁用，必填字段仍由表单提交校验兜底。
- [x] 修复应用部署 tab 部署配置列表操作列被横向溢出挤出可视区域的问题；三点菜单固定在列表右侧，编辑入口始终可见。
- [x] 发布日志弹窗的“部署日志 / 运行日志”切换复用统一 `SegmentedTabsList + Tabs` 组件，避免手写按钮风格和页面 tab 不一致。
- [x] 应用部署 tab 展示 Kubernetes 运行状态：按部署配置聚合对应 Deployment/Pod，映射 Running、Pending、CrashLoopBackOff、ImagePullBackOff、NotReady 等 Pod 状态并在 tooltip 展示摘要。
- [x] 应用部署 tab 将平台 Release 流程状态列命名为“发布状态”，与 Kubernetes “运行状态”区分。
- [x] 修复应用部署 tab 运行状态轮询闪烁：后台 refetch 保留上一轮 Kubernetes 状态，只有首屏无数据时显示检查中。
- [x] 应用部署 tab 行菜单支持重启当前部署配置对应 Kubernetes Deployment；该操作只触发 rollout restart，不新建 Release，并写入审计日志。
- [x] 修复 Release 同步/部署缺失 DeploymentTarget 时退化查询 `dplt` 的问题；发布执行会明确失败为部署配置缺失，状态同步会跳过异常历史记录，避免误报 `deployment_missing .../dplt not found`。
- [x] 应用部署 tab 展示当前命名空间内互相访问的 Service DNS，按部署配置从真实 Service 资源生成短名和完整 `svc.cluster.local` 域名并支持复制。
- [x] 构建日志、发布日志和运行日志默认打开时滚动到底部；仅当用户停留在底部时新日志自动跟随，向上查看历史时不强制回底。
- [x] 修复部署配置数据卷表单输入被打断：数据卷行使用稳定 UI key，避免每次输入后重新 parse 生成随机 id 导致输入框重挂载。
- [x] 应用删除改为异步清理流程：先进入删除中并阻断新的构建、发布和运行态变更，Worker 清理平台托管的 Kubernetes 运行资源后再软删除应用，托管数据卷默认保留，失败进入删除失败状态。
- [x] 应用删除前端防误触：项目空间应用列表删除时必须输入“项目空间/应用”完整名称后才允许确认。
- [x] 实现 rollout 状态等待。
- [x] 实现 Release 状态回写。
- [x] 实现回滚到上一成功版本。
- [x] Web Console 升级为 `xterm.js + WebSocket + Kubernetes exec TTY` 真实交互式终端，后端负责权限、审计和 Kubernetes 连接。

### 8.3 集群资源管理

- [x] 明确集群资源管理边界：只展示和管理 Liteyuki DevOps 创建或显式打平台标签的 Kubernetes 资源，不接管已有集群中的第三方/历史资源。
- [x] 统一平台写入 Kubernetes 资源的 labels/annotations：`app.kubernetes.io/managed-by=liteyuki-devops`、项目空间、应用、环境、发布和网关路由引用，作为后续实时查询和权限映射依据。
- [x] 设计后端 Kubernetes 资源聚合 DTO：Namespace、Workload、Pod、Service、HTTPRoute、Gateway、ConfigMap、Secret、PVC/Event 等只返回前端需要的摘要、状态、归属引用和时间，不返回 Secret 明文。
- [x] 新增后端集群资源 provider/list 接口：按 RuntimeCluster kubeconfig 实时请求 Kubernetes API，支持 namespace、resourceType、projectId、applicationId、environmentId 过滤，并默认只查平台自有资源。
- [x] 新增 Kubernetes 事件读取接口：按平台资源归属查询相关 Events，用于排查部署、网关和证书问题。
- [x] 实现集群资源权限校验：可访问集群 + 可访问归属项目空间才允许查看；普通用户不得查看无归属或非平台自有资源。
- [x] 前端“集群资源”页签接入真实列表：Namespaces、Workloads、Services、Configs、Storage 使用统一 `DataList` 展示，默认展示平台自有资源摘要。
- [x] 集群资源列表接入后端分页和稳定排序：后端按过滤后的可见资源返回 `items/page/pageSize/total/totalPages`，前端所有资源 tab 复用统一分页栏。
- [x] 收紧集群资源删除权限：删除前先读取 Kubernetes labels 并校验资源归属项目空间，前端仅对可管理归属资源展示删除入口。
- [x] 集群资源各类型列表的名称、命名空间、摘要和归属长文本改为单行省略，hover tooltip 展示完整值并提供复制入口，避免长 Kubernetes 资源名撑宽表格。
- [x] 集群资源列表按资源类型裁剪列：命名空间页隐藏命名空间列并前置归属列，工作负载、服务与入口、配置与密钥、存储页均展示归属。
- [x] 集群资源各类型列表支持批量选择可删除资源，批量删除入口提升到 `ContentTabs.tools` 当前页签工具区，并通过二次确认批量删除；每个资源仍逐条走后端归属和权限校验。
- [x] 集群资源支持查看只读 Kubernetes YAML：后端实时读取平台 managed 对象并校验资源归属，前端使用 YAML CodeEditor 展示；Secret 值必须脱敏。
- [x] `DataList` 支持 sticky 列；集群资源页将右侧操作列固定在可视区，其他信息列保留横向滚动。
- [x] 集群资源页新增资源“更新时间”列，后端基于 Kubernetes managedFields 最新写入时间并 fallback 到创建时间，前端复用智能相对时间格式展示。
- [x] 集群资源工作负载页按 Deployment 聚合展示，Pod 作为 Deployment 子行展开查看，Pod 子行不参与顶层分页。
- [ ] 前端提供集群、命名空间、项目空间、应用筛选和手动刷新；空状态说明“只展示平台管理资源”。
- [x] 集群资源归属展示补齐部署配置名称：Workload、Pod、Service、HTTPRoute、ConfigMap、Secret 当前只显示“项目空间 / 应用”，应显示“项目空间 / 应用 / 部署配置”，避免用户无法判断同一应用多部署配置资源来源。
- [ ] 资源详情抽屉展示 labels/annotations 摘要、状态条件、关联业务对象和 Events，不展示 Secret data。
- [ ] 集群资源管理 MVP 验收：在测试集群发布一次应用后，集群资源页能看到对应 Namespace、Deployment、Pod、Service、HTTPRoute/Gateway，并能查看相关事件；已有未打平台标签资源默认不显示。

## 9. 网关与域名

### 9.1 网关与域名 API/CRUD 优先

- [x] 实现 GatewayRoute 模型、迁移和 CRUD API。
- [x] 实现默认域名生成规则 `{projectSlug}-{appSlug}-{stage}.{rootDomain}`，支持用户填写短前缀自动拼接 root domain，并在冲突时自动追加序号。
- [x] 重构访问/域名模型：GatewayRoute 不再只按应用和环境归属，必须绑定到模块或部署配置（优先 DeploymentTarget，间接确定 DeploymentTarget + Environment + Service），访问页选择明确流量目标，避免多模块应用下域名无法判断应转发到哪个 Service。
- [x] GatewayRoute 增加 DeploymentTarget 绑定字段，使一个应用同环境存在多个部署配置时，HTTPRoute 能明确指向该部署配置对应的 Service。
- [x] 访问域名支持启用/关闭：创建默认启用，关闭时保留域名配置但撤销运行态 HTTPRoute，重新启用后重新下发。
- [x] 实现域名冲突检查 API。
- [x] 支持自定义域名创建和状态管理。
- [x] 域名检查体验优化：已创建路由点击“检查域名”时不应只提示“域名已被占用”，需要区分当前路由自身、其他路由占用、DNS/Gateway 可达性和证书状态。
- [x] 生成 CNAME 目标并返回给前端展示。
- [x] 支持 HTTP-only 访问开关。
- [x] 将默认域名后缀和外层访问协议下沉到运行集群级别：访问入口按部署配置所在集群生成默认域名、短前缀补全、CNAME 目标和控制台访问 URL，支持不同集群使用不同 Gateway 域名。
- [x] 运行集群支持多个可用域名后缀：管理员在集群层维护后缀列表，创建访问入口时按部署配置所属集群单选一个后缀，短域名前缀和默认域名生成均使用该选择。
- [x] 运行集群补齐 Gateway API 默认配置：Gateway Provider、控制器类型、GatewayClass、Gateway 名称/命名空间、外部 TLS 模式、转发头策略、可信代理 CIDR 和默认请求/响应头。
- [x] 访问入口补齐高级 Gateway API 配置：支持 Parent Gateway 覆盖、路径匹配、请求/响应头、URL rewrite、redirect、后端权重和备用域名字段，并按项目/平台管理员收紧高风险 header。
- [x] 破坏性迁移访问入口底层：未发版阶段清空旧 GatewayRoute 数据，Worker 主路径从 Kubernetes Ingress 切换到 Gateway API HTTPRoute，并新增 Gateway API CRD 探测和 HTTPRoute 状态读取。
- [x] 运行集群支持 Gateway listener 端口规则和外层访问端口分离：每个集群配置一个内部 HTTP listener、一个内部 HTTPS listener，以及一个外层访问协议/端口；访问入口按 TLS 终止位置自动选择内部 listener，URL 对 HTTP 80 / HTTPS 443 省略标准端口，非标端口显式展示。
- [x] 访问入口启用前预检后端 Service：创建/更新访问入口和 worker 下发 HTTPRoute 前检查部署配置对应 Service 与端口是否存在，缺失时提示用户重新发布部署配置，不在访问入口链路里自动创建 Service。
- [x] Gateway API HTTPS 证书能力第一阶段：运行集群支持手动 TLS Secret 绑定，管理员配置 Gateway TLS Secret 名称/命名空间后，worker 为 HTTPS listener 生成 `tls.certificateRefs`，用于已有证书或外部同步证书的生产过渡场景。
- [x] Gateway API HTTPS 证书能力第二阶段：接入 cert-manager，运行集群支持配置 `ClusterIssuer`/`Issuer`、稳定证书命名策略和证书命名空间，worker 检测 cert-manager CRD、创建/更新 `Certificate`，读取 Ready 状态并把 Secret 挂到 Gateway listener。
- [x] Gateway API HTTPS 证书能力第三阶段核心：支持运行集群启用 cert-manager DNS-01 wildcard 证书，创建包含根域名和通配符域名的 `Certificate`，并把输出 Secret 挂到 Gateway HTTPS listener，适配不暴露宿主机 80/443、外层网关转发到集群内部 listener 的部署方式。
- [ ] Gateway API HTTPS 证书诊断增强：在诊断中展示 DNS-01 / Certificate / Secret / Gateway certificateRefs 状态。
- [ ] Gateway API HTTPS 证书能力第四阶段：把 cert-manager HTTP-01 作为高级可选能力，明确要求公网 HTTP 入口可达且 Gateway 存在 port 80 listener；校验不满足时给出友好错误，避免误导用户在非 80 内部端口场景使用 HTTP-01。
- [ ] 收紧 Gateway `allowedRoutes`：当前 MVP 使用 `namespaces.from=All` 方便跨命名空间绑定，后续改为项目 namespace label selector。
- [x] 实现证书状态字段：disabled、pending、issued、failed、expired。
- [x] 应用访问列表展示证书运行态：Worker 周期同步 cert-manager Ready 信息、失败原因、到期时间和 Issuer，TLS 列使用状态 Badge 并在悬停时展示详情。

### 9.2 网关 Worker/控制链路

- [x] 创建 Gateway API Gateway 和 HTTPRoute。
- [x] HTTPRoute 下发支持 RequestHeaderModifier、ResponseHeaderModifier、URLRewrite 和 RequestRedirect，按运行集群默认值与路由覆盖值合并生成。
- [x] 校验 DNS CNAME。
- [x] 支持 HTTP Challenge 证书申请。
- [x] 实现证书续期检查和状态回写。

## 10. 前端联调验收

- [x] 实现仓库绑定页，并与 Git 集成基础 CRUD 占位联调。
- [x] 仓库绑定页接入真实 OAuth、仓库列表、分支读取和 webhook 创建状态。
- [x] 仓库绑定页和应用创建/编辑表单增加自动配置 Webhook 开关，默认开启并保留失败后手动重试入口。
- [x] 实现镜像站页，并与 ArtifactRegistry 联调。

### 10.1 前端 CRUD 联调优先

- [x] 实现构建页，并与 BuildProvider、BuildRun、BuildJob CRUD 联调。
- [x] 实现部署配置页，并与 RuntimeCluster、DeploymentTarget、Release CRUD 联调。
- [x] 实现网关域名页，并与 GatewayRoute CRUD 联调。
- [x] 将构建、部署、访问从顶层菜单收敛到应用详情页 `ContentTabs`，并新增独立集群页。
- [x] 侧边栏项目空间支持项目下钻应用快捷入口，应用详情页承载概览、模块、构建、部署和访问。
- [x] 侧边栏具体项目支持独立展开/收起，默认收起，展开后再加载并展示应用快捷入口。
- [x] 使用 Chrome 验收构建、部署、域名 CRUD 页面。

### 10.2 前端执行态联调

- [ ] 构建页接入 BuildRun 实时状态和日志查看。
- [ ] 部署环境页接入 Release 状态、rollout 进度和回滚结果。
- [ ] 网关域名页接入 DNS 校验、HTTPRoute/Gateway、证书申请和续期状态。
- [ ] 使用 Chrome 验收仓库绑定、构建、镜像站、部署、域名完整链路。
- [x] 使用 Chrome 验收镜像直部署链路：Neo Blog 数据库、后端、前端和域名入口已通过浏览器创建并访问成功。

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

### 11.1 超大文件拆分与可维护性收敛

- [x] 拆分 `web/src/pages/applications/ApplicationConfigPage.tsx`：按应用详情 tab 拆为 `overview/`、`builds/`、`deployments/`、`gateway/` 子组件；将表单 defaults/types、镜像引用、仓库 URL、构建运行展示、部署摘要等纯函数迁入 `utils.ts` 或邻近模块；入口文件只保留数据查询、tab 编排和跨 tab ref 调度。验收：单文件降到 400 行以内，各子组件只负责一个 tab，`pnpm --dir web lint` 和 `pnpm --dir web build` 通过。
  - [x] 第一刀低风险拆分：抽出构建日志抽屉 `application-build-log-panel.tsx` 和运行终端 `application-runtime-terminal-panel.tsx`，`ApplicationConfigPage.tsx` 从 3039 行降到 2881 行。
  - [x] 第二刀低风险拆分：抽出构建运行筛选条 `application-build-run-filter-bar.tsx`，`ApplicationConfigPage.tsx` 降到 2800 行。
  - [x] 第三刀构建页拆分：抽出 `application-build-run-row.tsx` 和 `application-builds-panel.tsx`，构建运行展示、筛选、日志抽屉和触发表单不再堆在入口文件。
  - [x] 第四刀部署/访问入口拆分：抽出 `application-deployments-panel.tsx` 和 `application-gateway-panel.tsx`，入口文件只负责应用数据查询、tab 编排和跨 tab ref 调度。
  - [x] 第五刀部署运行态拆分：抽出 `application-deployment-runtime.tsx` 和 `application-deployment-runtime-utils.ts`，运行态状态 badge、内网访问域名和 Kubernetes 资源状态归一逻辑独立维护。
- [x] 拆分 `internal/api/build_handlers.go`：按职责拆为 `build_provider_handlers.go`、`build_variable_handlers.go`、`build_run_handlers.go`、`build_job_log_handlers.go`、`build_validation.go`、`build_variables.go`；BuildRun 创建/重试/取消、BuildJob 日志/SSE、变量和密钥解析分离。验收：handler 文件只做 HTTP 入参、权限和响应，构建校验与变量解析可被单测直接调用，`go test ./...` 通过。
- [x] 拆分 `internal/worker/worker.go`：将调度器/周期任务放入 `scheduler.go`，构建任务租约和状态同步放入 `build_reconciler.go`，发布执行放入 `deploy_runner.go`，网关执行放入 `gateway_runner.go`，部署钩子放入 `deployment_hooks.go`，Kubernetes spec 组装与命名工具放入 `kube_specs.go`。验收：`Runner` 主文件只保留构造、启动和公共依赖，部署/网关/Hook 单元逻辑分别可测试，`go test ./...` 通过。
- [x] 拆分并退场旧 `internal/api/builder_handlers.go` 主路径：Builder 认证与心跳、任务 claim/lease、日志/进度回写等旧 Agent API 已不再注册路由；构建 payload 组装迁入 Worker 可复用运行时，镜像引用和 tag 渲染 helper 继续复用。验收：构建主路径走 `build:run` + Kubernetes Job，`go test ./...` 通过。
- [x] 拆分 `internal/api/deployment_handlers.go`：按运行集群、项目环境、部署配置、发布/回滚、自动部署匹配拆为独立文件；集群 kubeconfig flatten、环境 slug 校验和自动部署匹配规则迁入独立 helper/service。验收：每个 handler 文件只处理一个资源族，自动部署匹配规则有单测覆盖，`go test ./...` 通过。
- [x] 后端热点第二轮拆分：继续拆分部署配置、发布运行时和运行集群资源文件；`deployment_target_handlers.go` 只保留部署配置 HTTP 入口，发布运行时日志/exec/terminal 独立到 `release_runtime_handlers.go`，运行集群资源浏览/权限/响应/kubeconfig helper 独立成文件。验收：`go test ./...` 通过。
- [x] Kubernetes provider 热点拆分：`resources.go` 拆出 runtime 日志、exec、terminal 和 metrics 逻辑，`deploy_resources.go` 拆出 Hook Job 逻辑；行为不变，`go test ./internal/provider/kubernetes` 与 `go test ./...` 通过。
- [x] 前端热点页面继续拆分：运行集群页抽出集群资源面板和资源权限 helper，部署面板抽出部署配置编辑弹窗；`ClustersPage.tsx` 和 `application-deployments-panel.tsx` 均降到 1000 行以内，`pnpm --dir web lint/build` 通过。
- [x] 拆分 `web/src/api/client.ts`：已拆为 `web/src/api/core.ts`、`urls.ts`、`types.ts` 和 `web/src/api/domains/*`，入口只组合领域 API；业务页面统一改从 `@/api` 引入。验收：`pnpm --dir web lint`、`pnpm --dir web build` 通过。
- [x] 拆分 `web/src/pages/registries/RegistriesPage.tsx`：已拆出 `registry-form-model.ts`、`registry-list-panels.tsx`、`registry-dialogs.tsx`；主页面保留 tab、查询和 mutation 编排。验收：`pnpm --dir web lint`、`pnpm --dir web build` 通过。
- [x] 拆分 `internal/builder/agent.go` 遗留 executor 工具：旧 `agent.go` 已不存在，现有 builder 已收敛为 `executor/run.sh`、`logs.go`、`payload.go`、`types.go` 等中性模块；无 Docker executor/Agent 生命周期残留。验收：`go test ./...` 通过。
- [x] 拆分 `web/src/pages/code-repositories/CodeRepositoriesPage.tsx`：已拆出 `code-repositories-panels.tsx`、`code-repositories-dialogs.tsx`、`code-repositories-form-model.ts`、`code-repositories-utils.ts`；主页面降到约 300 行，仅保留 tab、查询和 mutation 编排。验收：`pnpm --dir web lint`、`pnpm --dir web build` 通过。
- [x] 拆分 i18n locale 文件：`web/src/i18n/locales/zh-CN.ts` 和 `en-US.ts` 已是 namespace 聚合入口，实际文案位于 `web/src/i18n/locales/<locale>/*`。验收：单个入口文件不超过 400 行，`pnpm --dir web lint`、`pnpm --dir web build` 通过。
- [x] 拆分 Git provider client：已按错误映射、OAuth、类型、仓库/分支/文件/Webhook、构建选项发现等职责拆分 `internal/provider/git/*`；前端仍只调用平台后端 API。验收：`go test ./internal/provider/git` 和 `go test ./...` 通过。
- [x] 拆分大测试文件：`internal/api/handlers_test.go` 已拆为 core/auth/tasks/git-security 等领域测试文件，原文件仅保留 package 声明。验收：`go test ./internal/api` 和 `go test ./...` 通过。

## 12. 可观测性

原则：所有可观测能力默认关闭，只有对应显式开关为 `true` 才启用；metrics 属于本地暴露类能力，`METRICS_ENABLED=true` 后使用 API `:9090`、Worker `:9091` 和 `/metrics` 默认值启动独立 listener。外部上报、查询和跳转类能力必须同时具备真实 endpoint/base URL，未配置时不注册外部导出器、不暴露外部查询入口、不在 UI 展示不可用的 Grafana/Trace/Log 跳转。平台状态仍以数据库业务记录为准，Prometheus、Tempo、Loki 只作为观测与排障数据源。

### 12.1 配置开关与安全边界

- [x] Metrics MVP 配置闭环：新增 `METRICS_ENABLED`、`METRICS_ADDR`、`METRICS_PATH` 配置读取，默认关闭；API/Worker 显式开启后会用进程默认地址启动独立 metrics listener，配置项可覆盖监听地址和路径。
- [x] Metrics MVP 部署闭环：`.env*` 示例、Docker Compose 和 Helm Chart 均补齐 API `:9090` / Worker `:9091` 独立 metrics 端口；Compose 仅 `expose` 容器内端口，Helm metrics Service/ServiceMonitor 默认关闭且不配置 Ingress。
- [ ] 定义可观测配置模型和环境变量读取：`METRICS_ENABLED`、`METRICS_ADDR`、`METRICS_PATH`、`PROMETHEUS_QUERY_ENABLED`、`PROMETHEUS_BASE_URL`、`GRAFANA_LINKS_ENABLED`、`GRAFANA_BASE_URL`、`OTEL_TRACING_ENABLED`、`OTEL_EXPORTER_OTLP_ENDPOINT`、`OTEL_TRACES_SAMPLER`、`STRUCTURED_LOG_ENABLED`、`LOG_EXPORT_ENABLED`、`LOG_EXPORT_OTLP_ENDPOINT`、`LOKI_LINKS_ENABLED`、`LOKI_BASE_URL`、`ALERT_LINKS_ENABLED`、`ALERTMANAGER_BASE_URL`；每项必须有显式开关，metrics 可使用进程默认地址和路径，外部依赖 URL/endpoint 缺失时强制禁用并输出一次启动日志。
- [ ] 在 `.env.example`、`.env.development.example`、`.env.worker.example`、Docker Compose、Helm values 和配置参考文档中补齐可观测环境变量；默认值全部关闭，示例配置必须标明“配置后才启用”。
- [ ] 收紧可观测安全边界：API/Worker metrics 仅在 `METRICS_ENABLED=true` 时使用独立 listener，不挂在业务 API 端口；默认 API `:9090/metrics`、Worker `:9091/metrics`，生产环境 metrics Service 默认只在集群内暴露，不配置 Ingress；Prometheus/Loki/Tempo/Grafana 查询和外链由后端生成或聚合，前端不得直接拼底层平台 API；日志、trace attribute 和 metric label 禁止记录 Secret、Token、Cookie、Authorization header 和用户输入原文。
- [ ] 增加配置自检与系统设置展示：管理员可以看到每个可观测模块的启用状态、缺失配置键、采集端点和最后一次导出/查询错误；普通用户只看到可用的业务状态和受控跳转。

### 12.2 指标与 Prometheus/Grafana

- [x] API/Worker metrics MVP：接入 Prometheus client，独立 registry 注册 Go/process/up 指标；API 记录 HTTP 请求量、延迟和 inflight；Worker 记录任务 started/completed、耗时和 inflight，并按 `build/deploy/light` 队列打标签。
- [x] API 在 `METRICS_ENABLED=true` 时启动独立 metrics HTTP server，默认监听 `:9090/metrics`，并暴露 HTTP 请求量、延迟、错误响应、PostgreSQL 连接池、PostgreSQL/Redis 依赖健康指标；未启用时不监听 metrics 端口。
- [x] Worker 在 `METRICS_ENABLED=true` 时启动独立 metrics HTTP server，默认监听 `:9091/metrics`，并暴露任务启动/完成、耗时、重试、inflight、队列深度、队列等待和依赖健康指标；多副本指标可按 Prometheus 维度聚合，不依赖进程内全局状态表达分布式事实。
- [x] 运营面板 iframe MVP：平台管理员可在站点设置中配置 Grafana dashboard/panel 嵌入地址，并在系统管理区查看运营面板；普通用户不展示入口。
- [x] 构建链路补齐 Prometheus 指标：构建结果、构建耗时和超时/lost 结果；标签只允许稳定低基数字段。阶段耗时和镜像推送细分需等待 builder 结构化阶段事件后再补。
- [x] 发布与运行态补齐 Prometheus 指标：发布结果、发布耗时、ready/desired/available/updated/unavailable 副本；运行态资源指标优先从 Kubernetes/Prometheus 查询，不把 worker 内存状态当真相源。
- [x] 访问入口补齐 Prometheus 指标：路由状态、TLS/DNS/证书状态分布、网关同步结果和耗时；入口真实流量优先复用 Traefik/Gateway API controller 指标。
- [ ] 在 `PROMETHEUS_QUERY_ENABLED=true` 且 `PROMETHEUS_BASE_URL` 已配置时，后端提供受控查询 API，为看板、项目空间概览、应用概览和部署页返回聚合后的轻量趋势；未配置时 UI 隐藏趋势图并保留业务状态。
- [ ] 在 `GRAFANA_LINKS_ENABLED=true` 且 `GRAFANA_BASE_URL` 已配置时，按页面上下文生成 Grafana dashboard 深链；未配置时不展示 Grafana 入口。
- [x] 提供 Grafana dashboard JSON 或 Helm ConfigMap：Liteyuki Overview、API、Worker、Build、Release、Gateway、Runtime、Dependencies；不同指标按 stat、time series、bar gauge、table 选择合适图表。

### 12.3 链路追踪

- [ ] 接入 OpenTelemetry SDK：仅当 `OTEL_TRACING_ENABLED=true` 且 `OTEL_EXPORTER_OTLP_ENDPOINT` 已配置时初始化 tracer provider 和 OTLP exporter；未配置时使用 no-op tracer，不影响主流程。
- [ ] 为 Gin 请求、GORM 查询、Redis/Asynq 投递、外部 Git/Registry/Kubernetes provider 调用建立 span，span 命名使用稳定路由模板和操作名，不包含用户输入原文。
- [ ] Asynq 任务 envelope 透传 W3C Trace Context；API 创建 BuildRun/Release 后投递任务，Worker 继续同一 trace，构建/部署/网关任务默认全量保留错误和慢任务 trace。
- [ ] 为构建和发布阶段增加业务 span：checkout、build、push、apply、rollout wait、gateway sync；BuildRun/Release 详情保存 trace_id，用于 UI 生成受控 trace 跳转。
- [ ] 支持采样配置：`OTEL_TRACES_SAMPLER` 和采样比例环境变量；错误 trace、慢任务 trace 和构建/发布任务优先保留。
- [ ] 在 `GRAFANA_LINKS_ENABLED=true` 且 `GRAFANA_BASE_URL` 已配置时，为构建详情、发布详情和访问入口生成 Tempo/Trace 深链；未配置时不展示 trace 入口。

### 12.4 日志上报与查询

- [ ] 将 API/Worker 日志统一为可选结构化输出：仅当 `STRUCTURED_LOG_ENABLED=true` 时使用 JSON slog，默认保持当前开发友好输出；JSON 字段包含 service、component、trace_id、request_id、task_id、project_id、application_id、build_run_id、release_id、operation 和 error_code。
- [ ] 日志导出显式开关：仅当 `LOG_EXPORT_ENABLED=true` 且 `LOG_EXPORT_OTLP_ENDPOINT` 或 `LOKI_BASE_URL` 已配置时启用日志导出；未配置时只输出到 stdout，不初始化远端日志客户端。
- [ ] 构建日志、Hook 日志、发布日志和运行 Pod 日志继续按业务权限在平台内展示；同时在启用日志导出时附加 trace_id、build_run_id、release_id 和 deployment_target_id，便于 Loki 聚合检索。
- [ ] 在 `LOKI_LINKS_ENABLED=true` 且 `LOKI_BASE_URL`/`GRAFANA_BASE_URL` 已配置时，后端为构建、发布、运行日志生成受控 Loki/Grafana Explore 深链；未配置时仅展示平台内日志。
- [ ] 增加日志脱敏统一组件验收：Secret、Token、Authorization header、Cookie、Registry 凭据、Git 凭据和 URL 内敏感参数不得进入平台日志、导出日志或 trace attribute。
- [ ] 规划大日志归档：对象存储归档仍独立于 Loki，启用归档需要单独显式开关和存储配置；未配置时只保留数据库日志窗口。

### 12.5 告警与用户体验闭环

- [ ] 提供 Prometheus alert rules：API 5xx、API P95 延迟、Worker 队列积压、构建失败率、发布失败率、Redis/PostgreSQL/Kubernetes 不可用、证书失败过多；规则文件随 Helm/Compose 示例提供，但不默认启用外部告警发送。
- [ ] 在 `ALERT_LINKS_ENABLED=true` 且 `ALERTMANAGER_BASE_URL` 已配置时，管理员页面展示 Alertmanager 入口和当前告警摘要；未配置时不展示告警入口。
- [ ] 平台看板增加可观测摘要：平台健康、队列积压、近期失败构建/发布、运行集群异常；所有摘要缺少 Prometheus 查询配置时回退到数据库业务记录。
- [ ] 项目空间和应用概览增加用户友好的可观测卡片：构建成功率、最近发布状态、副本健康、访问入口状态、资源趋势；趋势依赖 Prometheus 查询开关，状态依赖平台业务记录。
- [ ] 构建/发布失败自动关联最近日志、Kubernetes Events 和 trace_id，前端优先展示“可能原因 + 下一步操作”，深度日志和 trace 作为辅助入口。
- [ ] 完成可观测 MVP 验收：不开任何可观测环境变量时平台行为与当前一致；只开 metrics 时 Prometheus 可 scrape API/Worker；只开 tracing 时构建/发布 trace 可在 Tempo 查看；只开日志导出时 Loki 可按 trace_id/build_run_id 查询；只开 Grafana 链接时 UI 展示受控 dashboard/trace/log 跳转。

## 13. 通知适配器

目标：平台内部只产生结构化通知事件，发送层统一交给通知适配器；飞书、企微等可由 Webhook 预设生成渠道快照，只有 SMTP 这类不同协议或未来需要专用能力的平台才新增独立适配器。

- [x] 定义通知核心模型与迁移：`NotificationChannel`、`NotificationTemplate`、`NotificationRule`、`NotificationDelivery`；渠道保存适配器类型、配置快照、Secret 引用、启用状态和最近投递状态，敏感字段不明文落业务表。
- [x] 实现通知适配器 Registry：统一 `Validate`、`Render`、`Send`、`Test` 接口；业务模块只 emit `NotificationEvent`，不关心渠道平台和消息格式。
- [x] 实现 Webhook 适配器内核：支持 method 白名单、URL/Header/JSON Body 模板、Go template 安全函数、JSON 校验、超时和 SSRF 防护；投递记录脱敏在异步投递层补齐。
- [x] 实现 SMTP 适配器内核：支持 SMTP/STARTTLS/TLS、登录、From/To/Cc/Bcc、subject/body 模板和测试发送；密码通过 Secret Store 保存。
- [x] 内置 Webhook 渠道预设定义：飞书 Bot、Lark Bot、企微 Bot、Gotify、钉钉 Bot、Slack Incoming Webhook、Discord Webhook 均以 webhook 模板表达；API 从预设创建渠道快照，预设更新不影响已有渠道。
- [x] 实现通知渠道/模板/规则 CRUD API：平台管理员可管理全局通知资源，后续再按项目空间扩展项目级规则；保存渠道时把敏感字段写入 Secret Store，响应只回显 `secretSet`。
- [x] 实现从 Webhook 预设创建渠道快照：用户填写预设密钥后生成普通 webhook 渠道配置和默认模板，后续可提供“按最新预设重置”。
- [x] 实现通知规则匹配与异步投递：按事件类型、项目空间、应用、部署配置、严重级别过滤；生成 `NotificationDelivery` 后由 worker 调用适配器投递并记录结果、重试次数、耗时和脱敏错误。
- [x] 接入首批失败事件：`build.failed`、`release.failed`、`hook.failed`、`gateway.apply_failed`；成功类事件先不默认开启，避免通知噪音。
- [x] 提供管理员通知配置 UI：渠道列表、创建/编辑 Webhook/SMTP、从预设创建渠道、测试发送、模板管理、规则管理和投递记录；用户可见文案全部接入 i18n。
- [x] 通知渠道测试发送增加二次确认，测试事件使用预设模板变量渲染，覆盖项目空间、应用、部署配置、构建、发布、Hook 和访问入口等常用字段。
- [x] 补充通知文档与验收：说明适配器边界、模板变量 schema、安全限制、Webhook 预设快照行为和 SMTP 配置示例。
- [x] 优化内置通知模板信息密度：Webhook 预设模板统一输出事件摘要、资源上下文、构建/发布/Hook/访问入口详情，并在配置 `PUBLIC_BASE_URL` 后附带可直达应用对应 tab 的详情链接。
- [x] 修复预设 Webhook 渠道默认投递模板不匹配的问题：规则未显式选择模板时复用渠道预设消息体，并对模板错误和非 429 的 4xx Webhook 响应跳过无意义重试。

## 100.优化需求

- [ ] 智能引导：例如用户在创建APP选择Git账号时发现没有账号，旁边用一个按钮引导去授权页面。这样的场景还有很多，不一定是Git账号，后续可以总结一批这样的场景进行统一优化。
