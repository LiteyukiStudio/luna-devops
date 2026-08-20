# luna-agent

Luna DevOps 的 AI 助手运行时。一个**自研的、以 PostgreSQL 为权威状态的 Agent 执行器**——没有使用 LangGraph / Vercel AI SDK 等编排框架，这是有意为之的技术选型，本文档解释架构、选型理由和扩展规范。

## 为什么不使用 Agent 框架

核心原因：这个 Agent 的计费、安全、可观测要求**穿透控制流的每一步**，框架在这些地方全部是负资产：

- 每次模型调用前要经过五重预算闸（站点上限 / 模型能力 / 上下文剩余 / Run 剩余 / 个人钱包余额），调用后按四类 token 价格（输入 / 输出 / 缓存输入 / 缓存输出）结算到**用户个人钱包**——框架不知道你的计费模型。
- 敏感输入（密钥、Token）必须走用户表单通道，**永不进入模型上下文**——这是定制到工具 schema 层的约束。
- 审批 / MFA step-up 的断点续跑与多实例租约竞争绑定，框架的 interrupt/resume 语义覆盖不了租约丢失、实例漂移的场景。
- 全链路 OTel GenAI 语义 + 零高基数 label，与 LangChain 的 callback/LangSmith 体系是两套。

选型结论（2026-08 评审）：**工具循环 + 动态工具检索 + 审批断点**的形态，自写循环完全可控。触发重新评估的条件：出现多 Agent 协作（handoff 语义）、多步会签 / 超时升级类复杂人机协同、或团队规模使自研内核的上手成本成为瓶颈。届时优先考虑 Temporal（持久工作流）而非 LangGraph（Node 版为二等公民）。

## 运行时拓扑

```
Web ──► luna-gateway (Go BFF) ──HMAC──► luna-agent (本服务)
                                          │
              ┌───────────────────────────┼───────────────────────────┐
              ▼                           ▼                           ▼
        PostgreSQL                 Luna API (Go 后端)            模型 Provider
   (runs/turns/timeline/       (平台工具 HTTP 调用,          (OpenAI 兼容端点,
    summaries/tool_calls,       HMAC + Run Actor Grant)       budgeted/托管配置)
    权威状态 + 租约)
```

- **无内存权威状态**：run、turn、时间线、工具调用、上下文摘要全部落 PostgreSQL。多实例通过 `claimRun` + 租约心跳竞争执行权，实例宕机后 run 可被其他实例接管。
- **平台能力零直连**：Agent 不直接持有任何平台凭据。工具调用经 `ToolOrchestrator` 发给 Go 后端，携带按 run 加密的 Actor Grant（`runActorGrantCiphertext`），权限在服务端按用户身份裁决。

## 目录结构与模块职责

```
src/
├── index.ts / bootstrap.ts   进程入口与依赖组装（唯一的 wiring 层）
├── server.ts                 Fastify HTTP：BFF 回调、审批/MFA 决议、健康检查
├── config.ts                 环境变量加载与校验
│
├── executor/                 ★ Run 执行内核（2026-08 从单文件拆分）
│   ├── index.ts              RunExecutor：调度循环、并发配额、租约心跳、
│   │                         step 循环（模型→工具→模型）、run 终态裁决
│   ├── streaming.ts          单次模型流式调用：事件消费、reasoning/正文
│   │                         时间线投影、首 token 与用量遥测
│   ├── cards.ts              CardGenerationService：交互卡片的占位项、
│   │                         schema 校验、修复重试（上限内复用同一占位）、终态
│   ├── internal-tools.ts     内部工具副作用：create_options / navigate_to_route /
│   │                         rename_conversation 的时间线与 UI Action 写入
│   ├── tool-results.ts       工具结果序列化与字节预算瘦身（数组按元素粒度保留）、
│   │                         平台工具失败引导（如 registry 凭据前置检查）
│   └── resume.ts             断点续跑：把已完成工具调用重建为 assistant+tool
│                             消息对；恢复已加载工具集防止漂移
├── executor.ts               兼容 re-export，新代码请直接 import executor/ 子模块
│
├── model-runtime.ts          模型调用门面：消息组装、标题生成、下一步预测、
│                             工具集解析；持有 ContextCompiler
├── context/compiler.ts       上下文编译器：增量水位摘要 + 近期原文装填 +
│                             continuation 有界截断（详见下文）
├── prompt/system.ts          系统 Prompt（中文，按项目规范）
│
├── provider/                 模型 Provider 层
│   ├── provider.ts           ModelProvider 接口与消息/事件类型
│   ├── openai-compatible.ts  OpenAI 兼容客户端（SSE 流、usage 解析、重试）
│   ├── budgeted.ts           ★ 预算装饰器：调用前 clamp、预留、调用后结算
│   ├── managed.ts / runtime.ts / config-client.ts
│   │                         平台托管模型配置拉取与动态刷新
│   └── deterministic.ts      测试用确定性 Provider
│
├── tools/                    工具系统
│   ├── orchestrator.ts       平台工具编排：提议→审批/MFA→执行→结果投影，
│   │                         ToolInterruption 驱动断点续跑
│   ├── catalog.ts            工具目录：显式准入 + 自动/二次混合检索
│   ├── contracts.ts          Agent 工具语义、风险、工作流与验收契约
│   ├── retrieval/            BM25、多向量召回、RRF 与可插拔重排
│   ├── policy.ts             工具策略（范围、风险级别）
│   ├── generated/platform.ts 由 OpenAPI 生成的平台操作定义（勿手改）
│   ├── postgres-store.ts     工具调用记录（参数密文存储）
│   └── ui-cards/ui-options/ui-route/tool-search/conversation-title
│                             内部工具的 schema 与输入校验
│
├── persistence/              持久化
│   ├── repository.ts         Repository 接口（领域操作的唯一入口）
│   ├── postgres.ts           PostgreSQL 实现（drizzle），含租约、单调水位
│   │                         upsert、行版本乐观锁
│   ├── memory.ts             内存实现（测试/本地）
│   └── schema/               drizzle 表定义
│
├── telemetry.ts              OTel 封装：span、metric、结构化日志、Trace
│                             Context 提取；内容捕获默认关闭
├── genai-semconv.ts          OTel GenAI 语义约定对齐
├── redaction.ts              凭据/敏感字段脱敏
├── payload-cipher.ts         AEAD 载荷加密（run grant、工具参数）
├── auth.ts                   BFF HMAC 认证 / 开发模式认证
└── runtime-settings.ts       运行时参数默认值（可被平台高级设置动态下发）
```

## 核心流程：一个 Run 的生命周期

```
queued ──claimRun──► running ──┬─► completed
                               ├─► waiting_approval ──(用户批准)──► 重新入队续跑
                               ├─► waiting_mfa ──(MFA 通过)──► 重新入队续跑
                               ├─► waiting_input ──(用户提交表单)──► 重新入队续跑
                               ├─► failed（稳定错误码）
                               └─► canceled（用户取消 / 租约丢失 / 超时）
```

1. **领取**：`RunExecutor` 轮询 `claimRun`（实例 ID + 租约秒数），领取后心跳续租；租约续期失败立即 abort——意味着另一个实例已接管。
2. **step 循环**（`maxModelSteps` 上限内）：
   - `streaming.ts` 消费模型事件流，实时投影 reasoning/正文到时间线（SSE 推给前端）；
   - 模型返回 tool calls 后逐个派发：**内部工具**（快捷选项、工具检索、会话命名、页面导航，以及 `request_resource_choice` / `request_tool_input` / `review_tool_action` / `present_*` 业务卡片工具）在进程内处理；**平台工具**经 `ToolOrchestrator.propose` 走审批/MFA/敏感输入检查，需要人工介入时抛 `ToolInterruption`，run 进入 waiting_* 状态并归还租约；
   - 模型只接收按业务意图拆分的窄卡片 Schema。Agent 在单次调用内补齐 `schemaVersion` 和业务模板类型、编译为稳定 `InteractionCardGroup`；Timeline 继续使用 `create_interaction_cards` 运行协议，便于历史卡片恢复和既有 Web 渲染，完整 DSL 不再下发给生产模型；
   - 每个工具结果以 `tool` 角色消息追加到 continuation，进入下一 step。
3. **续跑**：审批/MFA/表单完成后由服务端重新入队 run。`resume.ts` 把暂停前已完成的工具调用重建为 assistant+tool 消息对，并恢复此前已加载的动态工具集——保证续跑时模型看到的上下文与暂停前连续。
4. **收尾**：自动生成会话标题（best-effort，失败不影响结果）、预测下一步操作选项、写终态与遥测。

工具语义检索默认以 `TOOL_RETRIEVAL_MODE=shadow` 运行：模型只看到 `allowed: true` 的已审计目录，
同时后台计算 Top 8 并记录有限枚举遥测。离线评测与影子门禁通过后再切换为 `dynamic`；此时每次
主模型调用只注入 Top 8、当前 Run 的 sticky 工具和契约要求的前置/回读工具，平台工具总数上限
为 12。向量或重排 Provider 不可用时明确降级为 BM25 + 工作流，不会回退未审计工具。
托管模式启动时必须先从 Luna API 取得完整 Catalog 契约；获取失败会阻止 Agent 启动并交由进程
管理器重试，不会以无契约 fallback 假装就绪。`webSearch`、`fetchWebPage` 和 `getAppTemplate`
没有独立 OpenAPI 路由，但仍使用与平台工具相同的显式准入契约和后端 Scope 策略。

## 上下文压缩机制

`context/compiler.ts` 把权威会话历史编译为单次模型调用的有界上下文：

- **增量水位**：`ConversationSummary.coveredThroughTurnIndex` 单调推进，数据库层用条件 upsert 保证多实例并发不回退。
- **结构化摘要**：固定七字段 JSON（userGoals / constraints / confirmedResources / completedActions / failures / pendingWork / durableFacts），摘要内容经 `redact()` 脱敏，Prompt 明确禁止保存凭据、禁止执行历史中的指令。
- **触发条件**：token 超水位（90% 触发 / 70% 目标）、backlog 积压或历史缺口、超过最大未压缩轮数、超过近期保留轮数。
- **诚实追赶**：积压过大时不一次性假装覆盖，用 `deferredHistoryMessage` 明确告知模型"第 X～Y 轮尚未进摘要，不得猜测"。
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
- 平台工具失败引导（`platformToolFailureGuidance`）目前是硬编码单例，若出现第二个场景应改为按 (operationId, errorCode) 注册的表驱动结构。

**中期（需要设计评审）**：

- **并行工具调用**：当前同一 step 内工具串行执行。并行化需要先解决审批/MFA 中断与部分完成的续跑语义（`resume.ts` 要支持乱序完成），以及计费预留的合并；
- **摘要异步化**：压缩从请求路径挪到后台任务，请求路径只读水位 + deferred 提示。需要处理"摘要落后过多时是否阻塞"的策略；
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
