# TODO

## 2026-08-28 跨项目列表可见范围统一

- [x] 将看板、项目空间、事件和跨项目资源列表的 OpenAPI 查询契约统一为 `visibility=related|all`，默认 `related`，并明确管理员显式全局查询及 400/403 语义。
- [x] 同步 Agent 中文工作流与中英文公开文档，保留通知个人/共享双轨和账单独立范围。
- [x] 增加 OpenAPI 契约测试，锁定共享参数、目标 operation 覆盖范围及通知/收件箱排除边界。

## 2026-08-28 全局邮件服务与个人通知

- [x] 将注册邮件的 SMTP 配置迁移为平台级邮件服务，保持凭据只存 Secret Store，并提供管理员测试发送。
- [x] 向普通用户开放本人通知偏好和本人 Webhook 集成，个人邮件默认启用且只发送到账号邮箱。
- [x] 打通事件、API、Worker、OpenAPI、Web 与双语文档，并完成迁移、权限、投递及浏览器验收。
- [x] 为个人事件邮件增加管理员可配置的聚合冷却：默认 60 秒、范围 0–3600 秒，`0` 表示不等待，事务型邮件和共享 SMTP 不受影响。

## 2026-08-28 通知相关人与共享路由收口

- [x] 将个人事件收件人统一为“内部操作者 ∪ 被操作资源创建者”，去重后分别应用当前项目访问权和本人订阅偏好。
- [x] 在 Worker 发送前基于权威平台事件再次校验收件人、项目权限与订阅状态，拒绝伪造或已撤权的个人投递。
- [x] 将共享通知规则改为显式项目空间范围或显式全部项目，空范围、非法范围和旧空过滤器一律不匹配。
- [x] 通过持久化扇出状态、原子物化、统一入队恢复和原始 Trace Context 传播，关闭事件记录后崩溃、部分入队及重试覆盖窗口。
- [x] 同步 API、Worker、OpenAPI、Web、Agent 派生契约和中英文文档，并完成权限、去重、撤权、失败关闭及真实 PostgreSQL 迁移验证。

## 2026-08-27 过度防御设计收敛

- [x] 收敛 Web 中无生产者 Action、旧响应/路由兼容、重复事件去重、终端预授权和重复浏览器存储容错，同时保留 XSS、原型污染、登录跳转与 SSE 缺口恢复边界。
- [x] 收敛 API/Worker 中重复身份扫描、重复 OAuth 查询、非真实写入边界脱敏、机会性恢复扫描与旧兼容输入，同时保留 RBAC、Secret 脱敏、任务幂等与终态围栏。
- [x] 收敛 Agent 动态安全预算、闲置探针、递归上下文压缩和订阅者重复索引，固定 128 KiB UTF-8 输入及 4/64 SSE 订阅边界。
- [x] 删除数据卷导入的浏览器预哈希与声明式校验和，改由 API 和 Runtime Pod 在同一次流传输中独立计算并比对权威摘要。
- [x] 同步 OpenAPI、双语公开文档和内部规格，并完成 Go、Web、Agent、Docs 与契约全量验收。

## 2026-08-27 过度设计审计第二、三批收敛

- [x] 完成第二批机制收敛：Agent NetworkPolicy 改为 ingress 默认隔离、egress 显式启用，缩减主题目录，移除自动导航队列与角色模拟器，统一远程配置快照、BuildKit 执行、NetworkPolicy 适配和 CI 门禁。
- [x] 完成第三批产品取舍：审批仅保留单次批准/拒绝，Agent 收敛为单副本 PostgreSQL 持久时间线，一次性 Runtime Exec 替代持久 Shell 会话，并将 Web API Client 改为静态聚合。
- [x] 完成跨层契约、迁移升级/回滚、Go/Web/Agent/Docs/Helm 全量门禁与报告 DOM/交互脚本验收；本地 `file://` 的自动化视觉验收受浏览器 URL 安全策略限制。
- [ ] 修复 Agent 密钥派生测试未被 Vitest 收集的问题。

## 2026-08-26 过渡命令退场与孤儿引用审计

- [x] 删除运行时环境明文审计、旧数据卷中心迁移命令及仅由它们使用的代码、测试和操作文档。
- [x] 完成 Go、Web 与 Agent 的静态入口、导入导出、脚本和构建引用审计。
- [x] 完成第一批低风险清理：删除零入口 Web/Go 模块、旧 Agent 卡片协议与一次性 Prompt Cache benchmark，收敛 Worker 指标，并移除无消费者的任务影子事件及测试专用数据库 Schema。
- [x] 评估并实施审计报告中需要产品取舍的第二、三批简化；具体结果与验收记录见 2026-08-27 事项。

## 2026-08-26 终端日志单行渲染

- [x] 将 Go API、Worker、辅助进程与 Agent 的 console 日志统一为每条记录占一行，并保持结构化字段、凭据遮罩及 OTel 导出语义不变。
- [x] 同步中英文可观测文档，并用 Go 与 Agent 针对性测试锁定单行输出契约。

## 2026-08-25 移动端 AI 助手真实页面重构

- [x] 抽取唯一 AI Runtime 与共享 ChatSurface，保证页面/浮窗切换时会话、草稿、Mutation 和 SSE 不重建。
- [x] 新增 `/ai-assistant` immersive 路由与完整会话列表视图，移动端入口改为正常导航并保留原业务页面上下文。
- [x] 删除移动 Portal 与半屏会话层，完成键盘、safe-area、触控尺寸、错误状态、返回历史和横屏适配。
- [x] 同步五语言、中英文公开文档，并完成前端全量门禁与固定视口自动化验收。
- [ ] 在获得本地开发账号登录确认后，补充已登录移动浏览器的键盘开合、会话列表、横竖屏、返回与流式输出验收。

## 2026-08-25 Agent Provider 安全缓存能力

- [x] 用显式 Provider 兼容类型解决网关隐藏 DeepSeek hostname 后的适配器误判，同时保留直连地址的自动识别默认行为。
- [x] 为会话助手请求增加 capability-gated 匿名 `prompt_cache_key`；未知兼容端点默认不发送，DeepSeek 适配器强制截断。
- [x] 将用户/页面上下文与历史工具交互规范化为按 Turn 可重放的追加式消息组；动态工作流参考只用于当前轮，避免过期流程随历史累积，并稳定工具序列与单项裁剪结果。
- [x] 模型 Span 增加 `assistant|summary|title` 低基数 purpose，并让 Trace typed usage 只聚合 `assistant`。
- [x] 增加确定性 Prompt Cache 前缀稳定性与功能等价回归，并以 Provider 官方 usage 观测真实缓存命中。
- [x] 补齐双层 New API Header 转发与叶子截断说明、中英文文档、OpenAPI/Agent/Web 契约和分层测试。

## 2026-08-25 Agent Prompt 缓存命中率分层观测

- [x] 在 Agent 观测总览按选中时段展示官方加权缓存命中率，在 Trace 详情按当前 Trace 展示同口径结果；统一使用 `Σcache-read input tokens / Σinput tokens`，不得平均逐次请求比例。
- [x] 保证 cache-write 只进入总输入分母、不进入命中分子，并区分未报告或零输入的 `null` 与明确上报零命中的 `0%`。
- [x] 同步 Go、OpenAPI、Web 五语言、双语公开文档与契约测试，并完成列表、详情和时段切换验收。

## 2026-08-25 Agent Token 观测语义统一

- [x] 按 OpenTelemetry GenAI 当前语义统一模型 input、output、cache-read、cache-write 与 reasoning 用量字段，并修复 DeepSeek 缓存命中适配。
- [x] 将 Agent 会话与轮次聚合切换到权威 `ai.model_usages`，同步 Go、OpenAPI、Web 五语言和 Tempo 属性消费。
- [x] 补齐跨边界契约测试、中英文可观测参考与完整验收，确认缓存和 reasoning 子集不会重复计入总量。

## 2026-08-24 Agent 轮次 Trace ID 快速复制

- [x] 在轮次详情头部突出展示完整 Trace ID，并支持点击复制与成功/失败反馈。
- [x] 同步中英文可观测参考文档并完成前端交互验收。

## 2026-08-24 Agent 会话上下文用量连续展示

- [x] 后端在 Timeline 返回会话最近一次已确认的官方上下文大小，并在新 Run 活动期间保持该值。
- [x] 前端圆环改用会话级 `total_tokens`，新结果权威替换且上下文压缩后允许下降。
- [x] 同步 OpenAPI、中英文公开文档与前后端契约测试并完成验收。

## 2026-08-24 Agent 渠道亲和性

- [x] 在全局 AI 助手设置中新增默认启用的“渠道亲和性”开关和可访问说明 Tip。
- [x] 在 Luna API → Agent → Provider 请求链路中下发配置，并仅发送匿名会话亲和键。
- [x] 在中英文文档站参考区增加 New API 配置与验证示例。
- [x] 完成后端、Agent、Web、文档站与浏览器全量验收。

## 2026-08-24 Agent 活动流与终态持久化重构

- [x] 将模型正文与思考增量从 PostgreSQL 逐事件事务迁移到有界活动流，并保留模型步骤与 Run 终态的原子事实写入。
- [x] 增加跨 Agent 副本的 Redis 短期回放、显式 SSE 心跳、数据库连接池限界与 API 非流式超时配置。
- [x] 修复刷新后活动 Run 恢复、连接停滞回读重连和当前回合输出提示，同步 OpenAPI、部署配置和中英文文档。
- [x] 完成同口径重构后 benchmark、独立代码审查、完整测试、真实浏览器对话与本地 HTML 报告验收。
- [x] 补齐 Agent 缺少 Redis 启动配置时的精确诊断，并增加 Agent 专属 Helm Redis 注入契约门禁。

## 2026-08-24 运行集群实时资源压力

- [x] 新增基于 Kubernetes Nodes、Pods 与可选 Node Metrics 的 CPU/内存实时压力观察，按 10 秒轮询且不持久化当前状态。
- [x] 集群列表展示 requests / allocatable 圆环与实际用量提示，部署集群选择仅向普通用户展示加权压力等级。
- [x] 同步 OpenAPI、Agent 工具契约、五语言文案、双语公开文档和自动化验证。

## 2026-08-23 AI 官方用量、上下文压缩与计费链路重构

- [x] 以 Chat Completions 官方 `usage` 作为已发生 Token 的唯一事实源，删除本地 Token 估算、预留值回填和 cached output 链路。
- [x] 拆分每 attempt 信用 hold 与权威 usage，同步 Provider、Agent、PostgreSQL、Go 结算、OpenAPI 与 Web 五语契约。
- [x] 完成可销毁 PostgreSQL、模拟 Chat Completions、临时 OTel、全量测试与中英文档验收。

## 2026-08-23 应用模板安装工具契约增强

- [x] 为 `installAppTemplate` 补齐稳定 CLI 命令、Agent 语义、严格阶段枚举与可修复参数校验。
- [x] 统一后端结构化阶段错误，并让 Agent 对明确不可重试的同参数失败即时熔断。
- [x] 同步 CLI 生成契约、双语公开文档并完成全链路验证。

## 2026-08-22 Kubernetes 资源分配与运行计费

- [x] 将 requests/limits 策略绑定 RuntimeCluster，部署配置改为 CPU/内存配额并移除独立 limits。
- [x] 使用 metrics.k8s.io 保存分钟级不可变观察，按小时逐区间聚合 max(有效 request，实际用量)。
- [x] 同步迁移、API、OpenAPI、Agent 派生契约、Web 五语言、双语文档和自动化验证。

## 2026-08-22 全项目日志与错误边界重构

- [x] 统一 Go 与 Agent 的结构化日志契约、终端渲染、OTel 导出和凭据遮罩行为。
- [x] 收敛 API、Worker、辅助进程、Agent、Web 的终态错误记录与安全响应边界。
- [x] 同步环境变量、Docker Compose、Helm 和中英文文档，并完成全量测试、构建与当前环境可执行的启动失败验收。

## 2026-08-22 前端 i18n 完整性门禁

- [x] 修复构建模板与冷启动页面漏加载翻译 bundle，增加静态 key、五语言一致性、插值变量、lazy 资源、路由 import 图和动态 key 审计检查，并接入前端 lint。

## 2026-08-22 应用市场分类收敛

- [x] 将模板目录从 15 个细分类合并为 7 个任务导向分类，同步筛选契约、中英文公开文档与多语言标签，并完成目录/API/前端/文档验证。

## 2026-08-22 配置文档语义与删除收敛

- [x] 统一 API、Worker、Agent 中英文配置表说明，每项同时说明用途和可填值。
- [x] 清理已废弃专用配置的代码、测试、部署入口和公开文档残留。
- [x] 完成文档构建、页面验收与全仓残留检查。

## 2026-08-22 Luna CLI Agent 可观测诊断

- [ ] 使用已登录且授予 `agent-observability:read` 的平台管理员实例完成 overview → turns/tools → tool-calls/trace 真实读取验收；当前本地 CLI 无有效登录，远端 OpenAPI digest 也尚未升级到本次契约。

## 2026-08-20 Agent 工具成功率观测

- [ ] 在具备已登录会话的浏览器中验收“对话轮 / 工具情况”切换、工具搜索分页、调用明细展开和 Trace 下钻；当前内嵌浏览器会话已过期且 Chrome 控制连接不可用。

## 2026-08-18 默认计费单价与 AI 模型建议价格

- [ ] 在具备已登录浏览器会话的环境验收模型表单建议价格提示与一键填充交互。

## 2026-08-17 部署配置阶段与安全错误边界

- [ ] 在获得隔离 PostgreSQL schema 创建/删除确认后执行部署配置集成用例，并用临时 OTel 栈抽样验证成功、失败及审计写入失败链路。
- [ ] 在具备已登录浏览器会话的环境验收非法阶段首次预检、选择合法阶段后重新预检与导入成功的桌面端/移动端交互。

## 2026-08-17 部署配置导入候选分页

- [ ] 在具备已登录浏览器会话的环境补充导入 Dialog 桌面端与移动端验收，覆盖搜索、排序、跨页选择和重新预检。

## 2026-08-17 AI 多模型与按模型 Token 计费

- [ ] 补充已登录浏览器的模型能力表单/稳定预算错误展示验收，以及临时 OTel 栈的成功、调用前拒绝和跨服务 Worker 结算 Trace 抽样。

## 2026-08-16 部署默认值与运行计量口径收敛

- [ ] 补充使用可销毁 PostgreSQL、Kubernetes 与临时 OTel 栈的运行计费跨服务 E2E，覆盖观察采样失败、历史窗口待补结算和重算策略。

## 2. 项目基础与前后端脚手架

- [ ] 完成前端视觉体系后续整改阶段 D 视觉回归：在固定桌面/移动视口、暗色模式和高饱和主题下完成全量页面级截图检查。

## 4.1 应用最小单元与模块退场重构

- [ ] 补齐部署失败诊断第二阶段：发布日志和集群工作负载页继续补 Pod events、重新同步入口和镜像架构提示。
- [ ] 镜像架构与本地集群提示：镜像部署失败时在发布日志或资源详情中提示 `no matching manifest for linux/arm64` 等架构不匹配原因，并引导改用支持当前集群架构的镜像或平台构建产物。

## 4.2 应用模板市场

- [ ] 项目空间内新增“从模板安装”入口：安装弹窗只展示模板 schema 里的必要字段，默认短名按 `{templateSlug}-{随机字符}` 生成，密码支持自动生成和复制。
- [ ] 安装完成页展示连接信息：按模板 outputs 展示内网服务域名、端口和建议环境变量，敏感字段默认隐藏并提供复制按钮。

## 5.1.1 删除与 Kubernetes 资源最终一致性

- [ ] 集群孤儿资源自动发现：周期扫描平台 managed labels 下但业务对象已不存在的 Kubernetes 资源，生成残留资源提示或清理建议；当前阶段仍可通过集群资源页手动清理。

## 5.2 安全审计后续

- [ ] 保留 Temporal 作为后期长流程备选：当部署流水线、人工审批、跨集群补偿和多日持久 workflow 复杂度升高时，再从 Asynq 迁移部分流程到 Temporal。

## 7. 平台构建

### 7.1 构建 API/CRUD 优先

- [ ] 项目空间服务依赖 P4（后续可选）：从网关流量、OpenTelemetry 或 Service Mesh 聚合带时间窗口的观测关系，仅作为声明关系的验证信号，不自动修改业务关系。
- [ ] 构建高级参数后续补齐：Dockerfile target stage、目标平台 platform、多镜像 tag、BuildKit secret mount、BuildKit SSH mount、cache import/export 细粒度策略和构建网络策略可视化，保持在高级折叠区渐进开放。

### 7.1 计费系统 MVP

- [ ] 计费采用事件 + 周期采样 + 批量聚合：构建完成按事件结算（已接入），容器运行和存储按分钟采样并按小时聚合，访问次数按时间窗口聚合后再生成账本流水，不在请求路径逐次扣费。

### 7.2 平台系统项目空间与集群探针

- [ ] 为平台组件实例增加卸载/重装二次确认：平台管理员删除或重装探针应用/部署配置时需要明确确认，并同步清理或更新系统组件安装记录。
- [ ] 前端增加系统组件状态视图：在运行集群详情或集群资源页展示探针安装状态、最近上报时间、采集窗口延迟和错误摘要，方便诊断“为什么流量账单没更新”。
- [ ] 补充平台系统项目空间验收：创建运行集群后可部署探针；创建访问入口并产生请求后能看到用量记录和账本流水；禁用探针后不再产生新的访问用量但不影响应用访问。

### 7.4 构建安全与网络策略

- [ ] 细化 Access Token scope：将应用、部署配置、发布、构建和网关接口从粗粒度 `project:read/write` 拆到稳定业务 scope，避免自动化授权语义模糊。

### 7.5 Kubernetes 构建 Job 详细排期

#### 7.4.3 Executor Image / Job 执行

- [ ] 制作平台自有 executor 镜像，内置 git、ca-certificates、buildctl、shell、jq、基础诊断工具。
- [ ] 新增 Build Job Profile：支持配置 executor image、CPU/内存/超时/并发、适用项目范围和能力标签。

#### 7.4.4 隔离与安全

- [ ] 将构建出口网络拒绝事件接入审计或日志视图。

#### 7.4.5 日志、结果和前端展示

- [ ] 后续增加日志对象存储适配，用于大日志归档、检索和下载。
- [ ] 前端构建详情页展示实时日志、状态流转、镜像引用和 digest。
- [ ] 构建列表增加手动刷新和状态过期提示，避免 BuildRun 已完成但前端仍显示 queued。
- [ ] 构建成功后自动创建 ContainerImage，并与 BuildRun、Application、commit 关联。
- [ ] 支持构建成功后按应用环境策略自动创建 Release 并投递部署任务。

#### 7.4.6 后续增强

- [ ] BuildKit 支持 registry mirror / pull-through cache 配置，优先支持 DockerHub、GHCR 和平台内网镜像站。
- [ ] 语言依赖工具链按生态注入镜像源，可选是否注入环境变量或配置文件：npm/pnpm/yarn、pip/poetry、GOPROXY、Maven settings、Gradle init、Cargo config 等。
- [ ] 镜像源凭据通过 BuildKit secret 或一次性文件注入，禁止进入最终镜像层。
- [ ] 支持远程 buildkitd pool，用于高并发或需要共享缓存的场景。
- [ ] 支持构建资源消耗统计，回写 CPU core seconds、memory MB seconds 和 creditCost。
- [ ] 支持构建队列优先级和项目级限额策略。
- [ ] 保留 External CI Provider 作为后期扩展，不作为 MVP 主路径。

## 8. 集群与部署

### 8.1 集群与部署 API/CRUD 优先

- [ ] 部署错误输出友好化：Kubernetes apply/rollout 错误返回稳定错误码，前端按 i18n 展示，避免直接暴露底层异常。
- [ ] 设计 Docker 运行时接入模型：支持 Docker host、Unix socket、TCP TLS CA/cert/key、连接测试、权限边界和部署适配，不复用 kubeconfig 字段。

### 8.2 部署 Worker/执行链路

- [ ] P5 继续评估更完整的高级编排：DaemonSet 是否仅作为平台探针/集群插件能力提供、原生多主容器可视化编辑、容器级独立配置引用、HPA custom metrics、自定义 metrics provider 接入和更严格的策略权限。

### 8.3 集群资源管理

- [ ] 前端提供集群、命名空间、项目空间、应用筛选和手动刷新；空状态说明“只展示平台管理资源”。
- [ ] 资源详情抽屉展示 labels/annotations 摘要、状态条件、关联业务对象和 Events，不展示 Secret data。
- [ ] 集群资源管理 MVP 验收：在测试集群发布一次应用后，集群资源页能看到对应 Namespace、Deployment、Pod、Service、HTTPRoute/Gateway，并能查看相关事件；已有未打平台标签资源默认不显示。

### 8.4 项目空间数据卷中心

- [x] 将数据卷传输改为 Worker 异步准备 Transfer Pod、API 经 Kubernetes exec 直接流式导入导出，并删除 S3、multipart、TUS、Range 和本地完整暂存设计。
- [ ] 在备份校验、对账和不可逆迁移门禁满足后执行 Contract DROP，物理删除 `retained_volumes` 与部署目标旧存储字段。
- [ ] 使用可销毁 PostgreSQL、Kubernetes、浏览器和 OTel 栈完成直接传输迁移/并发、PVC、中断清理及成功/失败 HTTP Trace 验收。
- [ ] 补齐真实取消、权限拒绝、计费终态和 Asynq 跨服务 Trace 的整链路外部 E2E 矩阵。

## 9. 网关与域名

### 9.1 网关与域名 API/CRUD 优先

- [ ] Gateway API HTTPS 证书诊断增强：在诊断中展示 DNS-01 / Certificate / Secret / Gateway certificateRefs 状态。
- [ ] Gateway API HTTPS 证书能力第四阶段：把 cert-manager HTTP-01 作为高级可选能力，明确要求公网 HTTP 入口可达且 Gateway 存在 port 80 listener；校验不满足时给出友好错误，避免误导用户在非 80 内部端口场景使用 HTTP-01。
- [ ] 收紧 Gateway `allowedRoutes`：当前 MVP 使用 `namespaces.from=All` 方便跨命名空间绑定，后续改为项目 namespace label selector。

## 10. 前端联调验收

### 10.2 前端执行态联调

- [ ] 构建页接入 BuildRun 实时状态和日志查看。
- [ ] 部署环境页接入 Release 状态、rollout 进度和回滚结果。
- [ ] 网关域名页接入 DNS 校验、HTTPRoute/Gateway、证书申请和续期状态。
- [ ] 使用 Chrome 验收仓库绑定、构建、镜像站、部署、域名完整链路。

## 11. 安全与后端结构优化

- [ ] 后续按领域继续拆分后端：将 Git/Registry/Auth/Project handler 的复杂流程逐步沉入 service/repository/provider，handler 保持 HTTP 适配层。

## 12. 可观测性

### 12.1 配置开关与安全边界

- [ ] 增加配置自检与系统设置展示：管理员可以看到每个可观测模块的启用状态、缺失配置键、采集端点和最后一次导出/查询错误；普通用户只看到可用的业务状态和受控跳转。

### 12.2 指标与 Prometheus/Grafana

- [ ] 在 `PROMETHEUS_QUERY_ENABLED=true` 且 `PROMETHEUS_BASE_URL` 已配置时，后端提供受控查询 API，为看板、项目空间概览、应用概览和部署页返回聚合后的轻量趋势；未配置时 UI 隐藏趋势图并保留业务状态。
- [ ] 在 `GRAFANA_LINKS_ENABLED=true` 且 `GRAFANA_BASE_URL` 已配置时，按页面上下文生成 Grafana dashboard 深链；未配置时不展示 Grafana 入口。

### 12.3 链路追踪

- [ ] 支持采样配置：`OTEL_TRACES_SAMPLER` 和采样比例环境变量；错误 trace、慢任务 trace 和构建/发布任务优先保留。
- [ ] 在 `GRAFANA_LINKS_ENABLED=true` 且 `GRAFANA_BASE_URL` 已配置时，为构建详情、发布详情和访问入口生成 Tempo/Trace 深链；未配置时不展示 trace 入口。

### 12.4 日志上报与查询

- [ ] 构建日志、Hook 日志、发布日志和运行 Pod 日志继续按业务权限在平台内展示；同时在启用日志导出时附加 trace_id、build_run_id、release_id 和 deployment_target_id，便于 Loki 聚合检索。
- [ ] 在 `LOKI_LINKS_ENABLED=true` 且 `LOKI_BASE_URL`/`GRAFANA_BASE_URL` 已配置时，后端为构建、发布、运行日志生成受控 Loki/Grafana Explore 深链；未配置时仅展示平台内日志。
- [ ] 规划大日志归档：对象存储归档仍独立于 Loki，启用归档需要单独显式开关和存储配置；未配置时只保留数据库日志窗口。

### 12.5 告警与用户体验闭环

- [ ] 提供 Prometheus alert rules：API 5xx、API P95 延迟、Worker 队列积压、构建失败率、发布失败率、Redis/PostgreSQL/Kubernetes 不可用、证书失败过多；规则文件随 Helm/Compose 示例提供，但不默认启用外部告警发送。
- [ ] 在 `ALERT_LINKS_ENABLED=true` 且 `ALERTMANAGER_BASE_URL` 已配置时，管理员页面展示 Alertmanager 入口和当前告警摘要；未配置时不展示告警入口。
- [ ] 平台看板增加可观测摘要：平台健康、队列积压、近期失败构建/发布、运行集群异常；所有摘要缺少 Prometheus 查询配置时回退到数据库业务记录。
- [ ] 项目空间和应用概览增加用户友好的可观测卡片：构建成功率、最近发布状态、副本健康、访问入口状态、资源趋势；趋势依赖 Prometheus 查询开关，状态依赖平台业务记录。
- [ ] 构建/发布失败自动关联最近日志、Kubernetes Events 和 trace_id，前端优先展示“可能原因 + 下一步操作”，深度日志和 trace 作为辅助入口。

## 16. Luna CLI

- [ ] 为 Agent 可调用 operation 补齐 JSON Schema Draft 2020-12 输入/输出契约和 `x-luna-cli` 风险、敏感字段、dry-run、并发、资源上限、批准元数据；实现 `agent=true`、`params=@file|@-`、按 query/category/risk/scope 受限发现和 Schema digest 漂移检测。
- [ ] 评估并实现短时单次服务端计划作为高风险操作的附加安全层；当前 CLI
  `high`/`critical` 操作采用交互逐次确认，非交互或 Agent 显式 `--yes`，且仍由
  绑定 actor、认证上下文、项目、目标、规范化参数和资源版本。
- [ ] 为中高风险更新补齐 ETag/version/resourceVersion 乐观并发控制；CLI 和 Skills 遇到冲突时必须重新读取、重新计划并再次确认，不允许盲覆盖或自动追加 `force`。
- [ ] 定义版本化 JSONL 长任务事件协议：首帧版本、sequence/eventId/correlationId/operationId/resourceRef、恢复游标、资源上限和唯一终态摘要；缺少摘要时不得报告成功。
- [ ] 实现由平台固定初始化、无需动态注册的内置 OAuth 公共 CLI Client：Token Endpoint 支持 `token_endpoint_auth_method=none`，仅允许严格 loopback redirect，完成 Authorization Code + PKCE、刷新和吊销。
- [ ] 为 Git Provider OAuth 增加短时授权事务创建/查询接口，回调写入事务终态；`luna git authorize` 打开浏览器并返回确定的 Git Account ID，不通过轮询账号列表猜测授权结果。
- [ ] 确认 npm `@liteyuki` 组织权限，使用 2FA 手动发布首个 `@liteyuki/luna-cli` public 预发布包；随后配置 Trusted Publisher 和 GitHub `npm` Environment，并以新的未发布版本完成真实 OIDC 发布验收。
- [ ] 接入 Apple Developer ID 和公证；macOS 制品完成平台代码签名后才可进入稳定矩阵。

## 17. 内嵌 AI 助手

- [ ] P3：评估显式长期记忆、项目空间共享知识和外部 MCP 扩展；默认不启用。
- [ ] 建立跨用户、跨项目、权限变化、提示注入、批准重放、Secret 脱敏、成本和 Agent 故障恢复测试门禁。

## 100.优化需求

- [ ] 智能引导：例如用户在创建APP选择Git账号时发现没有账号，旁边用一个按钮引导去授权页面。这样的场景还有很多，不一定是Git账号，后续可以总结一批这样的场景进行统一优化。
