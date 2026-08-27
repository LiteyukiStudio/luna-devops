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
- 在用户批准当前高风险 ToolCall 后才执行受控的平台操作；每个新 ToolCall 都按当前参数单独决策。
- 保留可恢复的会话和执行记录，刷新页面不丢失诊断进度。

核心原则：**助手代表当前用户工作，不拥有超出当前用户的权限。**

## 2. 核心架构决策

- 独立 `luna-agent` 服务承载模型编排，与 `cmd/api`、`cmd/worker` 分离；当前产品阶段固定单副本，
  活动 Run 的短期增量经有界进程内流传输，已提交时间线和终态事实落 PostgreSQL。不承诺滚动发布期间
  的跨 Pod SSE、取消或执行连续性。
- 平台能力以 OpenAPI 和既有 API 为唯一业务入口；MCP 不作为内部服务总线，仅保留为未来
  连接外部工具的可选协议。
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
  按浏览器标签页线性增加数据库轮询。每 Run 最多 64 个、每实例最多 512 个 SSE 订阅；单订阅待发送
  队列最多 2048 个事件或 4 MiB，慢订阅者只断开自身。
- 终态写入固定最初的 completed/failed/canceled/interrupted 意图，以同一幂等 batch 最多尝试 3 次
  指数退避；已提交但响应未知的重试不得重复事件或改变终态。确定性的状态冲突和契约错误不重试，
  也不得把 completed 降级伪装成 failed。
- Agent 只持有一个服务身份，不持有用户 Cookie、Token 或可转授权限。创建 Run 时保存 API 已验证的
  `actor_session_id`；工具请求只携带服务凭据、`runId` 和 `toolCallId`。Luna API 从 PostgreSQL 回读
  用户、会话、会话所有权、项目范围、ToolCall 与审批状态，再进入原有 Handler/Service/RBAC。

## 3. 执行与工具治理

- Agent Loop：补充上下文 → 模型判断 → 工具调用（经 Schema 与审批校验）→ 结果回灌 → 直至
  最终答复或明确终态（批准/补充输入/取消/超时/上限）。
- 工具只有 `requiresApproval` 一个审批字段。普通工具直接执行；高风险工具支持拒绝或批准本次，
  不保存跨 ToolCall 的永久豁免。拒绝只终结当前 ToolCall，让模型继续。
- 取消 Run 时先把 PostgreSQL 终态推进为 `canceled`，并向当前模型请求和正在执行的平台工具 HTTP 请求
  传播 `AbortSignal`。外部系统已经提交的副作用无法由断开 HTTP 连接回滚，因此取消只保证 Run 终态
  不被迟到结果覆盖；业务副作用仍必须通过对应工具约定的权威回读确认。
- queued Run 通过 `FOR UPDATE SKIP LOCKED` 原子更新为 running，并用 PostgreSQL `row_version` 约束
  本次执行的终态提交，迟到写入不能覆盖已变化的 Run。单副本进程正常停止时把正在执行的 Run 收敛为
  `interrupted`；启动时只处理上次进程遗留的 running Run，不从 Provider 或工具调用栈中间恢复执行。
- 卡片时间线保留真实事件顺序。`placement: turn_end` 只在渲染层把单张、阻塞后续流程的交互表单
  投影到本轮末尾；默认使用 `inline`。多卡片、展示卡、进度卡或一轮出现多个末尾卡片时保持事件原位。

### 3.1 工具目录下发与按需检索

- 后端 OpenAPI 注册操作是平台工具唯一事实源。满足普通 JSON 业务操作结构的路由自动进入目录；
  协议适配器、认证回调、凭据与计量入口由 `operationId -> reason` 集中禁用表排除。
- 不存在 `x-luna-agent.allowed`、Agent 手写平台 fallback、手工 base operation 白名单、重复描述、
  predecessor/followup 图、JSON Pointer verifier 或 readback engine。
- `search_tools` 的 query 可空；空查询只分页浏览完整轻量目录，非空查询使用带字段权重的 BM25F
  对 operationId、动作、资源、名称、用途、标签、中英文别名、参数、输入输出字段、禁用场景、
  前置条件和成功回读排序。每次非空检索自动把前 5 个候选加入当前 Run，单次最多 8 个；结果不含
  Schema，模型可直接调用已经加入的操作，不必机械地再查详情。
- `get_tool_details` 每次接收 1–8 个精确 operationId，只返回用途、禁用场景、前置条件、成功回读、
  Scope、审批要求和参数语义等紧凑消歧信息；HTTP 方法、路由和完整输入/输出 Schema 只在执行器与
  实际模型工具定义中使用，不进入详情结果。没有 embedding、多向量、RRF、reranker 或影子检索。
- 每个 Run 持久化按最近使用排序的 `selectedOperationIds`，最多保留 16 个平台操作；检索、详情、
  实际调用、审批和输入恢复都会沿用同一集合，内部工具不计入。重复的相同检索或详情请求返回首次
  命中的缓存结果并明确 `cacheHit/duplicate`，不得伪装成空结果。淘汰只影响后续模型工具定义，
  不删除历史 ToolCall 或事件。
- 单一 `RemoteConfigSnapshot` owner 负责拉取、验证并原子提交 Provider、运行参数和 Catalog；
  `ManagedProvider` 只按已提交的 version/model 缓存构造后的客户端，不再自行刷新远端配置。Run 创建时
  快照当前 Catalog digest；配置刷新先完整构建和校验新目录，再切换当前版本，并保留仍被活动 Run
  引用的旧目录。新 Run 使用新 digest，旧 Run 始终使用原 digest；非法候选或短时断联不替换上一份
  有效快照。
- Catalog 必须从真实 OpenAPI operation 归一化 operationId、用途、标签、别名、审批要求、幂等性、
  HTTP 路由、Scope、输入/输出 Schema、敏感路径和传输参数。目录只负责发现，最终权限仍由 Luna API
  权威回读用户、Session、会话、项目空间、ToolCall 与审批状态后进入原 Handler/Service/RBAC。
- OpenAPI path item 与 operation 两层的 parameters 必须按 `in + name` 合并，operation 层覆盖同名定义；
  Agent 可调用 Handler 直接消费的 `Query`、分页、排序、搜索和布尔查询参数必须全部进入模型可见输入
  Schema。契约测试需要从 Handler 源码反向核对这些字段，禁止依赖 Handler 宽松读取或模型重试掩盖
  OpenAPI 漏项。
- 应用市场工具使用两阶段契约：`listAppTemplates(query?, category?)` 只返回可检索摘要，选定真实
  `templateId` 后调用 `getAppTemplate(templateId)` 获取安装参数。列表无匹配时返回空数组，不视为工具
  失败；详情不得回显内部渲染字段，Secret 参数的默认值必须清空。
- 普通 `/api/v1` JSON operation 自动纳入目录；OAuth/OIDC 回调、认证凭据、文件传输、SSE、
  WebSocket 和其他特殊协议必须在集中 `operationId -> reason` deny map 中排除并测试。不得在 Agent
  维护重复白名单、执行路由或权限 fallback。
- 部署配置的 `clusterId` 为空表示平台默认集群：只有存在多个候选且必须由用户决定时才用
  `listRuntimeClusters` 的真实结果询问，禁止无候选凭空问“选择哪个集群”。
- 模型未来只使用 `present_card`、`request_input`、`request_choice` 三个通用交互工具；回复完成后
  不再额外调用模型预测下一步。
- 运行时密钥只能通过 `updateDeploymentTargetRuntimeSecrets` 或
  `updateProjectRuntimeConfigSetRuntimeSecrets` 处理：请求使用 `items[]`，每项必须声明
  `valueMode: secret` 和 `operation: set | generate | clear`。`set` 的非空 `value` 可由普通模型工具
  调用或用户可见安全表单触发的 Direct Tool Action 提交，空值表示不修改；`generate` 由平台后端
  生成并直接写入 Secret Store，`clear` 只清除明确字段。`inputMode` 只记录调用来源，不得作为执行
  门禁；聊天消息、最终回复和页面上下文仍不主动回显敏感值。成功结果仅返回键、`valueMode` 与
  configured/generated/cleared 状态，不返回明文或 Secret 引用。部署目标和运行配置集的普通 `environmentVariables[]` 每项必须声明
  `valueMode: public`；密钥语义字段名和带 URL 内嵌凭据的值作为纵深防御继续拒绝。
- 模型不得为交互卡片中的 `secret` 或 `key_value.valueMode: secret` 字段提供 `defaultValue`、示例密钥或任何
  预填明文；Web 对修复前持久化记录继续强制清空。用户没有主动输入时 Direct Tool Action 不
  提交该字段，随机生成与清除分别使用后端 `generate` 和独立 `clear` 操作。

## 4. 记忆与上下文

- 首版只提供会话内短期记忆，不自动建立跨会话长期记忆。
- 上下文先放稳定的核心系统提示，再把本轮目标命中的 Skill/参考以独立、较后的 system message
  加入，然后组装滚动摘要、近期完整原文和当前工具结果。新会话不做本地 Token 预检，直接
  请求 Provider。压缩只由上一次同模型官方 `prompt_tokens / maxContextTokensSnapshot`、未压缩
  轮次上限或结构化上下文超限错误触发。
- 上下文超限时先压缩更旧历史，再以新 Provider attempt 重试；没有可压缩历史时返回
  `ai.model_context_insufficient`。摘要批次超限时按轮次二分，单轮仍超限才按明确字节上限分段。
  字节上限只用于传输与内存保护，不生成 Token 数据。鉴权、额度、Provider 不可用和用量契约错误不允许
  被 fallback 掩盖。
- 禁止进入上下文与记忆：Secret、Token、Cookie、Authorization、kubeconfig、Registry 密码、
  Git Access Token、完整终端历史、未脱敏的第三方响应与日志。

### 4.1 模型能力、用量预留与结算

- 模型目录的最大上下文、最大单次输出和 Prompt/Completion/缓存 Prompt 三类价格由 `internal/aimodel`
  统一校验；Run 创建时保存模型身份、能力与价格快照，后续管理变更不影响历史 Run。
- Run 不设置累计 Token 或 Credits 预算。每次实际 Provider attempt 调用前在 `ai.model_credit_holds`
  原子创建独立 hold，只保存模型硬上限与价格快照推导的最大信用风险，不保存或推导实际 Token。
  SDK/HTTP 模型客户端不自动重试；上层因结构化上下文错误重试时，每次都创建新 attempt 和 hold。
- `ai.model_usages` 只能由严格校验通过的 Provider 官方 `usage` 创建：`prompt_tokens`、
  `completion_tokens`、`total_tokens`、可选 cached/cache-write Prompt 和 reasoning Completion 明细。缺失、
  类型/范围/关系非法或调用结果不可知时，hold 进入 `reconciliation_required`，不写入可结算 usage。
  明确被 Provider 拒绝或未发送的请求可释放 hold；过期且无法确定结果的 hold 进入对账，不会复制风险上限
  伪造 Token。
- Provider 官方用量超过 hold 假设时保留原值，hold/usage 进入 `hold_deficit` /
  `reconciliation_required`。Worker 只选取 `reported + pending` 且 hold 为 `usage_recorded` 的记录结算。普通
  Prompt 从官方关系扣除 cached/cache-write 子集，Completion 已包含 reasoning，不重复扣费。AI 费用
  只属于发起用户个人钱包，
  `project_id` 始终为空。钱包普通 debit 和负 adjustment 都要扣除未结束 hold。
- 全链路锁序为 wallet → Run → hold（无 Run 的结算/普通扣费为 wallet → hold）。Run 锁只用于
  attempt 顺序、归属与模型快照一致性，不承载累计预算。
- 工具结果上限、压缩触发比例、保留轮次和历史/摘要/续跑负载字节上限是 Agent 进程内无状态策略，
  只允许通过可选环境变量覆盖；平台数据库与动态配置 API 不存储或下发 Token 输入/摘要预算。

## 5. 安全与 Prompt Injection 防护

以下内容一律视为不可信数据：仓库文件、README、Issue、构建/Pod 日志、Kubernetes Events、
镜像标签、提交消息、用户输入、Webhook、第三方 API 和外部网页内容。

防护要求：

- 系统指令和工具策略不与不可信内容拼成同一权限层。
- 模型看到的日志使用明确数据边界和长度上限。
- 网页读取仅允许无凭据的只读 HTTP/HTTPS GET，限制响应类型、大小、重定向次数与文本长度，
  重定向逐跳重新校验；目标由站点级域名/IP 黑名单和端口策略约束。
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
- 审计只记录命令长度和 SHA-256，不保存命令正文；命令参数作为敏感工具输入处理，stdout 与 stderr
  使用合计上限并返回截断标志。
- 需要修改部署、配置、网关或运行态资源时使用对应业务工具，不通过命令执行绕过批准、审计和幂等。

## 8. 运行时与 Skill 覆盖

Agent 通过 Skill 引导完成平台工作流。Skill 以公开使用文档、控制台主要页面和业务 API 的高频
用户旅程为覆盖口径，一个工作流需同时具备"发现目标 → 收集参数 → 真实操作（缺工具时明确阻塞）→
处理单次批准/冲突/异步 → 权威回读 → 给出终态结论"才计为已覆盖。

Skill 已覆盖不代表对应写工具已在 Tool Catalog 开放；工具未注册时 Skill 必须阻止模型虚构执行，
明确报告"尚未执行"。

变更工具目录、Schema、Scope 或审批要求时，至少验证完整目录分页、覆盖全部可用 operation 的中英文
检索矩阵、相似读写操作边界、Run 内自动加载与 16 项 LRU、精确详情缓存、目录热更新隔离、未加载
工具不可见、集中排除原因、普通与高危工具、拒绝/单次批准、跨用户/Session/项目空间
隔离，以及一条真实副作用和权威回读。检索词不得进入普通遥测属性、日志字段或 Metric label。

## 9. 参考与事实源

- 交互卡片协议契约：[`12-AI声明式交互卡片Schema.md`](12-AI声明式交互卡片Schema.md)
- 工具注册闭环与调用链约束：`AGENTS.md`
- 可观测插桩：[`可观测和插桩规范.md`](可观测和插桩规范.md)
- 已实现的数据模型、接口字段、目录结构：以代码和 OpenAPI 为准
