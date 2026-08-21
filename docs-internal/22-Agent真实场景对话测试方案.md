# Agent 真实场景对话测试方案

本文定义 Luna DevOps Agent 的真实入口、权限、工具、事件、交互卡片、上下文、计费和可观测验收。
目录算法专项契约见 [`23-Agent工具搜索专项测试方案.md`](23-Agent工具搜索专项测试方案.md)，长期
架构边界见 [`11-AI助手与Agent规格.md`](11-AI助手与Agent规格.md)。

测试结论必须来自真实 Web/BFF、Luna API、Agent、PostgreSQL 和对应业务 Provider。单元 fixture
只能用于故障注入，不能代替生产 OpenAPI Catalog、真实权限或最终业务回读。

## 1. 环境与安全

- 使用隔离开发数据库、专用普通用户/管理员、独立项目空间和可回收资源；禁止连接生产环境。
- 所有副作用资源使用测试前缀并记录真实 ID；结束后通过正常业务 API 清理，历史 AI 事件不删除。
- Provider、runtime 与 Catalog 由 Luna API 内部配置端点统一下发；Agent 缺少完整配置必须启动失败。
- Web 只经 BFF HMAC 进入 Agent；Agent 只持有固定回调服务身份，不持有用户 Token 或 Run Grant。
- 内容观测默认关闭。需要检查 Prompt/参数时只在临时隔离环境开启，结束后恢复关闭。

## 2. 核心场景矩阵

| ID | 场景 | 关键断言 |
| --- | --- | --- |
| `CAT-01` | 空 query 浏览目录 | 稳定分页遍历全目录；摘要不含 Schema |
| `CAT-02` | 中英文目标检索 | ProjectVolume 等工具可由 operationId、中文、英文发现 |
| `CAT-03` | 精确详情加载 | 仅选中 operationId 的完整 Schema 进入下一模型步骤 |
| `AUTH-01` | 跨用户访问 | 用户 A 无法读取、批准、执行或订阅用户 B 的 Run/ToolCall |
| `AUTH-02` | Session 失效 | 执行前重验失败，原业务路由不产生副作用 |
| `AUTH-03` | 项目空间错绑 | 会话/Run/业务参数不一致时 fail closed |
| `APP-01` | 普通工具 | 无审批直接执行，但仍经过原 Handler/Service RBAC |
| `APP-02` | 拒绝高危调用 | 当前 ToolCall rejected；Run 可继续回答或选择其他工具 |
| `APP-03` | 单次批准 | 仅当前 ToolCall 执行，下一次仍需审批 |
| `APP-04` | 始终批准 | 创建账号级 user+operation 豁免；同用户同 operation 可直接执行 |
| `APP-05` | 撤销豁免 | 撤销后下一次同 operation 再次 waiting approval |
| `RUN-01` | 用户取消 | Run canceled，已写事件与工具结果保留 |
| `RUN-02` | 进程中断 | running Run 进入 interrupted/failed，不被其他实例 takeover |
| `RUN-03` | 循环上限 | 单 Run 同 operation+规范化参数、总 ToolCall、模型步骤各自有界 |
| `EVT-01` | SSE 断线恢复 | sequence 无缺口；cursor 恢复与权威快照一致 |
| `EVT-02` | 标题与初始消息 | 用户消息、自动/手动标题和手动锁可由不可变事件重放 |
| `CARD-01` | 三类新卡片 | present/input/choice 均编译为 InteractionCardGroup v1 |
| `CARD-02` | 历史卡片 | 旧 payload 原样渲染，不回写新格式 |
| `CARD-03` | 非法卡片 | 最多一次模型修复；仍失败则输出文本 fallback |
| `CTX-01` | 长会话压缩 | 近期原文+原始工具交互+单份滚动摘要；历史 Timeline 不变 |
| `CTX-02` | 新 Run 重新查询 | 旧 Run 空结果不阻止新请求；无跨 Run 指纹或 sticky 工具 |
| `BILL-01` | 模型计费 | assistant/summary/title 预留、确认、释放正确；无 next-step 调用 |
| `BILL-02` | 归属 | AI usage/ledger 的 project_id 始终为空，只扣个人钱包 |
| `OBS-01` | 成功链 | Web/BFF→Agent→模型/工具→Luna API→DB 在同一父子 Trace |
| `OBS-02` | 失败链 | 失败 Span 状态、稳定日志与敏感字段缺失断言通过 |

## 3. ProjectVolume 真实闭环

ProjectVolume 是本轮强制业务基准，按以下顺序串行执行：

1. 从 Web 真实入口新建会话并发送“列出这个项目空间的数据卷”。
2. Agent 调用空 query 或目标 query 的 `search_tools`，返回轻量摘要。
3. Agent 用精确 operationId 调用 `get_tool_details`，下一模型步骤出现真实 list/get 工具 Schema。
4. 执行 list/get 并从原 ProjectVolume Handler 返回当前事实。
5. 发送“创建一个可回收的测试数据卷”，检查 create 是否按 Catalog 的 `requiresApproval` 进入正确
   路径；批准后通过真实业务路由产生副作用。
6. 用独立读取 operation 回读 ID、名称、状态、存储类型和项目归属。
7. 执行 preview/update 或 transfer 相关可用能力，确认目录、参数和最终业务行为一致。
8. 删除测试数据卷，验证 reject、approve 与 approve_always 中至少两条高危路径；随后撤销豁免。
9. 最终 list/get 回读资源已删除或进入权威删除终态。

特殊文件上传、导入、重试或被集中 deny 的 operation 应返回 missing，不能由人工 fixture 伪装可见。

## 4. 权限与身份验收

### 4.1 用户与 Session

- 创建两个用户和两个 Session，各自创建会话、Run 与 ToolCall。
- 互换 conversationId、runId、toolCallId、SSE cursor 和审批请求，全部返回 403/404 稳定错误。
- 让用户 A Session 失效，再由已有 Run 执行普通读取；Luna API 必须在业务路由前拒绝。
- Agent 发送伪造用户、项目、operation 或自定义 Header 不得改变数据库权威身份。

### 4.2 项目空间与 RBAC

- 同一用户在项目 A 为 maintainer、项目 B 为 viewer；分别执行读取和写入。
- 页面上下文中的 projectId 只作为模型提示，不能覆盖会话和业务资源归属。
- 业务 Handler/Service 仍是最终权限权威；Agent 目录和审批不能把 viewer 提升为写权限。

### 4.3 敏感输入

- 模型参数中包含 Token、密码或敏感路径时，调用在 Agent 侧 fail closed，不请求 Luna API。
- 通过 `request_input` 安全表单提交同一值后，值只在受控 direct input 中进入工具参数密文。
- 普通日志、Span、Metric、SSE、Timeline 响应和 Web 详情都不出现明文。

## 5. 审批与豁免

每个审批事件必须包含 toolCallId、operationId、脱敏参数摘要、decision、操作者和时间；不再把
argumentsHash/expectedVersion 作为客户端协议。

- `reject`：ToolCall 为 rejected，写 `tool.rejected`/approval 事件，Run 重新排队或直接形成诚实回复。
- `approve`：持久化当前 ToolCall decision，Luna API 执行前复核，仅使用一次。
- `approve_always`：先批准当前 ToolCall，再写入 userId+operationId 豁免。
- GET exemptions 只返回当前账号记录；DELETE 只能撤销当前账号 operationId，成功返回 204。
- 并发重复批准与撤销必须幂等；不同用户或不同 operation 不共享豁免。

## 6. 事件与中断

权威事实是单 Run 单调 sequence 的不可变事件。验收至少写入并重放：

- 初始用户消息/input received；
- reasoning/assistant message 的创建、增量和完成；
- tool proposed、waiting approval、approval decision、tool result；
- waiting input、用户提交和卡片 payload；
- 自动标题、手动标题与手动锁；
- canceled、failed、interrupted、completed 终态和稳定错误码。

制造 SSE sequence gap 后，Web 必须标记 desync、读取同一快照与 cursor，再继续订阅。关闭 Agent
进程时，已领取 Run 不可保持 running，也不可由另一实例从中间调用栈接管；重启后用户可基于保留
证据发起新的 Run。

## 7. 卡片与上下文

### 7.1 卡片

- `present_card` 只呈现可信结果，不等待用户输入。
- `request_input` 收集结构化字段，Secret 不预填、不回显。
- `request_choice` 只用于少量可信候选；不是完成回复后的独立预测阶段。
- 旧 `create_interaction_cards` 和旧窄业务卡片只作为历史 parser/renderer 输入。
- 模型第一次返回非法 payload 时允许一次定向修复；第二次失败写稳定事件并展示安全文本。

### 7.2 上下文

创建超过压缩阈值的会话，混合用户消息、模型回复、工具调用、拒绝、失败和卡片。压缩前后断言：

- 完整 Timeline 与不可变事件逐字不变；
- 近期原始消息和工具交互成对保留；
- 只有一份滚动摘要，覆盖线单调前进；
- 无 deferred/catch-up 包、多层摘要或 next-step 模型调用；
- 摘要失败退回近期原文并记录失败 Span，不伪造已覆盖历史。

## 8. 计费与可观测

计费不因架构瘦身迁出 Agent：模型调用前仍由 PostgreSQL 原子预留个人钱包预算，完成后确认实际
usage，异常或取消释放；Go billing worker 继续负责最终结算。验证 assistant、summary、title 三类，
不存在 next_steps 预留。所有 `ai.*` meter 的 usage/ledger `project_id` 为空。

临时 OTel 环境至少抽样：

1. 成功：搜索→详情→普通读取→完成；
2. 失败：失效 Session 或跨项目执行被拒绝；
3. 跨服务：高危批准→真实业务写入→PostgreSQL→权威回读。

检查父子 Trace、失败状态、结构化日志关联字段和低基数 Metric。query、资源名、用户/项目/request/
trace ID 不能成为 Metric label；Authorization、Cookie、Session、API Key、Prompt 和敏感参数不能
进入普通遥测。

## 9. 完成门禁

- Go 全量 test，包含 OpenAPI、身份中间件、审批/豁免和 fresh/upgrade migration；
- Agent lint、typecheck、test、build；
- Web AI 助手测试、lint、build、singleton 检查；
- 中英文 docs build 与链接检查；
- fresh 数据库和带历史事件/卡片/审批的升级数据库内容哈希不变；
- 真实 ProjectVolume 读取与可回收写入/回读；
- 浏览器中目录发现、审批三路径、豁免撤销、SSE 恢复、历史卡片和 interrupted 提示；
- 临时 OTel 成功、失败、跨服务链抽样；
- `git diff --check`，且不提交本地环境、构建产物或临时日志。

全部通过后才在 `TODO.md` 标记验收完成。
