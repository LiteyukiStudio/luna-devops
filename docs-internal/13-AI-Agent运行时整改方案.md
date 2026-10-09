# AI Agent 运行时整改方案

## 1. 文档目的

本文用于整改 `luna-agent` 当前的 Agent Loop、工具执行、审批恢复、流式输出和终态判定。
目标是向成熟 Agent Runtime 的通用范式靠拢，减少自研控制流和启发式补丁，同时保留 Luna
DevOps 必须掌握的权限、审批、审计、幂等、业务终态和计费边界。

本文是内部实施方案，不是新的 API 契约。实现后的字段、迁移、OpenAPI 和测试以代码与生成物为准。

## 2. 整改结论

当前实现已经形成一个自研 Agent Runtime，主要问题不是单个条件判断，而是模型协议、业务状态、
恢复状态和流式状态没有使用同一个显式模型。`RunExecutor` 同时承担调度、模型循环、工具执行、
卡片、审批、恢复、终态持久化和遥测，后续每增加一个异常路径都会扩大状态组合。

Pull Request #20 对“工具执行后模型只输出计划文字”的问题提供了有限止血，但它通过候选回答、
继续指令和确认次数补偿协议缺口，不能作为最终架构。首步计划文字仍可能被当成最终答复，且
候选文字、工具交互和恢复状态仍然分散在多处。

整改采用以下方向：

1. 以显式状态图替代 `RunExecutor` 内部的隐式布尔状态组合。
2. 以结构化模型决策替代“普通文本到底是计划还是最终答复”的猜测。
3. 以持久化 checkpoint 替代内存中的 continuation、确认次数和关键 loop 状态。
4. 以 `tool call → tool result → verification` 作为任务型 Agent 的基本执行单元。
5. 以中断/恢复作为审批、用户输入和异步外部任务的统一机制。
6. Langfuse 或 OpenTelemetry 只负责观测，不参与控制流；LangGraph 风格的图编排作为参考，
   是否引入具体框架在最小状态机完成后再决定。

## 3. 参考范式与边界

### 3.1 参考范式

成熟 Agent Runtime 通常将一次 Agent Run 组织为：

```text
模型调用
  ├─ final output       → 结束
  ├─ tool calls         → 执行工具，回灌结果，再次调用模型
  ├─ handoff            → 切换 Agent 或工作流节点
  └─ interrupt          → 持久化状态，等待用户/审批后恢复
```

OpenAI Agents SDK 将上述循环封装在 Runner 中，并使用 `max_turns` 限制模型步数；工具审批和
中断通过可恢复的 RunState 继续。LangGraph 使用状态图、条件边和 checkpointer 表达工具循环、
`END`、人工介入和故障恢复。

参考资料：

- [OpenAI Agents: Running agents](https://openai.github.io/openai-agents-python/running_agents/)
- [OpenAI Agents: Results and resumable state](https://openai.github.io/openai-agents-python/results/)
- [LangGraph: Persistence and durable execution](https://langchain-ai.github.io/langgraph/concepts/durable_execution/)
- [LangGraph: Tool routing](https://langchain-ai.github.io/langgraphjs/reference/functions/langgraph.prebuilt.toolsCondition.html)
- [Langfuse: Observation types](https://langfuse.com/docs/observability/features/observation-types)

### 3.2 Luna 的职责划分

| 层 | 负责内容 | 不负责内容 |
| --- | --- | --- |
| Agent Runtime | 模型循环、状态图、tool call 路由、checkpoint、interrupt/resume、max turns | Luna 权限和业务终态 |
| Luna Tool Adapter | OpenAPI operation、参数 Schema、服务端请求、工具结果映射 | 自己推断用户权限、跳过 API 鉴权 |
| Luna API/Service | RBAC、审批、幂等、审计、业务状态和权威回读 | 解析模型计划文字 |
| Timeline/Stream | 事件和过程输出的投影、SSE、断线回读 | 决定 Run 是否完成 |
| Langfuse/OTel | Agent、generation、tool、interrupt、verification 的 trace | 驱动 loop 或恢复执行 |

## 4. 当前问题清单

### P0：终态协议不明确

当前 `result.answer` 同时表示最终回答、执行计划和失败解释。`executionStarted`、
`pendingFinalAnswer`、`finalConfirmations` 和隐藏 continuation nudge 只能提高命中率，无法
证明模型真正完成了用户目标。

整改要求：普通文本不得直接证明业务完成。任务型 Run 必须使用结构化决策和服务端完成证据。

### P1：`RunExecutor` 职责堆积

`luna-agent/src/executor/index.ts` 同时实现调度、模型调用、工具发现、工具执行、卡片、审批、
取消、错误收敛、流式提交、标题生成和遥测。应拆成可单测的状态转换、模型决策、工具批处理、
checkpoint 和终态提交模块。

### P1：恢复状态分散且不可完整恢复

`continuationMessages`、`pendingFinalAnswer`、确认次数、空回答次数和 `InMemoryLoopGuard` 主要
存在于内存。进程在模型步骤之间、工具完成之后或流式提交期间退出时，重启只能从部分
`toolInteractions` 推断状态，无法恢复准确的当前节点和剩余预算。

### P1：过程消息和最终消息混用

候选计划文字通过 `commitStep()` 持久化为 `assistant_message`，后续最终回答又产生另一条消息。
历史会话和前端投影无法可靠区分过程文本与最终答案。

### P1：工具循环保护不是持久事实

重复调用计数和非重试集合在进程内存中。进程重启、审批恢复或未来多副本执行时，Run 级调用保护
可能丢失。工具执行还需要与下游 API 的幂等键和数据库状态共同约束。

### P1：工具发现、平台执行和 UI 卡片混在一个循环

`search_tools`、`get_tool_details`、`present_card`、平台读写工具和导航工具虽然都使用 tool call
接口，但它们的控制流含义不同：有的只发现能力，有的产生副作用，有的只呈现事实。当前逻辑用多个
布尔变量补偿这种差异，容易出现错误终态和额外模型调用。

### P2：多个预算没有统一语义

模型步数、工具调用数、同参数重复次数、卡片修复次数、空回答重试次数、继续确认次数和 Run 超时
分别维护，耗尽后错误码和用户可观察行为不一致。应统一为 Run Budget，并记录耗尽原因。

### P2：测试覆盖脚本路径多，恢复和并发证据不足

现有模型脚本测试覆盖了多种输出顺序，但还需要覆盖 checkpoint、进程恢复、审批恢复、重复执行、
并发 claim、异步权威回读、取消竞态和过程消息投影。

## 5. 目标状态模型

### 5.1 Run 阶段

将当前多个布尔变量收敛为单一阶段。阶段只描述控制流位置，不描述业务成功。

```ts
type RunPhase =
  | "model"
  | "tool_execution"
  | "verification"
  | "awaiting_input"
  | "awaiting_approval"
  | "waiting_external"
  | "finalizing"
  | "completed"
  | "failed"
  | "canceled"
  | "interrupted"
```

### 5.2 模型决策

Provider 输出先转换为内部判别联合，Executor 不再直接依据 `answer.trim()` 推断控制流。

```ts
type ModelDecision =
  | { kind: "tool_calls"; calls: ModelToolCall[]; finishReason?: string }
  | { kind: "final"; text: string; finishReason?: string }
  | { kind: "empty"; finishReason?: string }
  | { kind: "protocol_error"; code: "missing_tool_calls" | "invalid_finish_reason" }
```

`needs_input` 和 `awaiting_approval` 是工具执行后的 Run 阶段，不由模型普通文本表示。

### 5.3 工具结果

平台工具结果必须明确区分：

```ts
type ToolOutcome =
  | { kind: "succeeded"; result: unknown }
  | { kind: "failed"; errorCode: string; retryable: boolean }
  | { kind: "awaiting_approval"; toolCallId: string }
  | { kind: "awaiting_input"; fields: string[] }
  | { kind: "running"; externalOperationId?: string }
  | { kind: "canceled" }
```

工具成功不等于业务完成。写操作和异步操作必须进入 `verification`，由权威读取确认最终状态。

### 5.4 完成证据

Run 的 `completed` 需要满足：

1. 模型输出合法 final text；
2. 当前任务没有未处理的 tool call、approval、input 或 external operation；
3. 所有要求执行的写操作都有成功结果；
4. 异步任务已经通过约定的权威读取工具确认终态；
5. 没有未解决的工具错误或协议错误。

普通问答没有业务工具依赖时，可以直接满足第 1、2 条。

## 6. 目标模块拆分

### 6.1 `executor/decision.ts`

职责：把 Provider/ModelRuntime 的一次结果转换为 `ModelDecision`。

要求：

- 检查 `finishReason` 与 tool call 是否一致；
- 不访问数据库、不执行工具、不写 Timeline；
- 用纯函数测试所有组合：文字、空输出、工具调用、非法 finish reason、工具调用带文字。

### 6.2 `executor/graph.ts`

职责：实现显式状态转换。

建议入口：

```ts
type GraphAction =
  | { type: "call_model" }
  | { type: "execute_tools"; toolCallIds: string[] }
  | { type: "wait"; phase: "input" | "approval" | "external" }
  | { type: "finalize" }
  | { type: "fail"; errorCode: string }
```

该模块不直接做 HTTP 请求，只根据 checkpoint 和事件生成下一动作。

### 6.3 `executor/checkpoint.ts`

职责：读取、校验和原子保存 Run checkpoint。

checkpoint 至少包含：

```ts
type RunCheckpoint = {
  runId: string
  revision: number
  phase: RunPhase
  modelStep: number
  toolCallCount: number
  pendingToolCallIds: string[]
  pendingExternalOperationIds: string[]
  selectedOperationIds: string[]
  budget: {
    maxModelSteps: number
    maxToolCalls: number
    maxRepairAttempts: number
  }
  lastDecisionId?: string
  finalOutput?: string
}
```

关键点：checkpoint 是恢复控制流的事实；Timeline 是用户可见投影；工具表是工具执行事实；三者
不再互相推断代替。

### 6.4 `executor/tool-step.ts`

职责：执行一个结构化 tool batch，并返回统一 `ToolOutcome`。

要求：

- 平台权限和审批仍由 Luna API/ToolOrchestrator 最终判断；
- 每个调用携带 `runId`、`toolCallId` 和幂等键；
- 结果写入工具事实后再推进 checkpoint；
- `awaiting_approval`、`awaiting_input` 和 `running` 不进入模型最终回答分支；
- 不把 `present_card` 或 `navigate_to_route` 当成业务完成证据。

### 6.5 `executor/finalize.ts`

职责：检查完成证据，持久化 final output 和 Run 终态。

只有该模块可以将 Run 推进到 `completed`。模型循环、流式模块和工具模块不能直接完成 Run。

### 6.6 `streaming.ts`

流式层只输出事件，不决定业务终态。建议事件类型区分：

```text
model.started
model.text.delta
model.reasoning.delta
tool.started
tool.completed
run.interrupted
run.waiting_input
run.waiting_approval
run.completed
run.failed
```

计划文字如果需要实时展示，使用 `progress_message` 或未提交草稿语义；`assistant_message` 只表示
已经接受为对话输出的消息。

## 7. 数据和迁移方案

### 7.1 新增 checkpoint 存储

新增编号迁移和 Repository 方法，建议使用独立表而不是继续向 `runs` 堆叠临时字段：

```text
ai.run_checkpoints
  run_id              primary key
  revision            bigint not null
  phase               text not null
  model_step          integer not null
  tool_call_count     integer not null
  state               jsonb not null
  created_at          timestamptz not null
  updated_at          timestamptz not null
```

保存使用 `run_id + revision` 乐观锁。重复提交同一 revision 必须幂等，旧 revision 不能覆盖新状态。

### 7.2 工具调用幂等

工具事实表增加或复用以下约束：

- `run_id`；
- `tool_call_id`；
- `operation_id`；
- canonical arguments hash；
- attempt；
- 当前状态；
- 外部 operation ID（如有）。

同一次逻辑调用重试必须复用逻辑幂等键；不能每次因为模型再次生成而无条件创建新的副作用调用。

### 7.3 数据迁移原则

1. 先增加表和读路径，不改变旧 Run 的终态语义。
2. 新 Run 同时写旧事实和 checkpoint，做一段时间双读校验。
3. 确认恢复、审批和取消路径稳定后，切换 Executor 只从 checkpoint 恢复。
4. 清理 `pendingFinalAnswer`、continuation retry 字段和旧兼容读取前，完成历史 Run 处理和回滚方案。

## 8. 运行流程

### 8.1 普通问答

```text
claim Run
→ checkpoint.phase=model
→ call model
→ decision.final
→ finalize final output
→ completed
```

### 8.2 读取型任务

```text
model
→ tool_calls
→ execute read tools
→ persist tool results
→ checkpoint.phase=model
→ call model
→ final
→ completed
```

### 8.3 写操作和审批

```text
model
→ tool_calls
→ policy/approval
→ awaiting_approval
→ persist checkpoint
→ 用户批准
→ resume checkpoint
→ execute tool
→ verification
→ model
→ final
```

### 8.4 异步操作

```text
model
→ submit operation
→ external operation running
→ waiting_external
→ 按契约调用状态工具
→ verification
→ terminal / unavailable / failed
→ final 或 incomplete
```

### 8.5 模型协议错误

```text
model 返回需要工具的计划文字但没有 tool call
→ protocol_error
→ 最多一次有界修复
→ 仍无 tool call：model_protocol_violation / incomplete
```

修复请求不能伪装成用户消息，也不能把第二次普通文字自动当成业务完成证据。

## 9. 工具分类和终态规则

| 工具类别 | 示例 | 是否产生业务完成证据 | 默认后续 |
| --- | --- | --- | --- |
| 发现 | `search_tools`, `get_tool_details` | 否 | 回模型 |
| 只读事实 | 项目、部署、Pod、日志查询 | 可作为事实 | 视目标决定是否验证 |
| 写操作 | 保存配置、发布、重启、删除 | 否 | 权威回读 |
| 异步提交 | 构建、部署、证书申请 | 否 | 状态查询 |
| 交互 | `request_input`, `request_choice` | 否 | 等待用户恢复 |
| 呈现 | `present_card`, `navigate_to_route` | 否 | 回模型或结束呈现 |

只有写操作的成功结果加权威回读，或纯问答的 final output，才可以形成 completed。

## 10. 分阶段实施计划

### Phase 0：基线和冻结（1 个任务）

目标：停止继续增加 loop 特殊分支。

工作：

- 冻结 PR #20 的行为，只修复明确的错误，不再追加确认次数和 prompt nudge；
- 建立当前行为矩阵：普通问答、首步计划、工具后计划、审批、输入、异步、取消、重启；
- 记录当前 Run、ToolCall、Timeline 和 SSE 的事实源；
- 补充失败案例和最小复现，不改生产代码。

验收：每个场景都有当前结果、期望结果、负责模块和回归测试位置。

### Phase 1：抽出决策协议（小重构）

工作：

- 新增 `ModelDecision` 和 `ToolOutcome`；
- 把 finish reason、tool call 解析和空响应处理移到纯函数；
- `RunExecutor` 暂时保留，但只消费结构化决策；
- 删除主循环中直接依据 `answer.trim()` 完成 Run 的分支；
- 将协议错误映射为稳定错误码。

验收：协议单测覆盖完整组合；普通问答仍为一次模型调用；已有工具测试全部通过。

### Phase 2：引入显式 RunPhase 和 checkpoint

工作：

- 新增迁移和 Repository checkpoint 接口；
- 每个模型步骤和工具 batch 原子更新 checkpoint；
- 审批、用户输入、异步等待使用 checkpoint 恢复；
- 将 loop guard 的关键计数迁移到 checkpoint/数据库；
- 保留旧 timeline 作为投影，不让 timeline 反推控制流。

验收：在模型调用前、工具完成后、审批等待后、最终提交前模拟进程退出，重启后状态只执行一次。

### Phase 3：拆分 Executor

工作：

- 拆出 `decision`、`graph`、`checkpoint`、`tool-step`、`finalize`；
- `RunExecutor` 只负责 claim、生命周期和组件装配；
- 统一取消、超时、终态和 TerminalPersistence 错误处理；
- 把标题生成移出 Agent 核心终态路径，作为完成后的 best-effort 工作。

验收：主 Executor 不再包含工具目录、卡片 Schema 和具体平台工具分支；模块可以独立测试。

### Phase 4：统一工具适配和业务验证

工作：

- 发现、只读、写操作、异步提交、交互和呈现工具分别定义 adapter；
- 写操作统一返回 `verificationRequired` 或外部 operation 状态；
- 为发布、构建、部署和证书类异步任务建立权威回读节点；
- 取消和失败路径保留已提交副作用的真实状态，不伪造回滚成功。

验收：至少完成一条构建/发布/部署真实成功链路，以及失败、取消、审批和撤权终态链路。

### Phase 5：评估是否引入具体框架

只有 Phase 1–4 完成后才做框架选型。

评估条件：

- 如果自研 graph/checkpoint 代码仍然重复 LangGraph 的通用能力，则做小范围 PoC；
- 只迁移 `model → tool → checkpoint → interrupt → resume` 这条最小路径；
- Luna 权限、审批、审计、业务状态和 Tool Adapter 保持独立；
- 对比迁移后的恢复语义、数据库事务边界、流式事件和可观测成本。

禁止一次性把整个 `luna-agent` 重写为框架示例代码。

## 11. 测试和发布门禁

### 11.1 必须具备的测试层次

1. `ModelDecision` 和 `ToolOutcome` 纯函数单测。
2. 状态图转换单测，覆盖每个 phase 和非法转换。
3. checkpoint 乐观锁、重复提交和旧 revision 拒绝的 PostgreSQL 集成测试。
4. Tool Adapter 的参数、权限、审批、幂等和错误码契约测试。
5. SSE/Timeline 只验证事件投影，不验证业务终态逻辑。
6. Agent Run 集成测试覆盖成功、失败、取消、超时、审批、输入和异步恢复。
7. 至少一条 API → Agent → Tool → 外部平台 → 权威回读的 E2E。

### 11.2 必测场景

```text
普通问答一次完成
首步计划文字没有 tool call
工具后计划文字
工具调用带文字
finish_reason 与 tool call 不一致
工具参数错误后有界修复
工具等待审批后恢复
工具等待用户输入后恢复
异步提交后权威回读
模型调用前进程退出
工具成功后进程退出
终态提交响应未知后重试
同一 Run 并发 claim
相同工具和参数重复调用
取消模型和工具请求
Run 超时
目录发现不产生业务完成证据
present_card 不产生业务完成证据
Secret 不进入消息、日志和事件
```

### 11.3 发布门禁

- `go test ./...`
- `pnpm --dir luna-agent test`
- `pnpm --dir luna-agent lint`
- `pnpm --dir luna-agent typecheck`
- OpenAPI 与 Tool Catalog 契约检查
- 相关 PostgreSQL migration up/down/reapply
- Agent 关键 E2E
- 文档构建：`pnpm --dir docs build`

涉及 checkpoint、审批、幂等、Secret、取消和跨服务协议时，不能只以单元测试通过作为合并条件。

## 12. 回滚和兼容

1. 新 checkpoint 表和字段先向后兼容，不删除旧字段。
2. Agent 启动时可以读取旧 Run，但旧 Run 不重新执行未完成的模型步骤；必要时标记为 `interrupted`。
3. 新 Executor 通过 feature flag 仅用于内部测试 Run，生产默认保留旧路径直到集成验收完成。
4. 新旧路径产生的 Timeline 事件必须使用同一稳定事件语义，不能让前端按实现版本分叉。
5. checkpoint 写入失败时 fail closed，不能退回到仅内存 loop 继续执行写操作。
6. 确认所有活动 Run 已完成或中断后，再删除 `pendingFinalAnswer`、旧 nudge 和旧恢复分支。

## 13. 完成标准

整改完成必须同时满足：

- 主循环不再通过普通文字猜测业务是否完成；
- Run 有单一、可持久化、可恢复的控制流状态；
- 工具调用有持久化幂等和清晰的 outcome；
- 审批、输入和异步任务可以从 checkpoint 恢复；
- 业务 completed 只由 final output 和完成证据共同触发；
- 过程消息与最终回答在 Timeline 中可区分；
- 取消、超时、重启和终态持久化失败不会伪造成功；
- Langfuse/OTel 只观测，不改变控制流；
- 至少一条真实平台副作用链路完成成功、失败、取消和权威回读验收；
- 相关测试、迁移、OpenAPI、Agent catalog 和内部文档同步通过。

## 14. 明确不做

- 不为了“更像框架”增加多 Agent 自主讨论、投票或规划层。
- 不把 Langfuse 当 Agent 执行框架。
- 不把平台权限、审批、审计和业务终态交给模型或通用框架。
- 不保留多个互相兜底的 final 判定路径。
- 不通过无限重试、隐藏 prompt、进程内缓存或历史猜测掩盖不可恢复状态。
- 不在没有 checkpoint、迁移、回滚和 E2E 方案前进行全量框架迁移。
