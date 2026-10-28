# AI Agent 运行时整改方案

## 1. 文档目的

本文用于整改 `luna-agent` 的 Agent Loop、工具执行、审批恢复、流式输出和终态判定。
目标是向成熟 Agent Runtime 的通用范式靠拢，减少自研控制流和启发式补丁，同时保留 Luna
DevOps 必须掌握的权限、审批、审计、幂等和计费边界。

本文是内部实施方案，不是新的 API 契约。实现后的字段、迁移、OpenAPI 和测试以代码与生成物为准；
各阶段的行为变化、偏差和验证记录见 [14-Agent运行时行为矩阵.md](14-Agent运行时行为矩阵.md)。

## 2. 整改结论

整改前的 `RunExecutor` 同时承担调度、模型循环、工具执行、卡片、审批、恢复、终态持久化和遥测，
控制流由多个布尔变量和进程内存拼接。Phase 0–3 已经把它收敛为结构化决策、显式状态图、持久化
checkpoint 和独立的终态提交模块。后续整改遵循以下结论：

1. **显式状态替代隐式布尔组合**：控制流位置由 `ai.run_checkpoints.phase` 表达，Run 生命周期由
   `ai.runs.status` 表达，两者都不描述业务是否成功。
2. **Run 生命周期与业务结果分离**：`completed` 只表示 Agent 循环正常结束，不表示用户目标已达成。
   业务事实由工具结果和权威 API 响应表达，Runtime 不判定“用户要求的写操作是否全部完成”。
3. **沿用通用 final 语义**：没有 tool call 的合法文本就是最终答复。模型“说了计划却没行动”是
   模型行为质量问题，由 Prompt 和评测集治理，Runtime 不猜测文本是计划还是答复。
4. **结束原因只作辅助信号**：可解析的工具调用一律执行；只有“宣告工具调用却没有调用”、截断和
   内容过滤会改变控制流。
5. **checkpoint 保持当前粒度**：模型步、工具批次、等待和终态四类边界已持久化；不再扩展到模型调用
   内部或外部 HTTP 调用级别。
6. **不让 Run 等待长耗时异步任务**：异步提交以“已受理 + 资源引用”回到模型，终态由资源页面和通知跟踪。
7. **统一 Run Budget**：所有预算由同一结构约束，耗尽语义和错误码一致。
8. Langfuse 或 OpenTelemetry 只负责观测，不参与控制流；是否引入具体框架在 Phase 4 后再评估。

## 3. 参考范式与边界

### 3.1 参考范式

```text
模型调用
  ├─ 无 tool call 的文本  → final，结束
  ├─ tool calls           → 执行工具，回灌结果，再次调用模型
  └─ interrupt            → 持久化状态，等待用户/审批后恢复
```

OpenAI Agents SDK 的 Runner 以“没有 tool call 的输出”作为 final output，并用 `max_turns` 限制模型
步数；工具审批通过可恢复的 RunState 继续。LangGraph 使用状态图、条件边和 checkpointer 表达工具
循环、`END` 与人工介入。

参考资料：

- [OpenAI Agents: Running agents](https://openai.github.io/openai-agents-python/running_agents/)
- [OpenAI Agents: Results and resumable state](https://openai.github.io/openai-agents-python/results/)
- [LangGraph: Persistence and durable execution](https://langchain-ai.github.io/langgraph/concepts/durable_execution/)
- [LangGraph: Tool routing](https://langchain-ai.github.io/langgraphjs/reference/functions/langgraph.prebuilt.toolsCondition.html)
- [Langfuse: Observation types](https://langfuse.com/docs/observability/features/observation-types)

### 3.2 Luna 的职责划分

| 层 | 负责内容 | 不负责内容 |
| --- | --- | --- |
| Agent Runtime | 模型循环、状态图、tool call 路由、checkpoint、interrupt/resume、Run Budget | Luna 权限和业务结果判定 |
| Luna Tool Adapter | OpenAPI operation、参数 Schema、服务端请求、工具结果映射 | 自己推断用户权限、跳过 API 鉴权 |
| Luna API/Service | RBAC、审批、幂等、审计、业务状态和权威响应 | 解析模型文字 |
| Timeline/Stream | 事件和过程输出的投影、SSE、断线回读 | 决定 Run 是否完成 |
| Langfuse/OTel | Agent、generation、tool、interrupt 的 trace | 驱动 loop 或恢复执行 |

## 4. 问题清单与状态

| 级别 | 问题 | 状态 |
| --- | --- | --- |
| P0 | 交互工具 `request_input` / `request_choice` 创建卡片后 Run 直接 `completed`，不等待用户提交 | **未解决**，Phase 4 |
| P0 | 结束原因与工具调用不一致时拒绝执行可解析的工具调用；`tool_use` 未识别 | 已解决（Phase 1 修正） |
| P1 | `RunExecutor` 职责堆积 | 已解决（Phase 3） |
| P1 | 恢复状态分散在 timeline 与进程内存 | 已解决（Phase 2 checkpoint） |
| P1 | 过程文字与最终答复都落为 `assistant_message` | 已解决（Phase 3 `progress`） |
| P1 | 循环保护计数仅在进程内存 | 已解决（Phase 2：计数随 checkpoint 导出与恢复） |
| P2 | 模型步数、工具调用数、重复调用、卡片修复、协议修复、参数修复、空回答和超时各自维护；空回答会耗尽全部模型步数 | **未解决**，Phase 4 |
| P2 | 工具结果没有统一 outcome，异步提交与同步成功不可区分 | **未解决**，Phase 4 |
| P2 | 缺少 API → Agent → Tool → 外部平台的真实链路验收 | **未解决**，Phase 4 |

## 5. 目标状态模型

### 5.1 Run 状态

两层状态各司其职：

- `ai.runs.status`：生命周期，`queued → running → completed | failed | canceled | interrupted`，
  可恢复中断为 `waiting_approval`、`waiting_input`；
- `ai.run_checkpoints.phase`：控制流位置，`model`、`tool_execution`、`awaiting_input`、
  `awaiting_approval`、`finalizing`，由 `graph.ts` 校验合法转换。

`completed` 只表示模型给出合法 final 且没有挂起的工具调用或中断，不表示业务成功。

进程在 `running` 期间退出时，有 checkpoint 的 Run 由启动对账重新排队续跑（恢复次数上限 3）；
没有 checkpoint 或超过上限的 Run 置为 `interrupted`。该机制以单副本部署为前提。

### 5.2 模型决策（已落地）

```ts
type ModelDecision =
  | { kind: "tool_calls"; calls: ModelToolCall[]; finishReason?: string }
  | { kind: "final"; text: string; finishReason?: string }
  | { kind: "empty"; finishReason?: string }
  | { kind: "truncated" }
  | { kind: "filtered" }
  | { kind: "protocol_error"; code: "missing_tool_calls" }
```

判定规则：

1. 有可解析的 tool call：一律为 `tool_calls`，结束原因不一致只体现在遥测中。
2. 无 tool call，结束原因宣告工具调用（`tool_calls`、`function_call`、`tool_use`）：`protocol_error`。
3. 无 tool call，结束原因为 `length` 或 `max_tokens`：`truncated`；为 `content_filter`：`filtered`。
4. 无 tool call，非空文本：`final`；空文本：`empty`。未知结束原因按 `stop` 处理。

### 5.3 工具结果（Phase 4）

```ts
type ToolOutcome =
  | { kind: "succeeded"; result: unknown }
  | { kind: "accepted"; result: unknown }
  | { kind: "failed"; errorCode: string; retryable: boolean }
  | { kind: "awaiting_approval"; toolCallId: string }
  | { kind: "awaiting_input"; fields: string[] }
  | { kind: "canceled" }
```

- `succeeded`：同步 API 返回成功，响应即服务端权威结果，不需要额外回读；
- `accepted`：异步 API 返回 202，结果中包含资源引用和当前状态（如构建 `queued`）。模型可以按需调用
  只读状态工具，但 Runtime 不等待异步操作到达终态；
- `awaiting_*`：进入对应中断，不回模型。

### 5.4 完成语义

Run 进入 `completed` 需要且只需要：

1. 模型决策为 `final`；
2. 当前步骤没有未处理的 tool call、审批或输入。

业务结果不由 Run 状态表达。模型如实说明“权限不足无法删除”或“构建已提交，状态 queued”都是合法的
`completed`。最终答复不得把 `accepted` 描述为已完成，这一约束写入系统 Prompt 并纳入评测，
不由 Runtime 解析文本。

## 6. 模块职责

| 模块 | 职责 | 状态 |
| --- | --- | --- |
| `executor/index.ts` | Run claim、配额、并发、超时、取消、启动对账和组件装配 | 已落地 |
| `executor/run-loop.ts` | 单个 Run 的模型步骤、checkpoint 推进、恢复动作和终态持久化错误分支 | 已落地 |
| `executor/decision.ts` | Provider 结果 → `ModelDecision`，纯函数 | 已落地 |
| `executor/graph.ts` | 决策、工具批次和恢复计划 → 显式动作，并按 phase 校验合法转换 | 已落地 |
| `executor/checkpoint.ts` | 持久化 phase 与续跑状态，`resumePlan` 映射续跑动作 | 已落地 |
| `executor/tool-step.ts` | 工具批次、内部工具、卡片、导航和恢复批次核对 | 已落地；Phase 4 按 §9 拆 adapter |
| `executor/finalize.ts` | 唯一的 `completed` 提交入口，终态竞争返回实际终态；标题生成作为后处理 | 已落地 |
| `executor/streaming.ts` | 草稿流式输出；`final` 提交为 `assistant_message`，其余提交为 `progress` | 已落地 |
| Run Budget | 统一预算结构与耗尽语义，见 §6.1 | Phase 4 |

### 6.1 Run Budget

```ts
type RunBudget = {
  maxModelSteps: number
  maxToolCalls: number
  sameCallLimit: number
  maxRepairs: number      // 协议、参数、卡片 Schema 和空回答修复共用
  runTimeoutMs: number
}
```

- 预算取值继续来自 claim 时持久化的 `runtimeSettings` 快照，不重复写入 checkpoint；
- 已消耗量随 checkpoint 恢复（现有协议/参数修复次数与循环保护计数并入同一结构）；
- 由 `graph.ts` 在每次 `call_model`、`repair`、`retry` 和工具批次前检查；
- 同参数重复调用是对单次调用的拒绝，继续以 `ai.tool_repeated_in_run` 作为工具结果回灌，让模型改变做法，
  不终止 Run；
- Run 级预算（`model_steps` / `tool_calls` / `repairs` / `timeout`）耗尽统一以 `ai.run_budget_exceeded`
  终止 Run，遥测记录低基数属性 `luna.run.budget_kind`，前端按稳定错误码映射 i18n；
- 空回答计入 `repairs`，不再消耗整份模型步数；`ai.final_response_missing`、`ai.limit_exceeded`、
  `ai.run_timeout` 与 `ai.run_tool_call_budget_exceeded` 随之删除，代码、测试、i18n 和文档同步清理。

## 7. 数据与幂等

### 7.1 checkpoint（已落地）

`ai.run_checkpoints` 以 `run_id` 为主键，`revision` 乐观锁单调推进，同一 revision 重放幂等、旧
revision 被拒绝；`recoveries` 记录启动恢复次数。字段取舍与偏差见 14 号文档 §9.3：
能由 `ai.runs`、`ai.tool_calls` 或执行快照推导的数据不重复保存。

已知边界：工具事实、Timeline 投影与 checkpoint 仍是三次写入，依靠“事实先行 + 恢复核对”保证一致；
核对失败时失败关闭，不猜测推进。

### 7.2 工具调用幂等

- `ai.tool_calls.id` 是工具调用的唯一标识，下游写操作以它作为幂等键；
- 审批恢复执行已持久化的调用记录，不重新生成调用；
- 恢复时已落地的调用折叠结果继续，`running` 的调用以 `ai.tool_outcome_unknown` 失败关闭，
  不自动重放可能已送达外部平台的请求。

## 8. 运行流程

### 8.1 普通问答

```text
claim → model → final → finalizing → completed
```

### 8.2 读取型任务

```text
model → tool_calls → tool_execution → 工具事实落库 → model → final → completed
```

### 8.3 写操作和审批

```text
model → tool_calls → policy/approval → awaiting_approval + waiting_approval
→ 用户批准 → queued → claim → 按 checkpoint 核对 → 执行工具（幂等键 = tool call id）
→ model → final → completed
```

### 8.4 用户输入（交互卡片为 Phase 4）

```text
model → request_input / request_choice → awaiting_input + waiting_input
→ 用户提交 → queued → claim → 输入作为工具结果回灌 → model
```

工具缺必填参数已走该路径；交互卡片仍直接 `completed`，Phase 4 统一到此流程。

### 8.5 异步操作

```text
model → submit operation → accepted（资源引用 + 当前状态）→ model
→ 可选：调用只读状态工具 → final（如实描述当前状态）→ completed
```

### 8.6 模型协议错误

```text
结束原因宣告工具调用但无 tool call → protocol_error → 一次有界修复 → 仍无 tool call：ai.model_protocol_violation
length / max_tokens → ai.model_output_truncated
content_filter      → ai.model_output_filtered
```

修复请求使用 system 消息，不伪装成用户消息；被拒步骤的文字只以 `progress` 保留。

## 9. 工具分类和后续动作

| 工具类别 | 示例 | 默认后续 |
| --- | --- | --- |
| 发现 | `search_tools`, `get_tool_details` | 回模型 |
| 只读事实 | 项目、部署、Pod、日志查询 | 回模型 |
| 同步写操作 | 保存配置、重启、删除 | 回模型；API 响应即权威结果 |
| 异步提交 | 构建、部署、证书申请 | 回模型；结果为 `accepted` |
| 交互 | `request_input`, `request_choice` | `awaiting_input` 中断 |
| 呈现 | `present_card`, `navigate_to_route` | 回模型 |

所有类别都只决定下一动作；Run 结束只由 `final` 决策触发（§5.4）。直接工具路径
（用户在卡片上直接执行工具）仍以工具事实结束 Run，属于用户显式操作，不经过模型决策。

## 10. 分阶段实施计划

### Phase 0–3（已完成）

| 阶段 | 内容 | 记录 |
| --- | --- | --- |
| Phase 0 | 行为矩阵与特征化测试基线 | 14 号文档 §3–§7 |
| Phase 1 | `ModelDecision`；结束原因只作辅助信号；截断与过滤独立错误码 | 14 号文档 §8 |
| Phase 2 | `ai.run_checkpoints`、四类退出边界续跑、恢复次数上限、批次核对与失败关闭 | 14 号文档 §9 |
| Phase 3 | 拆分 run-loop / graph / tool-step / finalize；`progress` 投影；标题后处理 | 14 号文档 §10 |

冻结规则继续有效：不新增确认次数、prompt nudge、候选回答或兜底条件。

### Phase 4：交互中断、统一预算与工具 outcome

工作：

- `request_input` / `request_choice` 改为 `awaiting_input` 中断，用户提交后恢复，修复矩阵行 9；
- 按 §6.1 引入 Run Budget，并删除旧的分散错误码；
- 按 §9 的类别拆分 `tool-step.ts` 中的 adapter，输出 `ToolOutcome`；异步提交统一返回 `accepted`；
- 系统 Prompt 增加“`accepted` 不得描述为已完成”的约束，并纳入评测集；
- 取消和失败路径保留已提交副作用的真实状态，不伪造回滚成功。

验收：交互卡片在跨进程（新 Executor + 真实 PostgreSQL）下等待并恢复；连续空回答受 `repairs`
约束；至少一条 API → Agent → Tool → 外部平台的真实链路（如提交构建并查询状态）覆盖成功、失败、
取消、审批和撤权。

### Phase 5：评估是否引入具体框架

只有 Phase 4 完成后才做框架选型。若自研状态图和 checkpoint 代码仍然重复 LangGraph 等框架的通用能力，
只对 `model → tool → interrupt → resume` 最小路径做 PoC；Luna 权限、审批、审计和 Tool Adapter 保持独立。
禁止一次性把整个 `luna-agent` 重写为框架示例代码。

## 11. 测试和发布门禁

### 11.1 测试层次

1. `ModelDecision`、`graph` 转换和 Run Budget 纯函数单测。
2. checkpoint 乐观锁、恢复核对与恢复上限的 PostgreSQL 集成测试。
3. Tool Adapter 的参数、权限、审批、幂等和错误码契约测试。
4. SSE/Timeline 只验证事件投影，包括 `progress` 与 `assistant_message` 的区分。
5. Agent Run 集成测试覆盖成功、失败、取消、超时、审批、输入和预算耗尽。
6. 至少一条 API → Agent → Tool → 外部平台的 E2E。

### 11.2 Phase 4 必测场景

```text
request_input / request_choice 等待输入后跨进程恢复
连续空回答受 repairs 预算约束
各类 Run 级预算耗尽返回 ai.run_budget_exceeded 与对应 budget_kind
同参数重复调用仍作为工具结果回灌，不终止 Run
异步提交返回 accepted，最终答复不声称已完成（评测集）
同步写操作成功后不额外回读
真实链路：提交构建并查询状态，覆盖成功、失败、取消、审批和撤权
Secret 不进入消息、checkpoint、日志和事件
```

Phase 0–3 的必测场景已在 14 号文档登记回归位置，后续改动不得让其失效。

### 11.3 发布门禁

- `go test ./...`
- `pnpm --dir luna-agent test`（含真实 PostgreSQL）
- `pnpm --dir luna-agent lint`
- `pnpm --dir luna-agent typecheck`
- 涉及投影或错误码时执行 Web test/lint/build 与 i18n 检查
- OpenAPI 与 Tool Catalog 契约检查
- 相关 PostgreSQL migration up/down/reapply
- Agent 关键 E2E

## 12. 回滚和故障语义

1. 迁移可 down；回滚代码前先执行 down。
2. 进程在 `running` 期间退出：有 checkpoint 时续跑，超过恢复上限或无 checkpoint 时置为 `interrupted`。
3. 结果未知的工具调用失败关闭，不重放。
4. checkpoint 或恢复核对写入失败时失败关闭，不在内存中继续执行写操作。
5. 终态持久化耗尽时尽力置为 `interrupted`，不伪造 `completed` 或 `failed`。
6. 直接替换旧执行路径，不保留 feature flag 或新旧双路径；新旧代码产生的 Timeline 事件使用同一稳定语义。

## 13. 完成标准

- Runtime 不解析模型文字判断意图，final 只由结构化决策触发；
- 可解析的工具调用不因结束原因取值被拒绝；
- 审批和输入（含交互卡片）从 checkpoint 恢复，不从 timeline 反推；
- 预算统一，耗尽使用统一错误码；
- 过程消息与最终答复在 Timeline 中可区分；
- 取消、超时、崩溃和终态持久化失败不会伪造成功；
- Langfuse/OTel 只观测，不改变控制流；
- 至少一条真实平台副作用链路完成成功、失败、取消、审批和撤权验收；
- 相关测试、迁移、OpenAPI、Agent catalog 和内部文档同步通过。

## 14. 明确不做

- **不在 Runtime 判定业务完成**：Runtime 无法知道用户“要求执行”了哪些写操作，强行判定只会重新依赖
  模型文字；也不为每个写 operation 维护“写 → 验证读”的映射。
- **不让 Run 等待长耗时异步任务**：构建、部署、证书终态由资源页面和通知跟踪，不引入 `waiting_external`。
- **不扩展 checkpoint 粒度**：不做模型调用内部或外部 HTTP 调用级 checkpoint；多副本部署前不放宽
  “单副本启动恢复”的前提。
- 不为区分“计划文字”和“最终答复”增加确认次数、隐藏 nudge 或候选回答。若评测显示该行为频繁，
  再评估任务模式下的强制工具调用。
- 不保留 feature flag、新旧双路径或多个互相兜底的 final 判定路径。
- 不为了“更像框架”增加多 Agent 自主讨论、投票或规划层。
- 不把 Langfuse 当 Agent 执行框架。
- 不把平台权限、审批、审计和业务状态交给模型或通用框架。
