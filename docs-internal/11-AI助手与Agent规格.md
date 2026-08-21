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
- 在用户批准当前高风险 ToolCall 后才执行受控的平台操作；除非当前用户已对该
  operationId 设置可撤销豁免，每个新 ToolCall 都单独决策。
- 保留可恢复的会话和执行记录，刷新页面不丢失诊断进度。

核心原则：**助手代表当前用户工作，不拥有超出当前用户的权限。**

## 2. 核心架构决策

- 独立 `luna-agent` 服务承载模型编排，与 `cmd/api`、`cmd/worker` 分离；无状态可水平扩容。
- 平台能力以 OpenAPI 和既有 API 为唯一业务入口；MCP 不作为内部服务总线，仅保留为未来
  连接外部工具的可选协议。
- 编排采用显式 ModelRuntime + RunExecutor：ModelRuntime 只负责上下文编译、工具解析和 Provider 调用；RunExecutor 负责单 Run 循环、工具执行、审批、取消和调用上限。
- 数据落 PostgreSQL `ai` schema；事件按 Run 单调递增 sequence 持久化后再可读，SSE 支持断线续传。
- `run_events` 是不可变事实源：初始用户消息以 `run.input_received` 携带完整 Item 修订；工具终态在
  同一事务中保存 `tool_call` 修订、独立 `tool_result` 和事件；自动/手动标题事件记录来源与锁定状态。
  `ai.items` 与会话标题只是可重建投影，不能替代原始事件。
- Web 以服务端 Timeline 快照在 TanStack Query 中的投影作为唯一事实源；每个 Run 最多保留一条
  SSE 连接，事件直接合并到查询缓存，序号缺口通过权威快照恢复，不再维护独立 reducer 镜像状态。
- Agent 只持有一个服务身份，不持有用户 Cookie、Token 或可转授权限。创建 Run 时保存 API 已验证的
  `actor_session_id`；工具请求只携带服务凭据、`runId` 和 `toolCallId`。Luna API 从 PostgreSQL 回读
  用户、会话、会话所有权、项目范围、ToolCall 与审批状态，再进入原有 Handler/Service/RBAC。

## 3. 执行与工具治理

- Agent Loop：补充上下文 → 模型判断 → 工具调用（经 Schema 与审批校验）→ 结果回灌 → 直至
  最终答复或明确终态（批准/补充输入/取消/超时/上限）。
- 工具只有 `requiresApproval` 一个审批字段。普通工具直接执行；高风险工具支持拒绝、批准本次和
  始终允许。豁免只绑定 `user_id + operation_id`，可撤销；拒绝只终结当前 ToolCall，让模型继续。
- queued Run 通过 `FOR UPDATE SKIP LOCKED` 原子更新为 running，不记录实例 owner、租约或心跳。
  服务中断把运行中 Run 标记为 interrupted 并保留事件，不接管、不从调用栈中间恢复。
- fresh schema 不再创建 Grant/lease 列或 lease 函数；升级库仅保留可能承载历史值的旧列，运行时不读写，
  lease 函数和旧 claim 索引由非破坏迁移移除，避免为物理瘦身删除历史数据。
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
- Run 创建时快照当前 Catalog digest；配置刷新先完整构建和校验新目录，再原子切换当前版本，并同时
  保留仍被活动 Run 引用的旧快照。新 Run 使用新 digest，旧 Run 始终使用原 digest；刷新失败继续
  使用上一份有效目录。
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
  加入，然后组装一个滚动摘要、近期完整原文和当前工具结果。`ContextCompiler` 必须把所有 system
  message 一并计入 Token 预算并保持顺序。没有 deferred/catch-up、多级摘要复用或独立 compaction
  状态；压缩失败安全回退且不修改权威完整事件。
- 禁止进入上下文与记忆：Secret、Token、Cookie、Authorization、kubeconfig、Registry 密码、
  Git Access Token、完整终端历史、未脱敏的第三方响应与日志。

### 4.1 模型能力与 Run 预算

- 模型目录的最大上下文、最大单次输出和四类价格由 `internal/aimodel`
  统一校验；Run 创建时连同站点累计 Token/Credits 上限快照，后续管理变更不影响历史 Run。
- 所有归属 Run 的 Provider 调用经过 `BudgetedModelProvider`，且在调用前由 PostgreSQL
  原子 reservation 批准。输出上限是 Agent/站点上限、模型输出、上下文剩余、Run
  剩余 Token/Credits 与个人钱包可负担额度的最小值。
- 过期的 `reserved` 不可直接释放，因为 Provider 可能已收到请求；恢复时按全额
  保守确认。只有明确在外部请求前失败或 Provider 调用前取消才可释放。
- Worker 以 reservation ID 作为计费资源与幂等键，从 `confirmed` reservation
  生成四类 usage/ledger 并与 `settled` 转换同事务完成。AI 费用只属于发起用户个人钱包，
  `project_id` 始终为空。钱包普通 debit 和负 adjustment 都要扣除未结束 hold。
- 全链路锁序为 wallet → Run → reservation（无 Run 的结算/普通扣费为 wallet →
  reservation）；结算可用余额检查排除当前 reservation，但不排除其他活跃 hold。

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

## 7. Agent 运行命令会话

Agent 诊断容器时可创建短期、有状态的非 TTY Shell，多条命令间保留工作目录和环境变量；
浏览器 Web Console 继续使用原有 WebSocket + TTY 协议，两者不复用传输层。

- 命令会话固定绑定用户、登录 Session、Agent Run、项目空间、应用、Release、部署配置和容器，
  任一边界变化即拒绝复用。创建与每条命令都需要对当前参数逐次批准，并重新执行当前账号、
  登录 Session、RBAC、项目开关、工具策略、参数 Schema 与审计检查。
- 审计只记录命令长度和 SHA-256，不保存命令正文；返回输出有大小上限。
- SPDY 流和 Shell 进程只能由创建它的 API 实例持有，不落 PostgreSQL/Redis。`sessionId` 编码
  owner，非 owner 实例返回稳定的 `runtime.command_session.owner_mismatch`，绝不静默创建另一条
  Shell。多副本部署需对命令会话路径配置会话亲和或按 owner 路由。
- 不用数据库持久化会话：进程内 Shell、管道和 SPDY 连接无法从数据库恢复，持久化元数据只会
  制造"记录存在但连接不存在"的伪可用状态。

## 8. 运行时与 Skill 覆盖

Agent 通过 Skill 引导完成平台工作流。Skill 以公开使用文档、控制台主要页面和业务 API 的高频
用户旅程为覆盖口径，一个工作流需同时具备"发现目标 → 收集参数 → 真实操作（缺工具时明确阻塞）→
处理逐次批准/冲突/异步 → 权威回读 → 给出终态结论"才计为已覆盖。

Skill 已覆盖不代表对应写工具已在 Tool Catalog 开放；工具未注册时 Skill 必须阻止模型虚构执行，
明确报告"尚未执行"。

变更工具目录、Schema、Scope 或审批要求时，至少验证完整目录分页、覆盖全部可用 operation 的中英文
检索矩阵、相似读写操作边界、Run 内自动加载与 16 项 LRU、精确详情缓存、目录热更新隔离、未加载
工具不可见、集中排除原因、普通与高危工具、拒绝/单次批准/始终批准/撤销、跨用户/Session/项目空间
隔离，以及一条真实副作用和权威回读。检索词不得进入普通遥测属性、日志字段或 Metric label。

## 9. 参考与事实源

- 交互卡片协议契约：[`12-AI声明式交互卡片Schema.md`](12-AI声明式交互卡片Schema.md)
- 工具注册闭环与调用链约束：`AGENTS.md`
- 可观测插桩：[`14-可观测插桩与验收标准.md`](14-可观测插桩与验收标准.md)
- 已实现的数据模型、接口字段、目录结构：以代码和 OpenAPI 为准
