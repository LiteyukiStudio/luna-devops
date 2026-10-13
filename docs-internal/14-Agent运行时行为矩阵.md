# Agent 运行时行为矩阵

## 1. 用途与边界

本文件是 [13-AI-Agent运行时整改方案.md](13-AI-Agent运行时整改方案.md) Phase 0 的产出，记录 `luna-agent`
当前 Agent Loop 的**实际行为**、事实源归属和测试缺口，作为 Phase 1–4 的对照基线。

约定：

- 「当前结果」列描述基线代码的真实行为，不代表期望行为；「期望结果」列指向 13 号方案的整改目标。
- 表内行号对应基线提交 `10c5023e`。
- 基线为当前已合入实现；未合入 PR #20 的补偿改动不在记录范围内。
- 本文件不是 API 契约。字段、错误码和迁移以代码与生成物为准。

## 2. 事实源清单

| 事实 | 现在存于 | 写入方 | Agent 重启后 |
| --- | --- | --- | --- |
| Run 生命周期状态 | `ai.runs.status` + `row_version` | `Repository` 各状态方法 | 保留；`running` 会被启动对账置为 `interrupted` |
| 工具执行事实 | `ai.tool_calls.status/result/attempt/arguments_hash`（平台工具）及 `ai.items`/events（内部搜索、详情、导航、标题、卡片） | `ToolOrchestrator` + `ProjectingToolCallStore` + 各领域 Repository | 保留 |
| 用户可见投影 | `ai.items` + `ai.run_events` | `RunStreamSession`、Repository、内部工具/卡片服务与 `ProjectingToolCallStore` | 已提交内容保留，尚未提交的 session 正文丢失 |
| 单副本实时缓冲 | `InMemoryRunStreamBus` | `RunStreamSession.publish` | 丢失（仅降低 SSE 延迟，非事实源） |
| 终态提交意图 | `RunStreamSession.terminalIntent`（进程内存） | `commitTerminal` | 丢失；不会从 Timeline 恢复完成意图，遗留 `running` 置为 `interrupted` |
| Run 内工具循环计数 | `InMemoryLoopGuard`（进程内存） | `ToolOrchestrator` | 丢失；审批/输入挂起时不清空，同进程内可延续 |
| 模型参数修复次数 | 主循环局部 `Map providerArgumentRepairAttempts` | `RunExecutor` | 丢失 |
| 卡片修复次数 | timeline item content 的 `attempt/maxAttempts` + 进程内 `CardGeneration` | `CardGenerationService` | 计数保留，生成态丢失 |
| 已选工具集合 | `ai.runs.selected_operation_ids` + timeline 反推 | `touchRunSelectedOperations` | 保留 |
| 搜索/详情结果缓存 | timeline 反推 | `restoredInternalToolResults` | 重建，只覆盖有 `result` 的调用 |
| 暂停前的工具消息 | timeline 反推 | `resumedToolMessages` | 重建，只覆盖有 `tool_result` 的调用 |

关键结论：恢复所需的控制流信息（阶段、模型步数、剩余预算、待处理调用）目前没有一处权威存储，
只能由 timeline + Run 状态 + 进程内存拼接。这是 13 号方案 §5.3 要引入 checkpoint 的直接原因。

## 3. 行为矩阵

| # | 场景 | 当前结果 | 期望结果 | 负责模块 | 回归位置 |
| --- | --- | --- | --- | --- | --- |
| 1 | 普通问答，单次模型调用 | 非空文本直接判 `completed` | 保留；由 `finalize` 唯一提交 | `executor/index.ts:263` | `executor-baseline.test.ts`：普通问答一次模型调用完成 |
| 2 | 首步计划文字，无 tool call | 文本被当作最终答复，Run `completed` | `protocol_error` + 一次有界修复（§8.5） | `executor/index.ts:263` | `executor-baseline.test.ts`：用例 2 |
| 3 | 工具执行后模型只输出计划文字 | 无新平台调用时文本直接判 `completed` | 需要完成证据（§5.4） | `executor/index.ts:604` | `executor-baseline.test.ts`：用例 3 |
| 4 | 连续空回答 | `finalResponseMissing` 后重试，无独立空答预算，最多 256 个模型步；耗尽得 `ai.final_response_missing` | 统一 Run Budget 与耗尽原因 | `executor/index.ts:267,609` | `executor-baseline.test.ts`：用例 4a/4b |
| 5 | 达到 `maxModelSteps` | `ai.limit_exceeded` | 同上 | `executor/index.ts:609` | `executor-baseline.test.ts`：用例 5 |
| 6 | 工具调用同时带文字 | 文字先进入 timeline `assistant_message`，后续最终答复仍为 `assistant_message`，没有 progress 语义 | 过程文本使用 `progress_message` 语义 | `streaming.ts:104` | `executor-baseline.test.ts`：工具过程文字和最终回答 |
| 7 | 只调用 `search_tools`/`get_tool_details` | 强制回模型，即使模型附带文本 | 发现类不产生完成证据（§9） | `executor/index.ts:598` | `executor-tools.test.ts` |
| 8 | 只调用 `present_card`（presentation） | 回模型继续，不结束 Run | 呈现类不产生完成证据 | `executor/index.ts:600` | `executor-baseline.test.ts`：用例 8 |
| 9 | 调用 `request_input`/`request_choice`（interactive） | Run 直接 `completed` 并结束模型循环，不等用户提交 | 统一为中断/恢复（§5.1） | `executor/index.ts:601` | `executor-baseline.test.ts`：用例 9（input/choice） |
| 10 | 只调用 `navigate_to_route` | 落到"无平台工具 + 有文本"分支判 `completed` | 呈现类不产生完成证据 | `executor/index.ts:604` | `executor-baseline.test.ts`：用例 10（导航有/无同一步文字） |
| 11 | 平台工具等待审批 | Run 置 `waiting_approval` 并退出；审批后置回 `queued` 重新 claim，靠 timeline 反推上下文 | 由 checkpoint 恢复（Phase 2） | `executor/index.ts:588` | `executor-tools.test.ts:543` |
| 12 | 工具缺必填参数 | `ToolInterruption` → 事件 `run.input_required` + Run `waiting_input` | 同上 | `orchestrator.ts:73`、`index.ts:724` | `executor-baseline.test.ts`：缺必填参数先等待输入 |
| 13 | 取消 | abort + DB `cancelRun` 置 Run 与未终态 ToolCall 为 `canceled` | 保留；不伪造成功 | `index.ts:113`、`postgres.ts:610` | `executor-tools.test.ts:432` |
| 14 | 终态持久化耗尽 | 尽力置 `interrupted`，不伪造 `completed` | 保留（§12.5 fail closed） | `index.ts:637`、`run-stream-bus.ts:304` | `executor-tools.test.ts:340` |
| 15 | 进程退出（模型步间/工具完成后） | 内存计数丢失；timeline 反推部分状态；`running` 被对账置 `interrupted`，不续跑 | checkpoint 精确恢复（Phase 2） | `index.ts:759`、`resume.ts` | `executor-baseline.test.ts`：用例 15（三个重启边界） |
| 16 | 同一 Run 并发 claim | `for update skip locked` + `row_version` 乐观锁 | 保留 | `postgres.ts:660` | `postgres-repository.test.ts:74`（真实 PG，当前跳过） |
| 17 | 相同工具与参数重复调用 | 第 4 次同 `operationId + canonical args hash` 熔断，作为工具结果回灌 | 持久化幂等（§7.2） | `loop-guard.ts:84` | `loop-guard.test.ts` |
| 18 | Run 超时 | `ai.run_timeout` → Run `failed` | 统一 Run Budget | `index.ts:184` | `executor-baseline.test.ts`：用例 18 |
| 19 | 用户并发配额超限 | claim 后直接置 Run `failed`，`ai.quota.user_concurrent_runs_exceeded` | 保留 | `index.ts:163` | `executor-baseline.test.ts`：用户并发配额超限 |
| 20 | 目录发现不产生完成证据 | 成立（用例 7） | 保留（§9） | `executor/index.ts:598` | `executor-tools.test.ts:693` |
| 21 | Secret 不进入消息、日志和事件 | 现有脱敏单测覆盖成立 | 保留 | `redaction.ts` | `redaction.test.ts`、`telemetry-content.test.ts`；非完整 E2E 保证 |
| 22 | 异步构建提交返回 HTTP 202、外部状态仍 queued | 工具 `succeeded`；下一步纯文字使 Run `completed`，没有外部状态回读 | `waiting_external` + 状态查询和权威验证 | `tools/orchestrator.ts`、`executor/index.ts` | `executor-baseline.test.ts`：异步场景 |
| 23 | 终态落库成功但提交响应未知 | 同批次重试，唯一 completed 终态与消息 | 保留幂等提交 | `run-stream-bus.ts`、`persistence/postgres.ts` | `run-stream-bus.test.ts`：`retries an unknown commit result idempotently without duplicating the completed terminal` |

## 4. 预算与终态错误码现状

| 预算 | 当前位置 | 值 | 耗尽后 |
| --- | --- | --- | --- |
| 模型步数 | `runtimeSettings.maxModelSteps` | 256 | `ai.limit_exceeded` |
| 工具调用数 | `InMemoryLoopGuard`（proposed/executed 各计一次） | 256 | `ai.run_tool_call_budget_exceeded`，作为工具结果回灌 |
| 同参数重复 | `InMemoryLoopGuard.sameCallLimit` | 3 | `ai.tool_repeated_in_run`，作为工具结果回灌 |
| 卡片修复 | `runtimeSettings.maxCardRepairAttempts` | 2 | 卡片记录 `ai.interaction_card_schema_invalid`，主循环仍可转文字并 `completed` |
| 空回答重试 | 无独立预算 | 仅受模型步数约束 | `ai.final_response_missing` |
| Run 超时 | `runtimeSettings.runTimeoutMs` | 3600000 ms | `ai.run_timeout` |

上述预算分属进程内存、远程配置和局部变量三处，耗尽后的用户可观察行为不一致，对应 13 号方案 P2。

## 5. 覆盖与缺口

已有覆盖：

- 审批挂起与恢复、catalog 选择与缓存重放、取消竞态、启动对账、终态持久化耗尽：`executor-tools.test.ts`。
- 工具参数校验、审批决策、非重试熔断、Run 级取消：`tools.test.ts`、`approval-server.test.ts`。
- 循环保护计数：`loop-guard.test.ts`。timeline 与事件投影：`event-sequence.test.ts`、`run-stream-bus.test.ts`。

Phase 0 特征化测试映射：

- `luna-agent/tests/executor-baseline.test.ts`：用例 1–6、8–10、12、15、18、19、22；覆盖普通问答、计划文字、空答与模型步数耗尽、卡片/导航终态、输入恢复、超时、配额和重启边界。
- `luna-agent/tests/executor-tools.test.ts`：审批、取消、终态持久化耗尽等既有回归。
- `luna-agent/tests/run-stream-bus.test.ts`：用例 23，未知提交结果后的幂等重试。
- `luna-agent/tests/postgres-repository.test.ts`：用例 16，并发 claim 已存在，但依赖真实 PostgreSQL，当前跳过。
- `luna-agent/tests/loop-guard.test.ts`、`tools.test.ts`、`redaction.test.ts`：重复工具、参数校验和脱敏覆盖。

Phase 0 验收已完成：所需场景具备当前结果、期望结果、负责模块、回归位置和最小复现。
新增测试使用脚本模型、API 替身和内存 Repository，调用真实 Executor、工具编排和流式提交逻辑。
输入恢复直接追加输入并重新排队，不覆盖 HTTP 输入入口；重启用新 Executor 启动对账、保留仓储事实并丢弃旧流缓冲，
不代表真实进程崩溃或 PostgreSQL 持久性验证。真实 PostgreSQL 落库、Secret 全链路 E2E 与精确 checkpoint 恢复
不作为 Phase 0 已完成项，分别留到后续阶段。

### 5.1 失败案例最小复现

执行 `pnpm --dir luna-agent test tests/executor-baseline.test.ts`。测试锁定的是当前错误行为，后续整改时应改为期望行为断言。

| 失败案例 | 最小输入与步骤 | 当前可观察结果 | 测试定位 |
| --- | --- | --- | --- |
| 首步计划误完成 | 模型只输出“我先梳理一下执行步骤”，无工具 | 一次模型调用，Run completed，零工具副作用 | 用例 2 |
| 工具后计划误完成 | getProject 返回 200，下一步模型只输出计划 | 工具 succeeded，Run completed，无完成验证 | 用例 3 |
| 异步提交提前结束 | createBuild 返回 202 + queued，下一步输出“已提交” | 只有一次提交请求，无状态查询，Run completed | 异步场景 |
| 交互卡片提前结束 | 模型调用 request_input 或 request_choice | 一次模型调用后 completed，不等待用户提交 | 用例 9 |
| 空答消耗完整模型预算 | 模型持续返回空答且无工具 | 256 个模型步后 failed / ai.final_response_missing | 用例 4b |
| 过程与最终文字混用 | 模型输出文字并调用工具，下一步输出最终答复 | 两条消息均为 assistant_message | 工具过程文字和最终回答 |
| 重启不续跑 | claim 后在模型前、工具后或终态提交前丢弃进程状态，新 Executor.start | interrupted / ai.agent_restarted；已存工具事实保留，未提交正文丢失，零模型调用 | 用例 15 |

## 6. 冻结规则

Phase 0 期间 `RunExecutor` 主循环不再新增特殊分支：不增加确认次数、prompt nudge 或兜底条件。
允许的改动仅限明确缺陷修复，且必须同步更新本矩阵。Phase 1 引入 `ModelDecision` 后，用例 2、3、4
相关的特征化测试必然需要重写，重写清单随 Phase 1 一并给出。

## 7. 基线验证记录

`pnpm --dir luna-agent test`：41 个测试文件通过、2 个跳过；387 个用例通过、19 个跳过，无失败（Node v22.22.2，项目声明要求 Node >=24，当前仅作基线验证）。

两个跳过的套件是 `postgres-repository.test.ts` 与 `postgres-model-usage.test.ts`，未设置
`AGENT_TEST_DATABASE_URL` 时整体跳过；该变量指向专用、可销毁的测试数据库。因此本表用例 16（并发 claim）与所有依赖真实 PostgreSQL 的落库路径
（`persistRunStreamBatch`、`interruptOrphanedRun`、`finalizeStreamingItems`）在本次基线中没有被验证，
Phase 2 开始前必须先具备可运行的 PostgreSQL 测试环境。

Agent lint 与 typecheck 均通过，无诊断。实际执行包内 `node_modules/.bin/eslint .` 与
`node_modules/.bin/tsc -p tsconfig.json --noEmit`，等同 package.json 中的 lint/typecheck 脚本。
本次生产源码无改动，未提交或推送。

全量测试的 global setup 会执行 `go run ./internal/aitool/testdata/export_platform_catalog.go`。
本次沙箱内 pnpm 启动报 `fetch failed`，沙箱外执行通过；不能据此认定 global setup 需要网络。
