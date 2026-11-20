# Luna DevOps AI 助手与 Agent 规格

本文是 AI 助手方向的方案与决策记录。现行协议契约（交互卡片 Schema）独立维护在
[`12-AI声明式交互卡片Schema.md`](12-AI声明式交互卡片Schema.md)；已实现的具体数据结构、
接口字段与目录结构以代码和 OpenAPI 为唯一事实源，本文不再逐项罗列。

## 1. 产品定位

AI 助手不是悬浮在控制台上的通用聊天机器人。它理解用户当前所在页面、项目空间、应用和
部署配置，帮助用户：

- 解释构建、发布、Pod、Gateway、证书和通知问题。
- 汇总分散在多个页面中的状态、事件和日志。
- 把用户带到正确页面、Tab 或资源详情。
- 为表单预填安全的非密钥参数，由用户确认后保存。
- 按平台三级工具审批策略（`L0` 只读自动、`L1` 可逆常规非生产变更支持本会话同操作记忆、`L2` 高危与生产变更逐次参数绑定批准）受控执行平台操作。
- 保留可恢复的会话和执行记录，刷新页面不丢失诊断进度。

核心原则：**助手代表当前用户工作，不拥有超出当前用户的权限。**

## 2. 核心架构决策

- 独立 `luna-agent` 服务承载模型编排，与 `cmd/api`、`cmd/worker` 分离；当前产品阶段固定单副本，
  活动 Run 的短期增量经有界进程内流传输，已提交时间线和终态事实落 PostgreSQL。不承诺滚动发布期间
  的跨 Pod SSE、取消或执行连续性。
- `luna-agent` 与 `cmd/api` 之间不把 MCP 作为内部服务总线，统一走内部零信任 HTTP 网关；同时 `cmd/api` 在 `/api/v1/mcp` 暴露原生 Streamable HTTP MCP Server，与内置 `luna-agent` 共享同一套 `internal/aitool` 领域门面注册表、随包公开文档知识库与根目录 `skills/`，供外部 IDE/CLI Agent 使用 PAT 或 OAuth 2.0（`authorization_code` + PKCE `S256`）直接接入（含未启用网页内置 AI 助手的实例）。
- 编排采用显式 ModelRuntime + RunExecutor：ModelRuntime 只负责上下文编译、工具解析和 Provider 调用；RunExecutor 负责单 Run 循环、工具执行、审批、取消和调用上限。
- PostgreSQL `ai` schema 保存 Run 接纳、用户输入、工具审批与结果、模型步骤终态、最终 Item、用量和
  Run 终态；模型输出中的正文与思考 delta 只进入当前 Agent 的有界内存流，不逐片写数据库。
- 每次模型输出完成、失败或取消时，Agent 在一个事务中写入最终 Item 修订、必要的终态事件并推进
  Run 的事件 sequence 高水位；`run_events` 因此只保存可审计的工作流与终态事实，sequence 可以因
  未持久化的临时 delta 而稀疏。初始用户消息、工具终态、标题来源与锁定状态仍按原有事务边界持久化。
  活动 Run 期间的人工标题修改以 PATCH 返回的 Conversation 和后续权威回读为准，不向该 Run 追加
  PostgreSQL 事件，避免与进程内活动流形成两个 sequence 分配器；自动标题则进入最终事务。
- 进程内活动流只承担当前副本上的实时传输和短时断线回放，终态提交后保留 5 分钟；单事件限制
  256 KiB，单 Run 限制 131072 个事件和 64 MiB，进程总量限制 512 MiB。正文与思考帧只传当前增量，
  避免长输出形成 O(n²) 缓冲。超过限额时必须失败，不能继续执行一个前端无法观察的 Run。
- Web 以服务端 Timeline 快照在 TanStack Query 中的投影作为唯一事实源；每个 Run 最多保留一条
  SSE 连接，事件直接合并到查询缓存；同一进程内刷新按 `after`/`Last-Event-ID` 从活动流补发，sequence
  缺口或连接停滞先回读权威 Timeline 再重连。进程重启时未完成 Run 收敛为 `interrupted`，未确认的
  partial delta 不作为完成结果。显式 `stream.heartbeat` 不带事件 ID、不进入 Reducer，也不推进业务
  sequence。
- Agent 按 Run 共享一个进程内 reader 和一个 PostgreSQL 终态 watcher，再向本地 SSE 连接扇出；不会
  按浏览器标签页线性增加数据库轮询。每 Run 最多 4 个、每实例最多 64 个 SSE 订阅；单订阅待发送
  队列最多 256 个事件或 512 KiB，慢订阅者只断开自身。
- 终态写入固定最初的 completed/failed/canceled/interrupted 意图，以同一幂等 batch 最多尝试 3 次
  指数退避；已提交但响应未知的重试不得重复事件或改变终态。确定性的状态冲突和契约错误不重试，
  也不得把 completed 降级伪装成 failed。
- Agent 只持有一个服务身份，不持有用户 Cookie、Token 或可转授权限。创建 Run 时保存 API 已验证的
  `actor_session_id`；工具请求只携带服务凭据、`runId` 和 `toolCallId`。Luna API 从 PostgreSQL 回读
  用户、会话、会话所有权、项目范围、ToolCall 与审批状态，再进入原有 Handler/Service/RBAC。

## 3. 执行与工具治理

- Agent Loop：补充上下文 → 模型判断 → 工具调用（经 Schema 与分级审批校验）→ 结果回灌 → 直至
  最终答复或明确终态（批准/补充输入/后台任务等待/取消/超时/上限）。
- 工具审批采用 `L0 / L1 / L2` 三级策略，并结合会话级持久化的四档执行权限模式（`ai.conversations.approval_mode`：`strict / default / auto / full_access`，新建会话默认 `default`）：
  - `L0`（纯只读与发现工具，`requiresApproval=false`）：在全部模式下自动执行，不打断用户，也不写危险操作审计噪音；
  - `L1`（可逆常规非生产写操作）：`default` 模式下支持“拒绝”“批准本次（`approve`）”与“批准并在本会话记住（`approve_and_remember`）”，会话记忆保存在 `ai.conversation_tool_approvals`；`strict` 模式下禁用会话记忆并强制逐次批准；一旦目标涉及 `stage=prod`（含实库解析目标 `DeploymentTarget.Stage`）或服务端返回 `428 ai.approval_required` 则立即升级为 `L2`；
  - `L2`（删除、凭据写入、脚本 Hook、构建/发布/回滚、网关路由、集群/RBAC/计费/系统配置及所有 `stage=prod` 变更）：`default` 与 `strict` 模式下逐次预检展示参数对比并等待 `approve`；`auto` 模式下对 `CatalogOperation.AutoModeExempt=true` 的非生产常规交付/网络/卷/通知写操作自动执行预检签发 `X-Luna-AI-Preview-Lock` 并以 `approval_decision='mode_auto'` 放行，继续拦截删除/移除/解绑/吊销、生产环境（`prod`/`production`）、容器远程命令（`runtime_exec_command`）及平台/集群治理类操作；`full_access` 模式下在用户自身 RBAC 与单项目会话沙箱内对全部操作自动完成预检锁签发并以 `approval_decision='mode_full_access'` 放行。Go 零信任网关在执行入口实时回查 `ai.conversations.approval_mode`、预检锁与目标部署环境，拒绝越档伪造。拒绝只终结当前 ToolCall，让模型继续。
- 异步长任务（如构建 `build_run`、发布 `release`）采用双模式：
  - 默认单步触发：平台写工具返回已受理 ID 与初始状态后立即回灌模型，模型生成 `present_card`（`template: live_task`，绑定 `live_progress`）并正常 `completed` 收口本轮，交由前端实时订阅进度；
  - 多步串联编排或失败自动排障：模型显式调用内置工具 `wait_for_task`，执行器将 Run 置为 `waiting_external` 并写入指数退避 `wake_at` 立即释放执行槽位；到达终态（含 `succeeded` / `failed` / `canceled`）或超时后写回工具结果并将 Run 置回 `queued` 唤醒模型继续。
- 取消 Run 时先把 PostgreSQL 终态推进为 `canceled`，并向当前模型请求和正在执行的平台工具 HTTP 请求
  传播 `AbortSignal`。外部系统已经提交的副作用无法由断开 HTTP 连接回滚，因此取消只保证 Run 终态
  不被迟到结果覆盖；业务副作用仍必须通过对应工具约定的权威回读确认。
- queued Run 通过 `FOR UPDATE SKIP LOCKED` 原子抢占为 `running` 并写入 `lease_expires_at` 租约，执行期间后台续租（提交暂停或终态前立即停表），所有状态、工具与流式写入均受 `row_version` 乐观围栏保护；若续租发现 `row_version` 漂移（失租），旧执行器仅本地中止退出，绝不无围栏改写新执行器接管的 Run。租约过期且带有效 `ai.run_checkpoints` 的孤儿 Run 由对账重新入队恢复（上限 3 次），其余置为 `interrupted`。
- 卡片时间线保留真实事件顺序。`placement: turn_end` 只在渲染层把单张、阻塞后续流程的交互表单
  投影到本轮末尾；默认使用 `inline`。多卡片、展示卡、进度卡或一轮出现多个末尾卡片时保持事件原位。

### 3.1 工具目录下发与按需检索

- 后端 `internal/aitool` 强类型领域门面注册表（Code-First `Spec[In, Out]`，7 大业务域共 166 个工具）是内置 `luna-agent` 与公开 `/api/v1/mcp` MCP Server 的唯一平台工具事实源；不直接 1:1 暴露公开 REST API。内置 Agent 工具调用统一经内部零信任网关 `POST /internal/v1/ai/tools/preview`（`L2` 只读预检与 HMAC `X-Luna-AI-Preview-Lock` 签发）与 `POST /internal/v1/ai/tools/execute` 执行，公开 MCP 网关则按 Bearer PAT / OAuth Scope 鉴权并通过 `preview_tool -> previewLock -> execute_tool` 共享同一套预检锁、领域 Handler/Service、RBAC 与 AuditLog。
- 不存在 `x-luna-agent.allowed`、Agent 手写平台 fallback、手工 base operation 白名单、重复描述、
  predecessor/followup 图、JSON Pointer verifier 或 readback engine。
- `search_tools` 的 query 可空；空查询只分页浏览完整轻量目录，非空查询使用带字段权重的 BM25F
  对 operationId、动作、资源、名称、用途、标签、中英文别名、参数、输入输出字段、禁用场景、
  前置条件和成功回读排序。每次非空检索自动把前 5 个候选加入当前 Run，单次最多 8 个；结果不含
  Schema，模型可直接调用已经加入的操作，不必机械地再查详情。
- `get_tool_details` 每次接收 1–8 个精确 operationId，只返回用途、禁用场景、前置条件、成功回读、
  Scope、审批要求和参数语义等紧凑消歧信息；完整输入/输出 Schema 只在执行器与实际模型工具定义中使用，不进入详情结果。没有 embedding、多向量、RRF、reranker 或影子检索。
- 每个 Run 持久化按最近使用排序的 `selectedOperationIds`，最多保留 16 个平台操作；检索、详情、
  实际调用、审批和输入恢复都会沿用同一集合，内部工具不计入。重复的相同检索或详情请求返回首次
  命中的缓存结果并明确 `cacheHit/duplicate`，不得伪装成空结果。淘汰只影响后续模型工具定义，
  不删除历史 ToolCall 或事件。
- 单一 `RemoteConfigSnapshot` owner 负责拉取、验证并原子提交 Provider、运行参数和 Catalog；`ToolCatalogRegistry` 按 `adminOnly` 元数据维护 `tenant`（133 个）与 `admin`（166 个）两份确定性角色隔离快照及独立 `toolCatalogDigest`，执行器在 Claim 与恢复时实时回查用户角色并在降权时裁剪历史 `selectedOperationIds`。
- 所有写工具采用纯增量 Patch 语义（未传或 `null` 视为保持现状），敏感字段通过独立字段声明 `writeOnly + x-luna-sensitive` 并在持久化、Checkpoint、事件流与模型出站全链路脱敏；`L2` 工具在进入 `awaiting_approval` 前统一调用 `preview` 拦截 `409` 冲突并向审批卡片输出 `before -> after` 对比摘要。

## 4. 记忆与上下文

- 首版只提供会话内短期记忆，不自动建立跨会话长期记忆。
- 上下文固定以确定性的核心系统提示（含常驻交互与站内导航规则及领域工作流索引）开头以保护上游 Prompt Cache 前缀，随后组装结构化滚动摘要、近期完整轮次原文与当前轮工具消息（含按需调用的 `load_skill` 领域规程）。
- **全链路严禁本地按字节数、字符数或固定比例猜测 Token 数**。每次发起模型调用前仅依据最近一次模型响应返回的权威 `usage.totalTokens` 判定水位：当 `lastUsage.totalTokens > model.maxContextTokens - reserveTokens`（或上游明确返回结构化上下文超限错误）时触发 `Compaction` 生成结构化摘要推进切点，同时保留近期完整轮次原文；无可压缩历史时返回 `ai.model_context_insufficient`。物理字节上限仅用于用户输入防 DoS（`maximumTurnInputBytes`）、内存流缓冲（`InMemoryRunStreamBus`）与 OTLP 属性保护，绝不参与上下文截断或计费推导。
- 禁止进入上下文与记忆：Secret、Token、Cookie、Authorization、kubeconfig、Registry 密码、
  Git Access Token、完整终端历史、未脱敏的第三方响应与日志。工具入参密文与运行执行快照使用 AES-256-GCM（绑定主键 AAD，支持 `AI_INTERNAL_SECRET_PREVIOUS` 平滑轮换）加密存储，快照不落盘明文 `apiKey` 或全量工具目录。

### 4.1 模型能力、权威 Usage 门禁与异步流水结算

- 模型目录的最大上下文、最大单次输出和 Prompt/Completion/缓存 Prompt 三类价格由 `internal/aimodel`
  统一校验；Run 创建时保存模型身份、能力与价格快照，后续管理变更不影响历史 Run。
- 上游 Provider 请求固定开启 `include_usage: true`，忽略流式中间或尾随 `usage: null` 帧，兼容 OpenAI/DeepSeek 与 Claude/Gemini 网关口径归一化为 `{ input, output, cacheRead, cacheWrite, totalTokens }`。只要完成出口缺少合法官方 `usage`，立即抛出 `ai.provider_usage_unavailable` 终止请求并向前端透传明确提示，绝不静默放行或本地估算。
- 计费与模型调用链解耦，不设 `ai.model_credit_holds` 事前锁表预冻结：创建 Turn 入口无锁校验用户钱包可用余额 `> 0`（`balance_credits - pending_spend > 0`）；每步模型调用凭官方 `usage` 直接写入不可变 `ai.model_usages`（状态 `pending`，与会话物理删除解耦保留账本）；后台 Worker 按用户单事务聚合批量实扣。AI 费用只归属发起用户个人钱包，`project_id` 始终为空。
- 用户单轮文本固定为 128 KiB UTF-8；Web/API 原始 JSON 信封最多 1 MiB，Agent 内部信封最多 2 MiB。

### 4.2 Provider 协议方言与思维链回传

- 请求方言按"显式配置 → 官方 DeepSeek 端点 → 模型名"解析，`providerDialect(config, modelName)`
  是唯一入口：`auto` 在网关隐藏上游地址时按模型名识别 DeepSeek 系列，混合代理的网关无需逐渠道配置；
  Provider 连接测试回报首个模型实际生效的方言。Agent 每 30 秒拉取一次配置，方言变更只影响之后
  新开始的 Run（Run 在 claim 时冻结快照），配置版本变化会写一条 `agent.provider_config.updated`。
- 协议消息的 `reasoning` 字段保存该轮推理原文，只有需要回传思维链的方言写入请求；OpenAI 口径从
  构造上不产生该字段，也不会把响应里的 `reasoning_content` 当作协议字段解释。
- DeepSeek 思考模式下，仅对当前未结束轮次（最后一条 `user` 消息之后）的 `assistant` 工具子步骤回传原样 `reasoning_content`，已完成的历史轮次不回传推理文本以保护上下文窗口与缓存命中；网关在当前轮子步骤未回传推理原文时回填单空格 `" "`（穿透部分代理 DTO 的 `omitempty` 空串丢弃），绝不伪造中文占位或把正文冒充思维链。上游拒绝原因以脱敏截断形式写入 `provider.error.message` 日志字段。
- 推理原文计入 `gen_ai.output.messages` 的 reasoning 部分；高敏内容采集（`AI_OBSERVABILITY_CAPTURE_CONTENT=true`）入口叠加工具 `sensitivePaths` 遮罩与常见凭据特征脱敏。

## 5. 安全与 Prompt Injection 防护

以下内容一律视为不可信数据：仓库文件、README、Issue、构建/Pod 日志、Kubernetes Events、
镜像标签、提交消息、用户输入、Webhook、第三方 API 和外部网页内容。

防护要求：

- 系统指令和工具策略不与不可信内容拼成同一权限层。
- 模型看到的日志使用明确数据边界和长度上限。
- 网页读取仅允许无凭据的只读 HTTP/HTTPS GET，限制响应类型、大小、重定向次数与文本长度，
  重定向逐跳重新校验；直连请求在拨号时校验并固定同一解析 IP，代理请求在交给受信代理前校验
  目标；两者都由站点级域名/IP 黑名单和端口策略约束。
- 并发 Run 与请求速率按现有站点/用户边界限制；AI Token 和费用按 Run 快照与发起用户
  个人钱包限制，不关联项目空间计费归属。

## 6. 明确不做

- 首版不实现多 Agent 自主讨论或投票。
- 首版不自动执行用户未批准的写操作，不共享项目空间会话，不建立跨会话长期记忆。
- 不允许任意脚本、SQL 或文件系统工具。
- 不用 CLI 子进程作为内嵌助手的内部执行层。

## 7. Agent 一次性运行命令

Agent 只有在结构化状态、事件和受控日志不足以形成诊断证据时，才调用一次性、非 TTY 的
`execReleaseRuntimeCommand`；浏览器 Web Console 继续使用原有 WebSocket + TTY 协议，两者不复用
传输层。

- 每条命令固定绑定当前用户、登录 Session、Agent Run、项目空间、Release、部署配置和容器，并对
  当前参数逐次批准；后续命令不得复用上一次批准。
- API 每次重新执行当前账号、Session、RBAC、项目开关、工具策略、参数 Schema 和审计检查，不保留
  跨命令工作目录、环境变量或 Shell 状态。
- 审计只记录命令字节数，不保存命令正文或可用于离线猜测的摘要；命令参数作为敏感工具输入处理，stdout 与 stderr
  使用合计上限并返回截断标志。
- 需要修改部署、配置、网关或运行态资源时使用对应业务工具，不通过命令执行绕过批准、审计和幂等。

## 8. 渐进式领域 Skill 与高频诊断聚合

Agent 与 `/api/v1/mcp` 共享根目录单一事实源 `skills/`，内置 Agent 采用“常驻核心交互与站内导航规则 + 按业务域按需加载领域 Skill（内置 `load_skill` 工具）”的两层架构：
- 常驻层（`luna-devops-interaction`、`luna-devops-navigation`）固定在系统提示前缀中，保证多步工具调用与跨轮对话的 Prompt Cache 前缀稳定，仅维护通用安全边界、卡片/路由契约与 7 大业务域索引（`workspace-iam`、`source-registry-build`、`application-deployment`、`runtime-volumes`、`gateway-topology`、`diagnostics-governance`、`platform-admin`）；外部 MCP 客户端则由 `skills/luna-devops/SKILL.md`（或 `luna://skills/*` 资源）承担统一入口路由。
- 领域层通过 `load_skill` 在当前 Run 内按需加载 1–3 个业务域规程并自动去重；排障场景优先调用只读诊断聚合工具（`getProjectDiagnostics`、`getDeploymentTargetDiagnostics`）一次性获取实时状态、近期异常构建/发布、异常 Pod、告警事件与网关路由状态，再沿交付链首个异常边界下钻具体日志。

Skill 已覆盖不代表对应写工具已在 Tool Catalog 开放；工具未注册时 Skill 必须阻止模型虚构执行，明确报告"尚未执行"。

变更工具目录、Schema、Scope 或审批分级时，至少验证完整目录分页、覆盖全部可用 operation 的中英文检索矩阵、相似读写操作边界、Run 内自动加载与 16 项 LRU、精确详情缓存、目录热更新隔离、未加载工具不可见、集中排除原因、`L0/L1/L2` 三级审批与会话级记忆隔离、跨用户/Session/项目空间隔离，以及真实副作用和权威回读。检索词不得进入普通遥测属性、日志字段或 Metric label。

## 9. 参考与事实源

- 交互卡片协议契约：[`12-AI声明式交互卡片Schema.md`](12-AI声明式交互卡片Schema.md)
- 工具注册闭环与调用链约束：`AGENTS.md`
- 可观测插桩：[`可观测和插桩规范.md`](可观测和插桩规范.md)
- 已实现的数据模型、接口字段、目录结构：以代码和 OpenAPI 为准
