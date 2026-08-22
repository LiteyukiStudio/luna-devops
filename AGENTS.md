# AGENTS.md

本文件是给 AI 编码代理的项目级开发规范。保持简短、可执行、少歧义；细节优先从 `docs/`、`docs-internal/` 和现有代码中渐进读取。内部文档的分层与索引见 `docs-internal/README.md`；人类贡献者入口为 `CONTRIBUTING.md`。

## 0. 开工前必读

按需阅读，但开始实现前至少确认这些文件：

1. `README.md`
2. `TODO.md`
3. `docs-internal/README.md`（内部文档分层与索引）
4. `docs-internal/01-产品与一体化方案.md`
5. `docs-internal/07-代码健康检查SOP.md`

## 1. Hard MUST

- 先读现有代码和文档，再修改。
- 不主动执行 `git commit`、`git push`、创建/切换分支等 Git 操作，除非用户明确要求。
- 对一次可完成的小任务采用“一个目标一轮推进”的节奏：每完成一个可独立验收的事项（如一次功能点、文档修订、定位与修复闭环），要形成可追溯记录并与该事项绑定。
- 编写新功能或有逻辑改动时，必须同步更新 `docs/` 文档站内容；仅涉及旧文档归档时更新 `docs-internal/`。影响计划、验收或状态时也必须更新 `TODO.md`。
- 当问题根因来自职责堆积、抽象缺失、旧模型残留或重复逻辑时，优先通过小范围重构消除根因；不要为了“最小改动”继续堆临时 patch 或特殊 case。
- 完成实现后按改动规模选择验证：小功能改动只做针对性检查（相关 Go 包测试、TypeScript 类型检查或局部 smoke），不强制全量 lint/build/浏览器验收。
- 当一次改动满足任一条件时，必须执行完整验证并优先用浏览器验收前端交互：修改文件数超过 8 个、同时跨 3 个及以上业务域、涉及认证/权限/Secret/SSRF/数据库迁移/构建部署运行时、或用户明确要求验收。验收通过后再把 `TODO.md` 对应项标记完成。
- **MUST 端到端调用链一致性**：新增功能或修改既有行为时，必须逐层审计前端、API 后端、Worker、Agent 四个边界并明确适用或不适用；凡实际参与调用链的层都必须在同一事项中同步完成。请求/响应 Schema、OpenAPI、前端类型与 API Client、Agent 工具 Schema、异步任务载荷、事件/SSE 协议、权限与审计、错误码、幂等语义及可观测字段必须保持一致，禁止只修改某一层后依赖运行时容错、宽松解析或人工约定维持兼容。验证至少覆盖一条从真实入口到最终副作用或权威回读的成功链路，以及涉及层之间的契约测试；存在异步、失败或取消路径时还必须覆盖对应终态。
- **MUST i18n**：前端任何用户可见文本常量必须走 `i18next/react-i18next`，不可硬编码。包括标题、描述、按钮、菜单、表单 label、hint、placeholder、toast、错误/空状态、确认弹窗、aria-label、schema 校验文案和状态 badge。产品名、文件名、API enum 原始值、URL/slug 示例可以保留为数据或示例；只要作为 UI 文案展示，就必须用 i18n label。
- **MUST i18n 边界**：能在前端本地化的内容必须由前端按稳定 `code`、枚举值或状态 key 映射 i18n 文案；后端只返回稳定 key、原始枚举和必要的原始 message/remark 备注，不返回面向用户的本地化文案。日志正文、第三方原始文本和用户输入内容作为数据展示时例外，但不能冒充 UI 文案。
- **MUST 品牌命名边界**：用户可见品牌统一使用 `Luna DevOps`；项目自有运行标识统一使用 `luna-devops`、`luna.devops`、`luna-gateway` 或 `luna_devops_`（metrics/代码中需要下划线时）。项目尚未发版，不保留旧品牌技术标识兼容层，也不要把品牌技术标识做成用户可配置项。开发者、仓库、文档站和镜像发布地址仍使用真实可达的 Liteyuki Studio 资源：`github.com/LiteyukiStudio/luna-devops`、`https://luna-devops.liteyuki.org`、`liteyukistudio/devops-*`。
- **MUST 后端适配外部平台**：涉及 GitHub、Gitea、GitLab、Harbor、DockerHub、OIDC、Kubernetes、Traefik、AI Provider 等第三方/外部平台的读取、探测、搜索、状态同步和写操作，必须由后端 provider/service/API 适配、聚合或反代。前端只调用平台后端 API，不允许在前端编排第三方平台 API、暴露底层外部平台能力，或用多个底层代理接口拼出业务流程。
- **MUST 实时状态单一事实源**：当前状态、健康度、实时资源数量、实时指标等有时效要求的数据，必须在请求时从 Kubernetes 或对应外部平台读取。数据库只保存期望配置、资源引用、工作流过程与结果、不可变历史，不得持久化、回写或用 Redis/进程内缓存保存上游当前状态。上游不可达时统一返回 `unavailable` 和稳定 `observationCode`；实时响应必须使用 `Cache-Control: no-store`，前端不得通过长 `staleTime` 延续旧状态。
- **MUST Agent Prompt 中文**：`luna-agent` 中的系统 Prompt、模型任务提示、上下文包裹说明、工具描述和配套 Skill 必须使用中文编写；工具名、参数名、枚举、路由名、协议字段和用户原始输入保持原值。Prompt 仍应要求模型按用户当前语言回复。项目未发版，只保留当前 Prompt 版本，不维护旧 Prompt 前向兼容分支。
- **MUST 全链路可观测**：新增或修改业务功能时必须遵守 `docs-internal/14-可观测插桩与验收标准.md`。每个 HTTP/SSE/WebSocket、数据库、Redis、异步任务、外部 Provider、模型和工具调用都必须处于有效 Trace Context 中；关键状态转换必须输出可关联的结构化日志；可聚合结果必须补充低基数 Metric。禁止只给接口入口建 Span 而把内部操作留作黑盒。
- **MUST Context 传播**：请求、Repository、Secret、审计、外部 Client、任务投递和 Worker 执行必须继续传递现有 `context.Context` 或 W3C `traceparent`/`tracestate`，不得在业务调用链中改用 `context.Background()` 截断父链路。跨服务新增 HTTP、消息队列、SSE 或 WebSocket 通道时，必须同时实现传播与父子关系测试；Trace Context 只携带遥测标识，不得复制 Cookie、Token、请求正文或用户输入。
- **MUST 遥测安全与稳定维度**：Span 名、日志事件名和 Metric label 必须使用稳定模板与有限枚举，不得包含用户输入、URL 查询值、资源名、用户/项目/请求/Trace ID 等高基数内容；Secret、Token、Authorization、Cookie、密码、模型 Prompt、工具敏感参数和进程命令行参数不得进入遥测。
- **MUST 可观测验收**：涉及新业务边界、数据库/Redis、异步任务、外部平台或跨服务通信的改动，测试至少断言父子 Trace 传播、失败 Span 状态、关键日志关联字段及敏感字段不出现；完整验收时使用临时外部 OTel 栈抽样验证一条成功链路、一条失败链路和一条跨服务/异步链路，临时可观测组件不得写入仓库。
- Secret、Token、Registry Credential 不允许明文落业务表；密钥类字段不回显给前端。

## 2. 文档编写规范

- `docs/docs/{zh,en}` 是面向 Luna DevOps 使用者和部署管理员的公开文档；`docs-internal/` 存放内部开发文档（长期规范与方案/记录，分层见 `docs-internal/README.md`）；`AGENTS.md` 与 `CONTRIBUTING.md` 分别是 AI 代理与人类贡献者的约束入口。不同受众的内容不得混写。
- 公开文档以“帮助用户完成一个任务”为目标，先给结论、前置条件和最短可行步骤，再按需链接到参考信息；不要按代码模块、内部架构或研发流程组织内容。
- 使用渐进披露：主流程只说明当前步骤必须知道的内容；高级配置、完整参数、兼容范围和排障细节放到独立参考页，避免在入门页堆叠所有选项。
- 一个页面只承载一个清晰目标。优先采用“用途 → 前置条件 → 操作步骤 → 预期结果 → 常见问题/相关参考”的结构；没有实际内容的章节不要保留。
- 公开文档默认不写 CI/发布门禁、覆盖率统计、内部仓库关系、分支策略、源码协作方式、实现架构、迁移过程、历史决策、内部待办或发版前剩余事项。这些内容应写入 `docs-internal/`、`TODO.md`、代码注释或仓库内部开发规范。
- 不在公开文档中用“下一步”展示团队尚未完成的研发计划。仅当用户完成当前任务后确实需要继续操作时，才提供与用户旅程相关的下一步入口。
- 必须保留会直接影响用户操作和风险判断的信息，包括必要前置条件、权限要求、数据影响、安全警告、兼容范围、失败原因、恢复方式和不可逆限制；精简不得以隐藏风险为代价。
- 描述稳定的产品行为和用户可观察结果，不展开内部实现。命令、字段和配置只解释用户需要提供什么、何时使用及其影响。
- 配置文档先给可运行的最小配置，再把可选项按场景分组；不要让高级调优项阻塞首次安装或首次使用。
- 配置表的“说明”列必须用一句话同时说明配置用途或影响，以及允许填写的值、格式、范围或单位；不得只重复类型或枚举。跨字段约束、安全风险等一句话无法说明的内容使用上标脚注，并在表格下以“数字. 说明：”补充。
- 删除功能、配置或兼容路径时，必须同步删除现行代码、测试、部署模板、示例配置、导航、公开文档和操作示例中的对应入口与叙述。当前文档只描述删除后的现行行为；除非用户明确要求，或仍存在直接影响操作与安全的兼容约束，否则不得新增迁移理由、历史背景、替代方案或纪念性说明，也不得为填补删除后的空白擅自扩展相邻功能。
- 避免重复维护同一事实。版本明细以 Release 为准，API 契约以 OpenAPI 为准，命令与参数以 CLI 帮助为准；公开文档只保留必要说明和稳定入口。
- 顶级导航保持少而稳定，以用户旅程和任务类别命名；内部开发、历史记录和发布流程不得进入公开导航。
- 中文与英文文档的目录、导航和事实必须同步；翻译可以适应语言习惯，但不得出现一侧独有的重要限制或步骤。
- 文档变更至少检查中英文导航、内部链接和 `pnpm --dir docs build`；修改导航、主要用户旅程或页面结构时，还必须用浏览器验收桌面端页面，必要时补充移动端验收。

## 3. 技术栈

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

## 4. 目录边界

- 仓库是 monorepo。
- Go 后端在仓库根目录。
- 前端在 `web/`。
- 本地数据库依赖放 `docker-compose-dev-db.yaml`，只包含 PostgreSQL 和 Redis；本地可观测组件放 `docker-compose-dev-observability.yaml`。API、Worker、Agent 和 Web 均在宿主机手动启动，不纳入开发 Compose。
- `.env.*` 不提交；`.env.example` 可提交。
- 后端配置默认读取进程环境和仓库根目录 `.env`；需要临时使用另一份本地文件时可通过 `ENV_FILE=.env.local go run ./cmd/api` 显式替代 `.env`。

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

## 5. 后端准则

- 第一阶段采用模块化单体 + 多进程部署。
- `cmd/api` 负责 HTTP API、Webhook、OAuth 回调、CRUD、权限校验和任务投递。
- `cmd/worker` 负责构建、部署、状态同步、证书申请、资源清理等异步任务。
- 长耗时任务进入 worker，不在 HTTP 请求里同步执行。
- Handler 只做参数解析、权限入口和响应；业务逻辑放 service；数据访问放 repository；外部系统调用放 provider。
- 平台角色与项目角色必须复用 `internal/authz`、`web/src/lib/roles.ts` 和 OpenAPI 中的共享角色 schema，禁止在授权判断、输入校验和测试夹具中散落角色字面量。LLM 消息 role、资源 scope、Git owner 等同名字字段属于独立语义，不得混入授权角色常量。
- 构建/部署阶段的用户配置字符串默认允许使用 GitHub Actions 风格变量；最终执行前必须通过后端统一变量渲染组件处理，禁止在各业务里手写零散替换逻辑。
- 权限由后端最终判断，前端隐藏按钮只做体验优化。
- 危险操作必须写 AuditLog。
- **MUST Agent 工具注册闭环**：普通平台工具以 OpenAPI operation 为唯一事实源。新增或修改 `operationId` 时，必须在同一事项内同步真实业务路由、请求/响应 Schema、`requiredScopes`、`requiresApproval`、敏感字段、别名和用途说明，并确认 `internal/aitool.PlatformCatalog()` 能自动生成完整契约；特殊协议只能在集中 deny map 中以稳定原因排除。Agent 不维护手写平台 fallback、重复白名单或二次执行 router。仅模型内部工具需要同步 `tool-presentation` 注册、Executor handler 与测试。
- 修改既有工具的 Scope、参数 Schema 或审批要求时，必须验证远端 Provider Catalog、Agent 详情加载、固定服务身份直达原业务 Handler/Service、当前 Session/用户/项目空间权威回读、最终 RBAC 和审计保持一致。完成后必须用真实调用链验证副作用与权威回读，不能仅编译通过就交付。
- **MUST Agent 工具可检索性**：工具目录不得把全部定义无条件下发给模型。每个新增或变更工具必须提供可区分的用途、禁用场景、前置条件、主要参数和成功回读说明，并确保它能通过 `search_tools` 的 Unicode/BM25 分页摘要检索命中，再通过 `get_tool_details` 精确加载完整 Schema；模型在声明缺少能力前必须先检索目录。检索只负责发现，不授予权限、不执行操作，也不得把用户检索词写入普通遥测属性或 Metric label。常用场景评测必须覆盖正确工具命中与相似读写工具边界，不设置固定 Top K 或向量召回门禁。

## 6. 前端准则

- 页面按 `web/src/pages/<module>` 组织。
- **MUST 前端运行时单例依赖**：React/React DOM、CodeMirror 核心包，以及任何依赖 `instanceof`、全局 Symbol、Context/Provider 或扩展对象身份判断的库，在同一前端产物中必须只解析到一个兼容版本。新增或升级依赖后必须执行 `pnpm --dir web check:singletons` 并检查 `pnpm list`；发现重复版本时在 `web/pnpm-workspace.yaml` 用最小范围 `overrides` 统一依赖族并重新生成 lockfile，禁止依赖打包器偶然去重、运行时 catch 或页面级特殊 case 维持可用。
- `web/src` 下共享模块必须使用 `@/` 根目录导入；公共组件、API、app context、layout、lib、i18n 和跨页面引用都必须用 `@/`。相对导入只用于当前页面/组件目录内的私有文件。
- **MUST shadcn/ui**：前端基础 UI 必须优先使用 shadcn/ui。凡 shadcn/ui 已提供的基础组件、布局组件、表单组件、反馈组件、表格/分页组件，不允许手写同类轮子；只能在业务组合层做薄封装。
- shadcn/ui 基础组件放 `web/src/components/ui`，组件清单见 `web/SHADCN_COMPONENTS.md`。
- 两个及以上页面稳定复用的业务组件必须抽到 `web/src/components/common` 或更合适共享目录。
- 新页面必须归入资源列表、看板/概览、设置或工具工作区，并使用对应的 `PageShell` 宽度；不要在页面内自由维护根宽度与根间距。
- 业务页面优先使用 `Surface`、`Section`、`MetricGroup` 等语义布局；`DataList` 是列表唯一外壳，禁止无业务含义的 Card 嵌套。
- 状态色必须使用语义 token 或公共状态组件，不得在业务页面直接拼写 `red-*`、`amber-*`、`green-*` 等状态样式；第三方品牌色、终端和集中维护的图表色板除外。
- 页面主要区块、相关区块、表单工具和行内元素优先使用 `gap-6`、`gap-4`、`gap-3`、`gap-2`；优先使用 Tailwind 标准 token，不新增任意像素间距。
- 前端设计 token 集中维护在 `web/src/styles/design-tokens.css`，业务代码优先使用语义 Tailwind utility：`rounded-control/container/feature`、`gap-inline/field/group/section`、`p-group/section`、`px-page-inline`、`py-page-block`、语义颜色和 `shadow-raised/overlay`。不要在页面内重复定义页面留白、容器圆角或表面色；shadcn 基础控件继续使用其组件级尺寸，不用页面 token 强行覆盖。
- 登录后主内容画布统一使用“横向宽松、纵向紧凑”的响应式页面内边距：移动端 `px-8 py-4`、中屏 `px-12 py-6`、桌面端 `px-16 py-8`；顶栏使用相同横向 padding。`PageShell` 只负责最大宽度和区块间距，不使用 `mx-auto` 或额外水平 padding，业务页面不得自行用 margin、padding 或负间距重复补偿全局留白。
- 桌面端页面标题属于内容工作区，不使用独立全宽 topbar；标题与正文、Tab 和工具栏共享全局内容 padding 的左侧基线，并以紧凑纵向间距衔接。移动端保留包含侧栏入口和标题的顶栏。
- 桌面端页面头部统一使用 `PageChrome`：第一行左侧标题、右侧页面工具；传入 Tabs 时单独渲染第二行，不传时不保留空白 Tab 区域。`ContentTabs` 只负责 Tab 状态与内容切换，并把可选导航和工具交给 `PageChrome` 统一布局，不得在业务页面重复维护标题、工具和 Tab 的间距。中小屏页面工具保留在正文流中。
- 主表单、设置面板和账号面板默认使用 `p-6`；目录卡片和指标卡片可使用 `p-4` 或 `p-5`；`DataList`、日志、拓扑、终端和 iframe 外壳使用 `p-0` 并由内部结构控制留白。禁止在同一容器叠加父级 padding 与子级补偿 margin。
- 列表、概览、设置和工具工作区必须使用对应的结构化 skeleton；禁止在大容器中只展示一行“加载中”。`total === 0` 时不得展示页码、每页条数或翻页器；首次配置空状态应提供明确下一步，筛选为空状态应保持紧凑并提供清除条件入口。
- 同一页面或 tab 默认最多保留一个实心主色主操作，其他同级操作使用 outline、ghost 或菜单。
- 看板和概览中的失败、不可用和待处理状态必须在摘要层使用语义 tone，并提供明确文字；零值正常指标弱化，零值异常指标不得按中性样式展示。
- 桌面端超过 4 个筛选字段时，移动端必须使用 Sheet、弹出层或等价渐进披露方式；高频移动端列表应显式定义保留列，不能默认依赖桌面表格横向滚动完成适配。
- fixed/sticky 悬浮控件不得覆盖主操作、固定操作列、分页、toast、Dialog 或 Sheet；必须在桌面和移动端保留安全边距。
- 表单统一使用 React Hook Form + Zod。
- React 中能由 props、查询结果或现有 state 直接计算出的值必须在渲染阶段派生，必要时使用 `useMemo`；禁止用同步 `useEffect` 调用 `setState` 回填默认选项、修剪选择项、重置页码或复制受控属性。资源切换后的局部状态应按资源 ID/作用域隔离，用户操作导致的重置应放在对应事件入口。
- `useEffect` 只用于 EventSource、WebSocket、定时器、DOM 和其他外部系统同步。订阅状态必须绑定当前资源 ID，并在 cleanup 中关闭连接、阻止旧回调写入新资源状态；函数调用形式的初始 state 使用惰性初始化。
- 实时观察查询必须复用 `web/src/lib/live-observation-query.ts` 的查询策略；上游断联时展示公共状态组件映射的 `unavailable`，不得保留上一次成功状态冒充当前事实。
- 前端交付前必须保证 `pnpm --dir web lint` 和 `pnpm --dir web build` 无新增 error 或 warning。确属外部同步或工具链刻意行为的告警必须先确认语义，在最小代码范围注明原因；禁止通过全局关闭规则、批量 `eslint-disable` 或降低门禁掩盖告警。
- 必填项使用主题色 `*`，不可用红色强警告风格。
- 未满足要求前提交按钮保持 disabled/弱化；字段错误在对应字段附近展示。
- 设置类表单默认限制在 `max-w-3xl` 至 `max-w-4xl`；页面级表单操作统一使用 `FormActions`，桌面端按钮按内容宽度右对齐，移动端才允许全宽。同一设置页的不同 tab 默认都把保存操作放在表单末尾，不混用顶栏保存与底部保存。Dialog 使用 `DialogFooter`，登录/注册等单任务窄流程可以保留全宽按钮。
- 登录后控制台的页面顶栏与整个内容画布统一由布局层提供 `workspace-background` 低饱和单色主题背景，不使用渐变或多方向彩色光晕，也不在页面组件内使用负间距或超大装饰容器模拟全屏；多色主题的辅助色、支持色和强调色只能通过主题语义 token 参与选中态和结构线，不在业务组件内直接拼接具体色值。普通业务 `Surface`、`Card`、指标组、状态提示和资源陈列卡片的外层不绘制边框，以实体表面和圆角分层，也不添加常驻阴影。阴影仅用于 Dialog、Popover、悬浮层、明确的 raised 表面和交互 hover；表格行、分栏、输入框及表单内部需要表达归属的局部结构继续保留必要语义边界。
- 登录后应用根节点使用 `primary-subtle` 主题背景，桌面侧边栏保持透明并继承全局画布，不额外设置背景、右侧分隔边或菜单分组线；菜单类型通过分组标题与纵向留白区分，导航悬停和选中态使用主题语义 token，移动抽屉作为覆盖层继续使用实体主题背景。
- 控制台界面风格支持“标准 / 简约”及“跟随平台”三态偏好：平台配置只决定默认值，账号空偏好持续继承平台，显式个人偏好覆盖平台。简约模式只把大面积画布、侧边栏和导航弱表面恢复为中性白/灰表面；主操作、链接、焦点、选中指示和状态语义色仍使用既有主题 token。业务页面不得自行读取该偏好或增加模式分支。
- 侧边栏分组标题必须使用小于菜单项的弱层级字号、正常字重和较低对比度，不得与可点击菜单项争夺视觉注意力。
- 复杂字段必须提供可 hover/focus 的说明图标。
- 能搜索/选择的资源不要让用户手填。
- 密钥字段允许前端填写，但编辑时不展示原值；留空表示不修改。
- 列表类数据必须优先使用统一列表组件；管理台列表默认用表格/行列表并向上对齐，不用等宽卡片流冒充列表。
- 管理台资源列表默认复用构建页的 `DataList` 视觉和交互：固定表头、行内垂直居中、明确操作按钮、底部分页栏；不要为相同列表场景自造表格样式。
- 页面标题已经表达列表上下文时，不要在 `DataList` 工具区重复显示“XX 列表”；搜索、筛选、排序和刷新控件直接从工具区左侧开始排列。只有同一页面存在多个需要区分的独立列表时才传入 `DataList.title`。
- 列表页的搜索、筛选、排序等查询控件放入 `DataList` 的表头工具区，页面级创建操作放入 `PageChrome` 标题右侧；不要在列表上方额外堆叠独立 `PageToolbar`。
- 列表中的编辑、删除、测试、绑定等操作必须使用明确按钮或菜单入口；不要把整行或整张展示卡片做成编辑入口，避免误触和语义混乱。
- 涉及状态的展示必须使用有语义颜色的 `StatusValueBadge` 或带 `tone` 的 `StatusBadge`，包括集群健康状态、镜像站/外部连接健康状态、构建/部署/网关任务状态、Webhook/DNS/证书/扫描状态、启用/禁用和校验状态；不要在列表、详情或卡片中直接显示纯文本状态。
- **MUST 列表 API**：任何返回列表/批量对象的接口，只要未来数据量可能超过 100，就必须支持分页和排序参数，返回 `items/page/pageSize/sortBy/sortOrder/total/totalPages`。排序字段必须做后端白名单映射，排序方向只允许 `asc` 或 `desc`。OIDC Provider、少量系统配置定义等明确不太可能超过 10 条的小规模配置列表可以例外。
- 错误页面必须用户友好，并复用 `ErrorState`、`AuthErrorPage`、`ForbiddenPage` 等公共组件。
- 主题必须支持 light、dark、system 三态，并监听系统主题变化。
- 前端展示“Project”时统一称为“项目空间”，强调集合概念。

## 7. 集成与安全边界

- 平台构建主路径是平台 Builder + BuildKit rootless；GitHub/Gitea 只作为代码源、Webhook 和授权来源。
- 部署由平台执行并记录。
- 构建 Job 不挂载宿主机 Docker socket，不默认 privileged。
- 构建网络默认 restricted egress。
- 默认禁止访问元数据地址、Kubernetes API Server、Service CIDR、私有网段非 443 端口。
- 内网 registry/镜像源可通过白名单或私有网段 TCP 443 放行。
- Webhook 必须校验签名，只接受已绑定仓库事件。
- OIDC Provider 在平台后台配置，不通过环境变量 bootstrap。
- 内部平台不开放自由注册；本地账号由管理员创建、邀请或导入。
- **MUST 计费归属**：AI 模型费用只归属发起用户个人钱包，不关联项目空间——`billing_usage_records` 与 `billing_ledger_entries` 的 AI 记录（meter 以 `ai.` 开头）的 `project_id` 必须为空，不得从会话/应用/部署继承项目空间归属。项目空间维度的资源计费（构建、运行、存储、网关）仍可关联项目空间，但 AI 费用不参与项目空间维度的归集、筛选或分摊。

## 8. 常用命令

```bash
# dev deps
docker compose -f docker-compose-dev-db.yaml up -d

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

## 9. Git 提交消息

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

## 10. 不要做

- 不擅自引入未讨论的新框架，可以推荐给人类。
- 不为 MVP 预先实现完整计费、持久构建缓存、Service Mesh。
- 不把 Gitea/GitHub Actions 作为唯一构建路径。
- 不在 handler 中散落 GORM 查询。
- 不直接展示后端原始异常、OIDC 原始错误或技术堆栈给用户。
- 不提交本地环境文件、构建产物、依赖目录或临时日志。
