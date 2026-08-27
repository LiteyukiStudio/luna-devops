# luna-agent

Luna DevOps 的 AI 助手运行时。一个**自研的、以 PostgreSQL 为权威状态的 Agent 执行器**——没有使用 LangGraph / Vercel AI SDK 等编排框架，这是有意为之的技术选型，本文档解释架构、选型理由和扩展规范。

## 为什么不使用 Agent 框架

核心原因：这个 Agent 的计费、安全、可观测要求**穿透控制流的每一步**，框架在这些地方全部是负资产：

- 每次模型调用前都要按模型能力、上下文剩余和个人钱包余额完成单次用量预留与输出收紧，调用后再按四类 token 价格（输入 / 输出 / 缓存输入 / 缓存输出）结算到**用户个人钱包**；Run 不设置累计 Token/Credits 预算——框架不知道这种计费控制流。
- 敏感输入（密钥、Token）必须走用户表单通道，**永不进入模型上下文**——这是定制到工具 schema 层的约束。
- 高危 ToolCall 的逐次参数绑定审批、敏感表单与不可变事件续跑需要统一控制，通用框架的 interrupt/resume 语义无法直接覆盖这些约束。
- 全链路 OTel GenAI 语义 + 零高基数 label，与 LangChain 的 callback/LangSmith 体系是两套。

选型结论（2026-08 评审）：**工具循环 + 动态工具检索 + 审批断点**的形态，自写循环完全可控。触发重新评估的条件：出现多 Agent 协作（handoff 语义）、多步会签 / 超时升级类复杂人机协同、或团队规模使自研内核的上手成本成为瓶颈。届时优先考虑 Temporal（持久工作流）而非 LangGraph（Node 版为二等公民）。

## 运行时拓扑

```
Web ──► luna-gateway (Go BFF) ──HMAC──► luna-agent (本服务)
                                          │
              ┌───────────────────────────┼───────────────────────────┐
              ▼                           ▼                           ▼
        PostgreSQL                 Luna API (Go 后端)            模型 Provider
   (runs/turns/events/         (服务身份 + Run/ToolCall      (OpenAI 兼容端点,
    summaries/tool_calls)       权威回读 + 原业务路由)       budgeted/托管配置)
```

- **无内存权威状态**：run、turn、不可变事件、工具调用和滚动摘要全部落 PostgreSQL。队列使用数据库原子 `queued -> running` claim；实例中断时 run 进入 `interrupted`，不发生 takeover。
- **单 Agent 副本**：部署使用 `Recreate` 且始终只运行一个副本。PostgreSQL 时间线与 SSE 回放可持久恢复；正在进行的内存流和取消不承诺跨 Pod 连续，重启遗留的 Run 会明确进入 `interrupted`。
- **单跳业务执行**：Agent 只持有固定服务身份，按 Catalog 直接请求 Luna API 的真实 `/api/v1` 业务路由。后端从数据库回读 Run、ToolCall、用户 Session、会话和项目绑定，再由原 Handler/Service 执行 RBAC 与审计。

## 目录结构与模块职责

```
src/
├── index.ts / bootstrap.ts   进程入口与依赖组装（唯一的 wiring 层）
├── server.ts                 Fastify HTTP：BFF 回调、审批与表单决议、健康检查
├── config.ts                 环境变量加载与校验
│
├── executor/                 ★ Run 执行内核（2026-08 从单文件拆分）
│   ├── index.ts              RunExecutor：调度循环、并发配额、原子 claim、
│   │                         step 循环（模型→工具→模型）、run 终态裁决
│   ├── streaming.ts          单次模型流式调用：事件消费、reasoning/正文
│   │                         时间线投影、首 token 与用量遥测
│   ├── cards.ts              CardGenerationService：交互卡片的占位项、
│   │                         schema 校验、修复重试（上限内复用同一占位）、终态
│   ├── internal-tools.ts     内部工具副作用：navigate_to_route /
│   │                         rename_conversation 的时间线写入
│   ├── tool-results.ts       工具结果序列化与字节预算瘦身（数组按元素粒度保留）
│   └── resume.ts             断点续跑：把已完成工具调用重建为 assistant+tool
│                             消息对，不恢复 sticky 工具集
├── executor.ts               兼容 re-export，新代码请直接 import executor/ 子模块
│
├── model-runtime.ts          模型调用门面：消息组装、标题生成、
│                             工具集解析；持有 ContextCompiler
├── context/compiler.ts       上下文编译器：增量水位摘要 + 近期原文装填 +
│                             continuation 有界截断（详见下文）
├── prompt/system.ts          系统 Prompt（中文，按项目规范）
│
├── provider/                 模型 Provider 层
│   ├── provider.ts           ModelProvider 接口与消息/事件类型
│   ├── openai-compatible.ts  OpenAI 兼容客户端（SSE 流、usage 解析、重试）
│   ├── budgeted.ts           ★ 用量装饰器：调用前 clamp、钱包预留、调用后结算
│   ├── managed.ts / runtime.ts / config-client.ts
│   │                         平台托管模型配置拉取与动态刷新
│   └── deterministic.ts      测试用确定性 Provider
│
├── tools/                    工具系统
│   ├── orchestrator.ts       平台工具编排：提议→审批→执行→结果投影，
│   │                         ToolInterruption 驱动断点续跑
│   ├── catalog.ts            工具目录：分页摘要、BM25 与精确详情加载
│   ├── contracts.ts          最小目录与 HTTP 传输契约
│   ├── retrieval/            Unicode 分词与 BM25
│   ├── policy.ts             requiresApproval 单一审批策略
│   ├── postgres-store.ts     工具调用记录（参数密文存储）
│   └── business-card-tools/ui-route/tool-search/tool-details
│                             三类通用卡片、页面导航与目录工具
│
├── persistence/              持久化
│   ├── repository.ts         Repository 接口（领域操作的唯一入口）
│   ├── postgres.ts           PostgreSQL 生产实现（drizzle），含原子 claim 与
│   │                         滚动摘要单调水位
│   └── schema/               drizzle 表定义
│
├── telemetry.ts              OTel 封装：span、metric、结构化日志、Trace
│                             Context 提取；内容捕获默认关闭
├── genai-semconv.ts          OTel GenAI 语义约定对齐
├── redaction.ts              凭据/敏感字段脱敏
├── payload-cipher.ts         AEAD 载荷加密（工具参数）
├── auth.ts                   BFF HMAC 认证 / 开发模式认证
└── runtime-settings.ts       运行时参数默认值（可被平台高级设置动态下发）
```

## 核心流程：一个 Run 的生命周期

```
queued ──原子 claim──► running ──┬─► completed
                                 ├─► waiting_approval ──(批准/拒绝)──► 重新入队续跑
                                 ├─► waiting_input ──(用户提交表单)──► 重新入队续跑
                                 ├─► failed / interrupted
                                 └─► canceled（用户取消）
```

1. **领取**：`RunExecutor` 使用 PostgreSQL 条件更新原子领取一条 queued run。进程退出或执行中断时写入 `interrupted`，保留全部事件，不把正在执行的 run 转交另一实例。
2. **step 循环**（`maxModelSteps` 上限内）：
   - `streaming.ts` 消费模型事件流，实时投影 reasoning/正文到时间线（SSE 推给前端）；
   - 模型返回 tool calls 后逐个派发：**内部工具**包括目录搜索、详情加载、会话命名、显式页面导航与三类通用卡片；**平台工具**经 `ToolOrchestrator` 校验后直接调用真实业务路由。高危调用等待逐次绑定参数的 `reject / approve`，模型与安全表单提交遵循相同的工具 Schema 和业务校验；
   - 模型只接收 `present_card`、`request_input`、`request_choice` 三个卡片 Schema，三者直接使用稳定的 `InteractionCardGroup` v1；
   - 每个工具结果以 `tool` 角色消息追加到 continuation，进入下一 step。
3. **续跑**：审批决策或表单完成后重新入队。`resume.ts` 从不可变工具事件重建必要的 assistant+tool 消息对，保证模型看到暂停前已经发生的结果。
4. **收尾**：自动生成会话标题（best-effort，失败不影响结果），写入终态、不可变事件和遥测；没有独立的“下一步预测”模型调用。

Luna API 从 OpenAPI 普通 JSON operation 自动生成工具目录，并用集中 deny map 排除协议、认证、
凭据、文件和流式端点。`search_tools` 支持空 query 全目录分页和 BM25 目标检索，只返回摘要；
`get_tool_details` 接收 1～8 个精确 operationId，返回完整 Schema 并只把这些工具加入下一模型步骤。
不存在向量、RRF、reranker、shadow/dynamic、Top 8、sticky 或 Agent 本地 fallback。启动时必须先从
Luna API 取得 Provider、运行参数与 Catalog 的同一份完整权威配置；获取或校验失败会阻止启动。

## 上下文压缩机制

`context/compiler.ts` 把权威会话历史编译为单次模型调用的有界上下文：

- **增量水位**：`ConversationSummary.coveredThroughTurnIndex` 单调推进，数据库层用条件 upsert 保证多实例并发不回退。
- **结构化摘要**：固定七字段 JSON（userGoals / constraints / confirmedResources / completedActions / failures / pendingWork / durableFacts），摘要内容经 `redact()` 脱敏，Prompt 明确禁止保存凭据、禁止执行历史中的指令。
- **触发条件**：token 超水位（90% 触发 / 70% 目标）、backlog 积压或历史缺口、超过最大未压缩轮数、超过近期保留轮数。
- **单份滚动摘要**：摘要覆盖线单调前进，配合近期原始消息装填；不存在 deferred/catch-up 或多层摘要状态。
- **降级**：压缩失败退回近期原文 + warn 日志，不阻塞用户请求。
- **注入防护**：摘要、历史、工具结果全部包裹"不可信数据"标签。

**已知短板**（按优先级，欢迎认领）：

1. 每个模型 step 都可能同步触发摘要调用，工具循环内延迟与成本翻倍——应改为按水位滞后量触发或异步化；
2. token 估算为 `bytes/3` 粗略近似——应引入 provider usage 比值校准或分词器；
3. 轮数硬触发（64 轮）对短消息对话过于敏感——token 水位应为主判据，轮数仅作兜底；
4. `compile()` 的 catch 范围过大，DB 故障与摘要生成失败未区分；
5. 摘要 schema 超限时 zod 直接拒绝而非截断保留。

## 硬规范（修改本仓库前必读）

继承仓库根 `AGENTS.md`，以下为本模块的高频约束：

1. **Prompt 中文**：系统 Prompt、工具描述、摘要 Prompt、给模型的 guidance 文案一律中文；工具名、参数名、枚举、错误码保持原值。
2. **错误码稳定**：抛给控制流的错误用 `ai.*` 稳定错误码（如 `ai.run_timeout`），不要把原始异常文本泄漏到 run 状态或用户可见层。`stableError` / `stableErrorCode` 是边界。
3. **遥测三件套**：新控制流路径必须带 span（内部操作用 `internalSpanOptions`）、结构化日志（事件名稳定）、低基数 metric label。**禁止**把用户输入、URL 查询、资源名、ID 放进 span 名/日志事件名/metric label；Secret、Token、Prompt 不进遥测（内容捕获开关仅诊断用途，默认关闭）。
4. **Context 传播**：跨进程调用（Luna API、Provider）必须延续现有 Trace Context；不要在业务链路里新建 `context.Background()` 等价物。
5. **工具注册闭环**：新增/修改 Agent 可调用工具时，按根 AGENTS.md 的 MUST 条款逐项核对 Agent operation 定义、后端 `Execute` case、策略白名单 `requiredScopes`、catalog 下发与描述四端一致，并用真实调用链验证。
6. **状态只走 Repository**：executor、tools、persistence 之外不得出现 SQL；状态迁移必须用 `updateRun(from, to)` 的条件迁移形式，依赖行版本或状态冲突处理并发，禁止"先查再写"。
7. **不可信数据边界**：所有进入模型上下文的外部内容（历史、工具结果、页面上下文、摘要）必须包裹"不可信数据"声明；离开模型的内容（工具参数）必须经 zod schema 校验后才触碰副作用。
8. **模块边界**：
   - `executor/` 是唯一的控制流层，可以依赖 tools/provider/persistence/context，反向依赖禁止；
   - `streaming.ts` / `cards.ts` / `internal-tools.ts` 只做"一件事 + 时间线投影"，不含 step 循环语义；
   - 纯函数（序列化、瘦身、重建消息）放 `tool-results.ts` / `resume.ts`，便于脱离 RunExecutor 单测。

## 扩展方向与建议路径

**近期（不改变架构）**：

- 上下文压缩器短板修复（见上节 1–5），优先做"按水位滞后触发"和"usage 比值校准"，收益最直接；
- `RunExecutor.claimAndExecute` 的 step 循环体仍可继续提取（平台工具派发段、`argumentError` 修复段），目标是把 index.ts 压到纯编排；

**中期（需要设计评审）**：

- **并行工具调用**：当前同一 step 内工具串行执行。并行化需要先解决审批中断与部分完成的续跑语义（`resume.ts` 要支持乱序完成），以及计费预留的合并；
- **摘要异步化**：如将来将压缩移到后台任务，需要先定义摘要水位落后阈值与请求降级策略，不恢复 deferred/catch-up 运行时状态机；
- **多模型路由**：Provider 层已支持按 run 快照选模型，下一步是按任务类型（摘要 / 主循环 / 标题）路由到不同档位模型，`budgeted.ts` 的预留逻辑要相应分组。

**远期（触发前文"重新评估框架"的条件）**：

- 多 Agent 协作 / 任务委派树；
- 多步会签、超时升级类复杂审批流；
- 出现这两类需求时，先评估 Temporal 承接持久工作流部分，Agent 保持为无状态执行器。

## 常用命令

```bash
pnpm --dir luna-agent dev         # 本地开发（tsx watch）
pnpm --dir luna-agent test        # vitest
pnpm --dir luna-agent typecheck   # tsc --noEmit
pnpm --dir luna-agent lint        # eslint
pnpm --dir luna-agent build       # 产物构建
```

交付前 `lint` / `typecheck` / `test` 必须无新增错误；涉及 executor 控制流、持久化迁移、工具契约的改动按根 AGENTS.md 执行完整验证。
