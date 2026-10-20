# Agent 运行时行为矩阵

## 1. 用途与边界

本文件是 [13-AI-Agent运行时整改方案.md](13-AI-Agent运行时整改方案.md) Phase 0 的产出，记录 `luna-agent`
当前 Agent Loop 的**实际行为**、事实源归属和测试缺口，作为 Phase 1–4 的对照基线。

约定：

- 「当前结果」列描述基线代码的真实行为，不代表期望行为；「期望结果」列指向 13 号方案的整改目标。
- 表内行号对应基线提交 `10c5023e`。
- 第 3 节是 Phase 0 的基线快照；Phase 1 之后的行为变化、新增回归位置和遗留风险见第 8 节。
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

Phase 2 起该结论已被 `ai.run_checkpoints` 取代：控制流事实源的变化见第 9 节，
下表保留 Phase 0 的原始快照用于对照。

## 3. 行为矩阵

| # | 场景 | 当前结果 | 期望结果 | 负责模块 | 回归位置 |
| --- | --- | --- | --- | --- | --- |
| 1 | 普通问答，单次模型调用 | 非空文本直接判 `completed` | 保留；由 `finalize` 唯一提交 | `executor/index.ts:263` | `executor-baseline.test.ts`：普通问答一次模型调用完成 |
| 2 | 首步计划文字，无 tool call | 文本被当作最终答复，Run `completed` | 结束原因指示工具调用时改为 `protocol_error` + 一次有界修复（§8.5）；结束原因为 `stop` 或缺失时仍是合法 `final` | `executor/index.ts:263` | `executor-baseline.test.ts`：用例 2 |
| 3 | 工具执行后模型只输出计划文字 | 无新平台调用时文本直接判 `completed` | 需要完成证据（§5.4） | `executor/index.ts:604` | `executor-baseline.test.ts`：用例 3 |
| 4 | 连续空回答 | `finalResponseMissing` 后重试，无独立空答预算，最多 256 个模型步；耗尽得 `ai.final_response_missing` | 统一 Run Budget 与耗尽原因 | `executor/index.ts:267,609` | `executor-baseline.test.ts`：用例 4a/4b |
| 5 | 达到 `maxModelSteps` | `ai.limit_exceeded` | 同上 | `executor/index.ts:609` | `executor-baseline.test.ts`：用例 5 |
| 6 | 工具调用同时带文字 | 文字先进入 timeline `assistant_message`，后续最终答复仍为 `assistant_message`，没有 progress 语义 | 过程文本使用 `progress_message` 语义 | `streaming.ts:104` | `executor-baseline.test.ts`：工具过程文字和最终回答 |
| 7 | 只调用 `search_tools`/`get_tool_details` | 强制回模型，即使模型附带文本 | 发现类不产生完成证据（§9） | `executor/index.ts:598` | `executor-tools.test.ts` |
| 8 | 只调用 `present_card`（presentation） | 回模型继续，不结束 Run | 呈现类不产生完成证据 | `executor/index.ts:600` | `executor-baseline.test.ts`：用例 8 |
| 9 | 调用 `request_input`/`request_choice`（interactive） | Run 直接 `completed` 并结束模型循环，不等用户提交 | 统一为中断/恢复（§5.1） | `executor/index.ts:601` | `executor-baseline.test.ts`：用例 9（input/choice） |
| 10 | 只调用 `navigate_to_route` | 落到"无平台工具 + 有文本"分支判 `completed` | 呈现类不产生完成证据 | `executor/index.ts:604` | `executor-baseline.test.ts`：用例 10（导航有/无同一步文字） |
| 24 | 结束原因宣告工具调用但没有工具调用 | 基线：文本被当作最终答复直接完成 | 现在：注入一次有界修复，仍违规则 `ai.model_protocol_violation` 失败 | `executor/decision.ts`、`executor/index.ts` | `executor-protocol.test.ts` |
| 25 | 工具调用与非工具结束原因同时出现 | 基线：照常执行工具 | 现在：不执行工具，直接 `ai.model_protocol_violation` 失败 | `executor/decision.ts` | `executor-protocol.test.ts` |
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
| 卡片修复 | `runtimeSettings.maxCardRepairAttempts` | 2 | 卡片记录 `ai.interaction_card_schema_invalid`；Phase 1 前主循环一律转文字并 `completed`，现在只在 `final`/`empty` 时退化完成，协议违规改为 `ai.model_protocol_violation` |
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
- `luna-agent/tests/executor-decision.test.ts`：模型决策协议的五组纯函数组合。
- `luna-agent/tests/executor-protocol.test.ts`：有界修复、修复后继续执行、结束原因矛盾时拒绝执行工具。

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

后续补充：上述两项环境前置已解除。

- Node：本机 nvm 安装 v24.21.0，与 CI 的 `node-version: "24"` 对齐。
- PostgreSQL：在 `docker compose -f docker-compose-dev-db.yaml` 的容器内新建独立库 `luna_agent_test`，
  按 CI 的 `scripts/ci/prepare-agent-test-database.sh` 方式应用 `migrations/*.up.sql`，再以
  `AGENT_TEST_DATABASE_URL=postgres://devops:devops@127.0.0.1:5432/luna_agent_test?sslmode=disable` 运行。
  该库必须独立于 compose 的 dev 库 `devops`，集成用例会清空 `ai` schema。本机没有宿主机 `psql`，
  而 prepare 脚本依赖它，本次改用容器内 `psql` 执行同一批迁移文件；要直接复用脚本需先安装 libpq。
- 结果：Node v24.21.0 + 真实 PostgreSQL，`pnpm --dir luna-agent test` 45 个文件、417 个用例全部通过，无跳过。

## 8. Phase 1 变更记录

状态：主体完成，**未完全通过验收**。终态协议只做到"完成判定不再依据文本"，完成证据（§5.4）属于 Phase 3–4；
流式投影未收敛（见下）。

改动范围：新增 `executor/decision.ts` 纯函数，主循环只消费 `ModelDecision`；`streaming.ts` 透出
`finishReason`；删除主循环中两处依据文本结束 Run 的判定（原 `executor/index.ts:263` 与 `:604`）；
卡片修复耗尽分支改为消费同一个决策；终态遥测记录实际 `finish_reason`。

行为变化：

- 矩阵行 10 变化：同一步"导航 + 文字"不再结束 Run，改由下一步的 `final` 决策结束，模型步数从 1 变 2。
  原因是删除了"无平台工具 + 有文本"分支，完成判定只认结构化决策。用例 10 的断言已按此重写，测试名不再带"当前行为"。
- 矩阵行 2、3、4 行为未变：结束原因缺失或为 `stop` 时纯文本仍是合法 `final`，空回答仍走原有重试与
  `ai.final_response_missing`，所以基线用例 2、3、4a、4b 保持通过。它们记录的不是"猜文本"这一实现方式，
  而是完成判定的最终结论，结论未变即无需重写。
- 新增矩阵行 24、25：协议违规由稳定错误码 `ai.model_protocol_violation` 失败，并新增
  `web/src/i18n/locales/*/aiAssistant.ts` 的 `model_protocol_violation` 文案与
  `web/src/components/common/ai-assistant/errors.ts` 映射。
- 卡片修复耗尽分支不再绕过决策：此前它只检查 `toolCalls` 是否为空，结束原因宣告工具调用但没有工具调用时
  仍会把 Run 判为 `completed`；现在只有 `final`、`empty` 才退化为文字并完成，`missing_tool_calls` 以
  `ai.model_protocol_violation` 失败，其余情况（工具调用或结束原因矛盾）维持 `ai.interaction_card_schema_invalid`。
  回归位置：`executor-protocol.test.ts` 的"卡片修复耗尽"用例；回退该修复可复现 `completed` 的错误终态。
- 结束原因边界收紧：无工具调用时只有缺失结束原因、`stop` 或 `end_turn` 可以形成 `final`；`length`、
  `content_filter` 和未知值统一以 `ai.model_protocol_violation` 失败，避免截断或被过滤的文本伪装成完整回答。

未完成项：

- 流式投影未收敛：`streaming.ts:164` 在模型步内就把正文写成已完成的助手消息，`:177` 仍以 `answer.trim()`
  判断是否保留最终会话流。协议违规失败时，被拒的计划文字会以 `assistant_message` / `completed` 落库，
  与 §6.6 的"未提交草稿或 `progress_message` 语义"冲突。该项按方案归入 Phase 3，Phase 1 不宣称已解决。
- 完成证据未落地：模型输出合法 `final` 仍然是当前唯一的完成依据，写操作与异步操作的权威回读属于 Phase 3–4。

风险：

- 行 25 是新增失败面：此前"结束原因与工具调用矛盾"会照常执行工具，现在直接失败。若真实 Provider 存在该组合，
  会从可用变成失败；判定依据是方案 §5.2，观察方式是 `agent.run.failed` 的 `error.code` 与 `gen_ai.response.finish_reasons`。
- 有界修复次数与协议违规计数保存在进程内存，重启后重置，Phase 2 的 checkpoint 需要纳入。
- 结束原因收紧把被截断的回答（`length`）从"用部分文本完成"改成失败：这是有意的取舍，截断文本不应冒充
  完整答复，但用户可观察结果会从"拿到半截答案"变成"看到失败"，需要通过真实流量观察发生频率。

验证记录：

- 目标测试：`executor-decision.test.ts`、`executor-protocol.test.ts`、`executor-baseline.test.ts` 共 30 个用例通过。
- `tsc --noEmit` 与改动文件 `eslint` 无诊断。
- `pnpm --dir luna-agent test`：Node v24.21.0 + 真实 PostgreSQL（独立库 `luna_agent_test`），
  45 个文件、417 个用例全部通过，无跳过。沙箱内运行会因禁止监听 `127.0.0.1` 让
  `openai-chat-completions-http.test.ts` 报 `listen EPERM`，属环境限制而非代码问题，
  但沙箱内的结果不能当作门禁通过。
- `web` 的 `errors.test.ts` 与 `scripts/check-i18n.mjs` 通过。
- 仍未验证：真实 E2E（API → Agent → Tool → 外部平台 → 权威回读），属 Phase 4 入口条件。

## 9. Phase 2 变更记录

状态：主体接入完成。四个进程退出边界（模型调用前、工具完成后、审批等待后、终态提交前）
已可恢复；真实进程崩溃与 PostgreSQL 持久性仍未验证，见 9.6。

### 9.1 改动范围

- 新增迁移 `migrations/000105_ai_run_checkpoints.{up,down}.sql` 与表 `ai.run_checkpoints`
  （`run_id` 主键、`revision` 乐观锁、`phase`、`model_step`、`tool_call_count`、`recoveries`、
  `state jsonb`），并同步 Drizzle schema、领域类型和 Go 侧迁移门禁
  （`internal/database/fresh_database_migration_test.go` 的 `latestMigrationVersion=105`）。
  同一迁移为工具事实增加 `execution_count`，进入 `running` 时与状态一起更新；
  参数校验失败、审批拒绝不消耗执行次数，HTTP 428 及批准后的再次执行分别计数。
- 新增 `luna-agent/src/executor/checkpoint.ts`：`resumePlan` 把持久化相位映射为续跑动作，
  `checkpointState` 组装状态字段。
- `Repository` 新增 `loadRunCheckpoint` / `saveRunCheckpoint` / `recoverOrphanedRun`，
  PostgreSQL 与内存实现同步；`saveRunCheckpoint` 用 `revision` 单调推进做乐观锁，
  同一 revision 重放幂等，旧 revision 抛 `RunCheckpointConflictError`。
- `RunExecutor` 在五个边界写 checkpoint：每个模型步之前（`model`）、工具批次之前
  （`tool_execution`）、等待审批（`awaiting_approval`）、等待用户输入（`awaiting_input`）、
  终态提交之前（`finalizing`）。恢复时从 checkpoint 取阶段、模型步数、循环保护计数和
  协议/参数修复次数，不再从 Timeline 或进程内存推断位置。
- `tool_execution` 只是批次边界，不是结果：恢复时用工具事实表 `ai.tool_calls`
  （`ToolCallStore.listByRun` / `ToolOrchestrator.recoverToolBatch`）核对批次落地情况，
  已落地的调用折叠结果继续、`running` 的调用失败关闭、停在审批的回到等待审批。
  进程在“工具成功后、下一个 checkpoint 之前”退出属于这一路径，不需要重放工具。
- 核对同时把崩溃批次已落地的调用计入循环保护计数（`LoopGuard.raiseFromSettledCalls`）：
  计数只抬高不降低，重启后 Run 的工具预算与重复调用保护不放宽。
- 启动对账由 `interruptOrphanedRuns` 改为 `recoverOrphanedRuns`：有 checkpoint 的孤儿 Run
  重新排队续跑，恢复次数上限 3（`maxRunRecoveries`）；没有 checkpoint 的旧 Run 仍然置为
  `interrupted`（方案 §12.2）。
- 循环保护计数跨重启保留：`InMemoryLoopGuard` 新增 `exportRun` / `restoreRun`，
  `ToolOrchestrator.runLoopState` / `restoreRunLoopState` 供 Executor 读写 checkpoint。
  计数键导出时拆成 `{operationId, argumentsHash}`，因为 jsonb 不能保存 NUL 分隔符。

### 9.2 行为变化

- 矩阵行 15 变化：进程在模型调用前、工具完成后、审批等待后、终态提交前退出时，Run 不再直接
  进入 `interrupted` 终态，而是重启后续跑且状态只执行一次。回归位置：`executor-checkpoint.test.ts`
  的四个边界用例；旧行为由 `executor-baseline.test.ts` 用例 15 保留为“无 checkpoint 的旧 Run”。
- 矩阵行 11 变化：审批恢复不再只靠 timeline 反推上下文，等待中的调用由 checkpoint 记录；
  审批恢复与批次恢复统一核对工具事实，恢复计数并持久补齐缺失的 Timeline 投影，
  已有投影不重复写入；结果进入续跑上下文时不重复折叠（回归用例断言续跑上下文里
  该工具结果只出现一次）。
- 矩阵行 12 变化：等待用户输入时写入 `phase=awaiting_input`（需补充的字段仍只由
  `run.input_required` 事件承载，checkpoint 不重复保存），重新排队后从同一步继续，
  修复次数与循环保护计数不再重置。
- 新增行 26：工具批次中断后恢复。按工具事实核对：批次已落地则把结果折叠进续跑上下文并继续，
  不重复执行工具（工具成功但 checkpoint 仍停在 `tool_execution` 也在此覆盖）；调用停在
  `awaiting_approval` 则回到等待审批；调用处于 `running`（请求可能已送达外部平台）则以
  `ai.tool_outcome_unknown` 失败关闭；只有 `proposed` 或没有记录时没有副作用，回模型重新决策。
  投影缺失（事实已提交、Timeline 条目未写入）时先持久补齐调用与工具结果，再推进 checkpoint；
  补写中途再次退出时按已存在条目继续核对。批次同时存在审批等待与 `running` 时优先失败关闭。
- 新增行 27：恢复次数达到上限。Run 置 `interrupted` 且 `errorCode=ai.agent_restarted`，
  不再续跑，避免崩溃-恢复循环。
- 新增行 28：终态批次尚未提交（`phase=finalizing`）。恢复时不再调用模型：checkpoint 记录过
  未提交答复时重建助手消息与终态转换（仍走同一条流式批次路径），否则只补提交终态。

### 9.3 与方案 §6.3 / §7.1 的偏差

- `selectedOperationIds` 不写入 checkpoint：`ai.runs.selected_operation_ids` 已经是持久化事实，
  重复保存会产生两份互相漂移的状态。
- `pendingToolCallIds` 收敛为单个 `pendingToolCallId`：Executor 在第一个等待审批的调用处停止，
  列表没有消费者。
- `budget` 不写入 checkpoint：每次 claim 的 `executionSnapshot` 已持久化该 Run 的
  `runtimeSettings`，恢复后使用的是启动时的原始预算。
- `state.loopGuard.executed` 不单独保存：由 `tool_call_count` 列承载，避免同一事实两处存储。
- `pendingExternalOperationIds`、`lastDecisionId` 未引入：异步权威回读属于 Phase 4，当前无消费者。
- checkpoint 与工具事实仍然分属不同操作。当前顺序为工具事实先提交、投影写入后推进 checkpoint；
  审批与批次恢复均核对事实并补齐持久投影，计数以 checkpoint 与事实计数的最大值恢复。
  内存守卫在持久化前消耗预算，因此存储失败时不能保证 checkpoint 计数必定小于或等于事实计数。

### 9.4 未完成项

- 模块拆分（`decision`、`graph`、`tool-step`、`finalize`）已由 Phase 3 完成，见第 10 节。
- `ToolOutcome`、写操作 `verificationRequired` 与权威回读仍是 Phase 4 范围；`completed`
  目前仍只由模型合法 final 决定。
- 工具调用的逻辑幂等键与崩溃后重放（方案 §7.2）未做：恢复按工具事实核对，不重放已落地的
  调用；只有结果确实未知（`running`）时才失败关闭，不用新调用掩盖未知结果。
- 仍未实现“工具事实、投影与 checkpoint 同一事务”：外部 HTTP 无法纳入数据库事务，
  但 HTTP 返回后的同库事实、投影与 checkpoint 可以事务化，当前尚未合并这三个写入边界。
  当前依靠事实先行与恢复核对；核对读取或补写投影失败时 Run 以稳定错误码失败关闭，
  checkpoint 停原处等待人工核对，不猜测推进。
- 模型调用进行中退出会重跑该模型步：模型调用不产生业务副作用，四个验收边界也不包含
  调用中途，因此不引入调用级 checkpoint。
- 流式投影已由 Phase 3 收敛，计划文字改写为 `progress` 投影，见第 10 节。

### 9.5 风险

- 恢复会重新调用模型并再次计费：模型步边界崩溃后需要重新生成该步输出。
- `finalOutput` 保存脱敏后的最终答复副本，用于重建未提交的终态批次；未提交批次的文本
  因此短暂存在于两张表，Run 进入终态后 checkpoint 行不再被读取。
- 启动恢复依赖单副本部署：多副本下 `running` Run 可能属于其他实例，恢复会误判为孤儿。

### 9.6 验证记录

- 本轮修复后以 Node v24.21.0 直接执行本地 `vitest run`，连接独立 PostgreSQL 测试库：
  46 个文件、447 个用例全部通过，无跳过。新增覆盖审批恢复核对、精确执行计数、
  缺失投影持久补齐后再次恢复、重复核对与补写失败关闭；真实 PostgreSQL 验证计数存取及非负约束。
  Agent 的 `eslint`、类型检查与构建通过；Go 全量测试和真实数据库迁移检查通过；
  Web 515 个用例、lint、i18n、单例检查与构建通过。Helm 检查因缺少 `helm` 未完成；
  文档站构建因缺少本地 `rspress` 依赖未完成。
- `pnpm --dir luna-agent test`：Node v24.21.0 + 真实 PostgreSQL（独立库 `luna_agent_test`），
  46 个文件、440 个用例全部通过，无跳过。其中 `executor-checkpoint.test.ts` 覆盖四个退出边界、
  工具批次已落地/投影缺失/未开始/`running`/停在审批五种批次状态、崩溃批次计数恢复（移除
  `raiseFromSettledCalls` 时这两条用例会失败）、核对失败失败关闭、恢复次数上限与 revision 乐观锁。
- `luna-agent` 的 `eslint` 与 `tsc --noEmit` 无诊断。
- `000105` 迁移在空库完成 up、down、reapply；`go test ./internal/database/` 的
  `TestMigrateBootstrapsFreshPostgresSchema`（含最终 Schema 与 dirty 状态断言）通过。
- `go test ./...` 全部通过；`web` 的 `vitest run`（133 文件、515 用例）、`eslint`、
  `check:i18n`、`check:singletons` 通过。
- 新增 `web/src/i18n/locales/*/aiAssistant.ts` 的 `tool_outcome_unknown` 文案与
  `web/src/components/common/ai-assistant/errors.ts` 映射。
- 仍未验证：真实 E2E（API → Agent → Tool → 外部平台 → 权威回读），属 Phase 4 入口条件。
  进程退出用“重建 durable 状态 + 新 Executor”模拟，不代表真实进程崩溃与 PostgreSQL 持久性验证。

## 10. Phase 3 变更记录

状态：模块拆分与验收缺口修复完成后通过主体验收。Phase 4 的工具 outcome、写操作验证和
异步权威回读仍未开始。

### 10.1 改动范围

- `RunExecutor` 从 1126 行收缩为 295 行，只保留 Run claim、配额、并发控制、超时、
  取消、启动对账、远端配置刷新和组件装配。
- 新增 `executor/run-loop.ts`，负责单个 Run 的模型步骤、checkpoint 推进、恢复动作和
  TerminalPersistence 错误分支。
- 新增 `executor/graph.ts`，把 `ModelDecision`、工具批次结果和 checkpoint 恢复计划
  映射为显式动作：`execute_tools`、`finalize`、`repair`、`retry`、`fail`、
  `continue_model`、`waiting_approval`。
- 新增 `executor/tool-step.ts`，集中执行工具批次、内部工具、搜索/详情缓存、卡片生成、
  导航、会话重命名、平台工具调用和恢复批次核对。
- 新增 `executor/finalize.ts`，独占 `completed` 提交、终态失败/取消/中断、未提交答复重建，
  并把标题生成移到 Run 完成后以 best-effort 执行。
- `Resume` 辅助函数从 `executor/index.ts` 移到 `resume.ts`，由 `ToolStepRunner.initialState`
  统一恢复续跑上下文、缓存和已选工具。
- `TimelineItem` 增加 `progress` 类型；`RunStreamSession.updateItemWithEvent` 支持在提交前
  修正条目类型。
- `RunFinalizer` 只在确认实际状态已是终态时忽略 `RunStateConflictError`；数据库故障和
  `TerminalPersistenceExhaustedError` 上抛到统一持久化失败处理。
- 自动标题生成改为终态提交后的受控任务：使用 Run 的 AbortSignal，并设置 4 秒上限；
  标题失败不影响 Run，但任务必须结束并写入 `run.postprocess.completed`。
- `RunStreamHub` 对刚完成的 Run 等待 postprocess 标记，再把标题更新和终态事件交给 SSE；
  前端在收到 `run.stream.completed` 后才关闭订阅。
- 前端订阅协调保留最新完成 Run，直到对应流的 Manager 状态进入 `terminal`；页面刷新后
  也会为最新完成 Run 重建一次订阅，避免绕过后处理等待。
- 已订阅但尚未收到流结束事件的 completed Run 在下一轮开始后仍然保留，避免连续发送时
  丢失上一轮的自动标题更新。
- `run.completed` 只结束业务状态，不再把连接状态设为 `terminal`；只有
  `run.stream.completed` 才结束 SSE。Manager 会保留已结束流的 terminal 记录，避免重复重连。
- 序列缺口恢复在 completed 状态下继续重连，只有 failed/canceled/interrupted 才直接判定流结束。
- 服务端对终态 Run 的 SSE 重连同样创建 Hub 订阅，由 Hub 判断是否等待 postprocess。
- 管理员观测的 Run item 枚举与查询加入 `progress`，避免过程文字在观测页丢失。
- 前端观测 DTO 与 OpenAPI 枚举同步包含 `progress`，管理台 Loop 视图可正常渲染。
- 管理员 Loop 视图渲染 `progress` 项，并在五种语言中提供“执行过程”标签。
- 直接工具路径改由 `RunFinalizer.completeDirectTool` 提交，返回真实 `completed`/`failed`
  outcome，不再绕过 finalize 或误记完成指标。
- `RunFinalizer` 的完成、失败、取消和中断在终态竞争时都返回数据库实际终态；RunLoop 使用该值
  记录 outcome。完成竞争不生成标题或 postprocess，失败/取消/中断也不会覆盖已落库的实际终态。
- RunLoop 的取消分支直接接收 `finalizer.cancel()` 返回值，不再保留初始 `completed`；
  RunLoop 级测试覆盖模型流取消后返回 `canceled`。
- 错误处理中的卡片失败落盘异常进入统一终态持久化处理，不再跳过取消、失败或中断提交。
- `graph.ts` 为每个 `RunPhase` 定义合法动作集；决策、工具批次和恢复动作都会校验，
  非法转换抛 `ai.run_graph_illegal_transition`。
- Run claim 成功后立即进入统一生命周期：快照校验、目录恢复、并发查询、配额拒绝和
  工具预算装配都在同一异常边界内；初始化失败通过 `failBeforeExecution` 收敛为明确终态。
- `awaiting_input` 的 checkpoint、`run.input_required` 事件和 Run 状态更新作为一个
  持久化步骤；任一步失败都进入统一终态持久化处理，不再停留在 `running`。

### 10.2 行为变化

- 矩阵行 6 变化：模型在同一工具调用步骤输出的过程文字不再落为 `assistant_message`，
  而是 `progress`；只有合法 `final` 决策产生 `assistant_message`。
- Phase 1 遗留的协议违规草稿不再冒充最终回答：`missing_tool_calls` 的修复草稿和失败终态
  都保留为 `progress`。
- 取消时已经流出的部分正文以 `progress` / `failed` 提交，不产生最终回答。
- 标题生成不再参与终态事务，也不会因标题模型失败改变 Run 终态；完成事件先提交，标题随后更新。
- 自动标题更新通过 `conversation.title.updated` 送达当前会话；完成 Run 的 SSE 在标题任务
  结束并收到 postprocess 标记后才发送 `run.stream.completed`，前端不再在 `run.completed`
  时提前关闭订阅。
- `RunExecutor` 不再包含 `search_tools`、`get_tool_details`、卡片 Schema、导航或具体平台工具
  分支；这些逻辑只在 `ToolStepRunner` 中存在。

### 10.3 未改变或仍待后续

- `ToolOutcome`、`verificationRequired`、写操作权威回读、异步外部状态仍属 Phase 4。
- `completed` 仍由模型合法 `final` 或既有交互卡片完成分支触发；Phase 3 只收敛提交入口，
  不扩展业务完成证据。
- `decision.ts` 和 `checkpoint.ts` 保持 Phase 1/2 的结构，Phase 3 只把它们接入显式状态图。
- 直接工具路径仍保留原有的“工具事实已落地即结束 Run”语义；Phase 4 再统一到业务验证。
- 真实进程崩溃、多副本恢复和 PostgreSQL 持久性仍未在 Phase 3 重新验证。

### 10.4 验证记录

- 状态图纯函数测试 `executor-graph.test.ts`：5 个用例通过。
- Executor、协议、checkpoint、工具、状态图、finalize 和流式 hub 回归共 85 个用例通过，覆盖普通问答、工具过程文字、
  审批恢复、输入恢复、取消、终态持久化和 checkpoint 退出边界。
- Agent `tsc --noEmit` 与 `eslint .` 通过。
- 沙箱外全量 `vitest run`：46 个测试文件通过、2 个跳过；445 个用例通过、22 个跳过。
  跳过项依赖真实 PostgreSQL 或基准测试数据库，本轮未启动对应数据库。
- Web 全量 `vitest run`：134 个测试文件、519 个用例通过。
- `go test ./internal/agentobservability` 通过。
