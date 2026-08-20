# Agent 工具语义检索与工具设计优化方案

本文定义 Luna DevOps Agent 工具目录从“全量下发”迁移到“自动语义检索、按需加载”的可实施
方案，并同步约束工具描述、输入 Schema、工作流关系和交互卡片工具的后续治理。本文属于进行中
方案；完成实施和验收后，应把仍需长期遵守的规则提炼到
[`11-AI助手与Agent规格.md`](11-AI助手与Agent规格.md) 与 `AGENTS.md`，再删除本方案正文。

## 1. 结论

目标架构采用以下组合，而不是用正则、字符串包含或单一向量距离直接决定工具：

```text
结构化工具语义
  -> 多向量召回 + BM25 召回
  -> RRF 排名融合
  -> 工作流邻居扩展
  -> 语义重排
  -> Top 8 完整 Schema 按需注入主模型
```

检索必须在每次主模型调用前由 Agent 自动完成。`search_tools` 只保留为工具结果引出新业务域、
当前候选不足或模型需要二次发现时的扩展入口，不能再要求模型通过一次主动搜索才能获得第一批
平台工具。

工具目录约两百项，首版不引入独立向量数据库。工具向量按 Catalog digest 持久化为派生数据，
运行时加载到 Agent 内存并直接计算余弦相似度；查询向量不持久化。向量模型与重排模型通过统一
Provider 抽象接入，具体模型必须由 Luna DevOps 场景评测选择，不在方案阶段绑定厂商。

在影子评测达到门禁前，模型仍全量接收“已经显式准入”的工具，不提前按 Top 8 裁剪；M1 可以
先移除不适合 Agent 或契约不完整的 operation。不得以设计上更先进为理由把尚未通过检索门禁、
但已经审计准入的业务能力直接切断。

## 2. 当前基线与问题

截至 2026-08-20，平台 OpenAPI 生成 204 个 Agent 可用 operation；Agent 还合并 5 个 fallback
平台工具和 4～5 个内部工具，正常一轮模型请求可见约 213～214 个工具。

当前实现存在以下结构性问题：

- `ToolCatalog.resolve()` 忽略页面上下文、用户目标与 `loadedOperationIds`，无条件返回完整目录。
- 系统 Prompt 声称工具按任务动态加载，实际行为与 Prompt 不一致；`search_tools` 返回的
  `loadedOperationIds` 不改变下一轮工具集合。
- 204 个 OpenAPI operation 中有 183 个完全没有 `x-luna-cli` 元数据，且当前没有 operation 通过
  显式 `agentAllowed: true` 准入；目录采用“默认暴露、少量排除”的反向安全模型。
- 209 个平台工具中只有约 30 个具有定制模型描述、35 个具有专属用途指导；其余工具主要依赖
  operationId 和分类拼接通用文案。
- OpenAPI 的 summary/description 仅作为检索 hint，未形成可校验的适用、禁用、前置和回读契约。
- 成功响应中约 146～152 个属于缺少 JSON Schema 或使用 `BusinessObject`、`AIObject` 等通用对象
  的情况，具体数量随“通用响应”的统计口径变化；当前模型往往不知道应读取哪个字段、哪个
  状态才表示完成。实施时必须由脚本按固定口径生成覆盖报告，不能长期维护手工数字。
- 参数校验器虽然接收了 `pattern`、`minLength`、`minItems`、`oneOf` 等 OpenAPI 约束，却没有完整
  执行这些约束；执行编排又把所有校验失败转换成 `waiting_input`。字段已存在但值非法时，模型
  既得不到字段级问题，也可能看到空的“缺失字段”列表。
- `ai.quota.run_max_tool_calls` 目前只存在于平台配置定义和校验中，没有传到 Agent 或执行计数器；
  现有测试反而断言单 Run 不设工具调用上限，配置、行为和测试三者相互矛盾。
- 非 GET operation 自动生成的 `<operationId>_accepted` 不是可执行回读 operation；默认 verifier
  只检查 HTTP 2xx，导致“请求被接受”“异步任务完成”“最终状态验收”被错误合并。
- 工具风险、幂等和 verifier 主要由 HTTP Method 与 operationId 猜测。`preview*`、`test*`、
  `check*` 等无副作用诊断 POST 因而被误判为普通写操作，日志工具之间也缺少结果范围边界。
- 运行时资源工具族复用了含义不同的 `kind`：列表接口实际接收资源分类，YAML、事件和删除接口
  接收 Kubernetes 对象种类；列表参数没有 enum，读取接口的 Scope 与 Handler 管理权限也不一致。
- `execReleaseRuntimeCommand` 与较新的运行命令会话链路能力重叠，`createVolumeImport` 又要求 Agent
  无法完成的本地文件选择和上传，但二者仍可能进入 Agent 可执行目录。
- `createDeploymentTarget`、`updateDeploymentTarget` 等工具直接向模型暴露大型 REST DTO；模型要
  在一个调用中理解互不相关的配置域。
- `create_interaction_cards` 同时暴露业务模板和完整 UI DSL。其模型 Schema 约 47 KiB，包含
  数百个属性节点和大量联合分支，首轮生成与失败修复成本过高。
- 现有检索依赖字符串命中、手写类别正则和少量 guidance。它可以作为简单基线，不能作为生产
  动态加载的唯一依据。

因此，本事项不是“给现有 search() 再加几个关键词”，而是同时治理检索资料、召回、重排、
工具边界、执行循环和上线评测。一次诊断样本中出现 `listApplications` 重复 6 次、
`listReleases` 重复 3 次、`listRuntimeClusterResources` 在同一确定性参数错误上连续失败 4 次；
该样本只作为补充用例，不代表线上总体分布，后续必须通过低基数循环原因和离线回放验证。

## 3. 目标与非目标

### 3.1 目标

- 用户使用口语、同义表达、中英文混合、Kubernetes 错误码或日志术语时，正确的下一步工具能
  稳定进入前 8 个候选。
- 主模型只接收当前阶段所需工具的完整 Schema，降低上下文占用和相似工具竞争。
- 查询、创建、更新、删除、执行和验收工具具备可区分的正向与负向边界。
- 多步骤 DevOps 目标能根据已完成事实逐阶段加载下一步工具，不一次塞入整条可能路径。
- 检索失败安全降级，不授予权限、不绕过批准/MFA，也不把“检索到工具”解释为执行成功。
- 检索质量可以离线评测、影子观测、灰度比较和稳定回归。
- 卡片工具由“模型生成 UI DSL”收敛为“模型声明业务意图，平台编译受控卡片”。

### 3.2 非目标

- 不用检索结果替代 Luna API 的实时鉴权、Scope、RBAC、批准、MFA、幂等和审计。
- 不让向量模型读取或推断 Secret，不把原始日志、Prompt、工具参数或资源 ID 建入工具索引。
- 不引入自主多 Agent 讨论来投票选择工具。
- 不在首版引入 Milvus、Elasticsearch 或独立检索服务；当前目录规模不需要分布式向量检索。
- 不用向量相似度直接执行写操作；最终选择仍由主模型和平台策略共同决定。
- 不为了减少 Schema 而把一个用户目标拆成需要模型维护跨调用 UI 草稿的原子组件工具。

### 3.3 术语

- **Embedding / 向量化**：把一段文本转换为固定长度数字数组，使含义接近的文本在向量空间中
  距离更近。它负责按语义找候选，不直接决定最终工具。
- **BM25**：根据词在当前文档和整个目录中的出现情况计算相关度，适合 operationId、字段名、
  Kubernetes 状态和稳定错误码等精确技术词。
- **RRF**：只根据多个召回器给出的名次合并候选，避免直接相加不可比较的向量分数和 BM25
  分数。
- **Reranker / 重排器**：对少量候选做第二轮精读，结合适用、禁用、前置和当前工作流阶段重新
  排序。
- **Sticky tool**：当前 Run 已调用、正在审批、恢复后需要回灌或必须用于权威验收的工具；重新
  检索时不能把它移除。
- **Agent façade**：面向 Agent 的窄业务操作。它在后端复用现有 Service/API 能力，但不把大型
  REST DTO 原样交给模型。

## 4. 工具语义成为权威契约

### 4.1 元数据来源

有独立 OpenAPI operation 的工具，在 operation 的 `x-luna-agent` 中维护 Agent 契约；手写工具在
对应 TypeScript 定义旁维护相同结构。`x-luna-cli` 继续描述 CLI 行为，但不再兼任 Agent 准入、
风险和验收的权威来源。项目尚未发版，迁移完成后不保留两套字段的长期兼容分支。

构建 Catalog 时归一化为：

```ts
type AgentToolContract = {
  allowed: boolean
  resourceTypes: string[]
  action: "discover" | "read" | "create" | "update" | "delete" | "execute" | "verify"
  sideEffect: "none" | "external-read" | "external-write" | "platform-write" | "destructive"
  idempotent: boolean
  replaySafe: boolean
  risk: "low" | "medium" | "high" | "critical"
  approval: "never" | "always"
  mfaPurpose?: string
  intents: string[]
  useWhen: string[]
  avoidWhen: string[]
  prerequisites: string[]
  parameterSummary: string[]
  successEvidence: string[]
  commonErrorCodes?: string[]
  predecessors?: string[]
  followups?: string[]
  verification:
    | { mode: "response"; successCodes: number[] }
    | {
        mode: "readback" | "async-readback"
        operationId: string
        idSource: string
        argumentBindings: Record<string, string>
        completion:
          | { mode: "readback-success" }
          | {
              mode: "state"
              path: string
              pendingStates: string[]
              successStates: string[]
              failureStates: string[]
            }
      }
}
```

字段要求：

- `allowed` 必须显式为 `true` 才进入 Agent Catalog；缺失和 `false` 都不暴露。平台 Web/API 是否
  可用不受影响。
- `sideEffect`、`idempotent`、`replaySafe`、`risk`、`approval` 和 `mfaPurpose` 必须显式声明，
  不能继续只由 HTTP Method 或 operationId 前缀猜测。`idempotent` 表示重复请求不会改变最终平台
  状态，`replaySafe` 还要求不会重复发送通知、重复计费或触发不可忽略的外部动作；二者不可混用。
  后端实时鉴权、批准和 MFA 仍是最终权威。
- `intents` 使用用户可能表达的业务目标，不复制 operationId。
- `useWhen` 描述当前工作流阶段和必要事实。
- `avoidWhen` 明确与相似读写工具的边界，不能只写泛化安全提示。
- `prerequisites` 只写调用前必须具备的资源和事实，不把权限检查复制到描述中。
- `parameterSummary` 解释主要业务参数，不复制完整 JSON Schema。
- `successEvidence` 指向权威回读或可验证终态；“接口返回 2xx”不是复杂业务的唯一完成证据。
- `predecessors/followups` 表达常见工作流关系，不代表必须无条件执行。
- `verification.mode=response` 只适用于响应本身就是最终事实的读取、预览、校验或连接测试；任何
  异步或有资源副作用的写操作必须声明真实回读 operation、ID 提取和终态集合。
- `idSource`、`argumentBindings` 和 `completion.path` 使用受限 JSON Pointer 映射，不允许在元数据
  中嵌入 JavaScript、模板表达式或任意 JSONPath 脚本。

Catalog 构建门禁必须拒绝以下情况：

- `allowed` 缺失或不是显式布尔值；只有 `allowed: true` 的工具进入目录。
- 缺少 `resourceTypes`、`action`、`intents`、`useWhen` 或 `successEvidence`。
- 写、敏感或破坏性工具缺少 `avoidWhen` 与 `prerequisites`。
- 引用不存在的 predecessor、followup 或 verifier operationId。
- 写操作使用虚构的 `<operationId>_accepted` verifier，或把 2xx 当作异步任务成功终态。
- POST 未显式声明副作用、幂等、可重放性和验收方式，或把外部发送/可能计费的探测错误标记成
  `replaySafe: true`。
- 工具语义仍是“调用某平台接口”一类无法区分的通用文案。
- 描述或检索文档包含 URL 参数值、用户输入、Secret 或环境相关资源 ID。

### 4.2 检索文档

每个工具生成三份正向检索文档，而不是把所有字段拼成一个长字符串：

| 向量种类 | 内容 | 主要解决的问题 |
| --- | --- | --- |
| `intent` | 资源、动作、intents、useWhen | 口语、同义表达、中英文目标 |
| `parameters` | 参数摘要、常见错误码、输入概念 | 字段名、错误码、DevOps 技术术语 |
| `workflow` | 前置条件、前后工具、成功证据 | 当前阶段和后续验收 |

`avoidWhen` 不进入正向向量，避免“不要用于修改”反而提高“修改”查询的相似度。它只进入语义
重排输入和负例评测。

检索文档不嵌入完整 inputSchema、HTTP path、权限令牌或冗长通用安全文案。operationId、参数名和
稳定错误码保留在 BM25 索引中，以支持精确技术查询。

## 5. 索引与持久化

### 5.1 Embedding Provider

Agent 新增独立 `EmbeddingProvider` 接口，支持批量工具文档和单条查询：

```ts
interface EmbeddingProvider {
  identity(): { provider: string; model: string; dimensions: number }
  embedDocuments(input: string[], signal?: AbortSignal): Promise<number[][]>
  embedQuery(input: string, signal?: AbortSignal): Promise<number[]>
}
```

约束：

- 具体模型必须支持中英文、DevOps 术语和短查询，并通过同一评测集对比后选择。
- 不默认假设所有 OpenAI-compatible Chat Provider 都支持 embeddings；能力探测失败要返回稳定
  状态，不能在启动路径无限重试。
- 外部 Embedding Provider 与主模型使用同等级的网络、超时、取消、Trace 和敏感内容边界。
- Query 只包含经过安全裁剪的任务检索上下文；不得发送 Secret、完整日志、工具原始结果或系统
  Prompt。
- embedding 与 rerank 调用如果产生外部模型费用，必须进入现有 Run 预算和发起用户个人钱包
  归属；Catalog 批量建索引属于平台维护成本，不得错误归属项目空间。

### 5.2 持久化格式

工具向量是可再生成的静态派生数据，可持久化在 Agent 使用的 PostgreSQL `ai` schema。首版不
要求 pgvector 扩展；以 `bytea` 保存归一化 Float32 数组，启动时整批加载到内存。

建议表结构：

```text
ai_tool_embeddings
  catalog_digest
  provider_key
  embedding_model
  dimensions
  operation_id
  vector_kind          # intent | parameters | workflow
  document_hash
  vector_bytes
  created_at

PRIMARY KEY (
  catalog_digest,
  provider_key,
  embedding_model,
  operation_id,
  vector_kind
)
```

目录更新流程：

1. 以新 Catalog digest 和文档 hash 查找可复用向量。
2. 批量生成缺失或变化的向量，写入新 digest 分区。
3. 完整校验 operation 数量、维度、有限数值和向量种类。
4. 在内存中构建新索引，完成后原子替换活动索引。
5. 旧索引延迟清理；索引失败继续使用上一个完整索引并报告 degraded，不能半量切换。

用户查询向量只存在于本次检索内存中，不落数据库、普通日志或遥测属性。

## 6. 查询构造

主模型调用前，由 Agent 根据受控字段构造 `ToolRetrievalQuery`：

```ts
type ToolRetrievalQuery = {
  currentGoal: string
  routeName?: string
  resourceContext: string[]
  completedOperations: string[]
  stableOutcomes: string[]
  pendingState?: "user_input" | "approval" | "mfa" | "async_terminal_check"
  stableErrorCodes: string[]
}
```

来源边界：

- `currentGoal` 来自当前用户输入和既有会话目标摘要，设置严格字符预算。
- 页面只提供已注册 routeName 和资源类型；页面资源 ID 不进入检索文本。
- 工具结果只提取 operationId、稳定状态和稳定错误码，不拼接返回正文。
- 审批、MFA、等待输入和异步验收状态由 RunExecutor 权威状态提供。
- 当前轮上传内容、日志、README 和网页正文不直接进入检索查询；诊断目标只保留用户概括和稳定
  错误码。

不得使用正则把用户文本硬分类为部署、网关或日志域。词法分析使用正式中文分词/Unicode
tokenizer，并通过评测选择实现；operationId、字段名和错误码使用确定性标识符分词。

首版词法实现采用 Node.js `Intl.Segmenter("zh-CN", { granularity: "word" })` 处理自然语言；
拉丁标识符按 Unicode 字母/数字类别、大小写边界和 `. _ -` 分隔符确定性切分，稳定错误码同时
保留完整值。这里的切分只服务 BM25 建索引，不包含“出现日志就归入 runtime”等业务正则规则。
分词器封装为 `LexicalTokenizer` 接口，必须用同一评测集验证中文、英文、混合缩写和错误码。

## 7. 候选召回与融合

### 7.1 并行召回

每次自动检索并行执行：

- Query embedding 对 `intent` 向量取 Top 30。
- 对 `parameters` 向量取 Top 30。
- 对 `workflow` 向量取 Top 20。
- BM25 对精简检索文档取 Top 30。
- Run sticky tools 直接进入候选，不参与相似度淘汰。

当前仅约 209 个平台工具，向量侧在内存逐项计算即可。不得为了“用了向量”而先引入分布式
基础设施。

### 7.2 RRF 融合

不同召回器的原始分数不能直接相加。使用 Reciprocal Rank Fusion：

```text
rrfScore(tool) = Σ 1 / (k + rank_i(tool))
```

`k` 与各路 Top K 由离线评测确定，不按单个线上案例手调。RRF 只融合排名，不执行权限或风险
判断。

首版可使用 `k = 60` 作为可复现基线；只有评测报告证明 Recall@8、MRR 或 hard-negative 指标
改善时才允许修改。

### 7.3 工作流邻居

融合后的候选按当前已完成 operation 和 pending state 扩展一层工作流邻居：

- 调用前置事实仍缺失时，优先补入 predecessor。
- 写操作已成功但未验收时，优先补入 verifier/followup 读取工具。
- 正在等待批准、MFA 或表单提交时，不加载新的无关写工具。
- 已调用、正在审批、恢复后必须回灌或当前终态验收所需工具作为 sticky tools 保留到 Run 结束。

图关系用于扩大候选，不直接强制执行工具链。最终仍由语义重排和主模型结合事实决定下一步。

## 8. 语义重排

RRF 与工作流扩展后的前 20 个候选交给 `ToolReranker`。重排器只看到：

- 安全裁剪后的检索查询；
- 候选工具的精简语义；
- `useWhen`、`avoidWhen`、前置、动作类型和成功证据；
- 已完成 operation 与 pending state。

不提供完整工具 Schema、用户 Secret、原始工具结果或系统 Prompt。输出使用严格结构：

```ts
type ToolRerankResult = {
  matches: Array<{
    operationId: string
    relevance: number
    reasonCode:
      | "goal_match"
      | "required_predecessor"
      | "required_verifier"
      | "sticky_operation"
      | "ambiguous_candidate"
    missingPrerequisites: string[]
  }>
}
```

自由文本理由仅用于离线调试并默认关闭；线上普通遥测只保留有限枚举。重排器不得返回候选之外
的 operationId。

为控制延迟和费用：

- 评测确认的高置信读取场景可以跳过外部重排。
- 存在相似读写工具、候选差距小、跨业务域或包含写/敏感/破坏性操作时必须重排。
- 是否跳过不能只依赖拍脑袋余弦阈值；必须根据标注集做置信度校准。
- 重排不可用时使用 RRF 排名、sticky tools 和受控扩容结果，状态标记 degraded。

首版 `ToolReranker` 可复用当前模型 Provider 的低输出、禁用工具调用模式；接口必须保持独立，
便于后续替换为专用 reranker，而不改 ToolCatalog 和 RunExecutor。

## 9. 工具加载与执行循环

### 9.1 初始加载

每次主模型请求只包含：

- 必需内部工具：会话标题、受控交互、导航和二次工具搜索；
- 自动检索 Top 8 平台工具的完整定义；
- 当前 Run sticky tools；
- 本轮必须用于权威回读的 predecessor/verifier。

总量默认不超过 12 个平台工具；超过时先保留 sticky/verifier，再按重排顺序裁剪。敏感或破坏性
工具进入候选不代表获得执行权限。

### 9.2 二次检索

`search_tools` 复用同一检索管线，不维护第二套算法。适用范围：

- 新工具结果返回稳定错误码并引出另一业务域；
- 当前候选确实缺少完成目标的能力；
- 用户在 Run 中改变主要目标；
- 相似候选需要扩大到 Top 16 后再判断。

相同查询与相同工作流状态只执行一次。新结果加入 `loadedOperationIds`，下一次
`ToolCatalog.resolve()` 必须真实保留它们；禁止再次出现“搜索结果声称已加载，resolve 却继续
忽略”的不一致。

### 9.3 降级

- Catalog 向量尚未生成：使用 BM25、工作流图与重排，影子/灰度阶段仍可回退全量目录。
- Query embedding 暂时不可用：不持久化重试队列，使用 BM25 + 工作流图 + 重排。
- Reranker 不可用：使用 RRF Top K，并把检索 outcome 标记为 degraded。
- 所有检索能力不可用：灰度期回退全量目录；稳定期加载有限基础工具集并明确
  `ai.tool_retrieval_unavailable`，不能错误声称平台没有相应能力。
- Catalog digest 与索引不一致：继续使用上一个完整索引或安全降级，不拼接两个版本的工具定义。

### 9.4 权限预过滤

检索前可以根据当前 Run 的服务端签名有效授权上下文排除明确不可用的工具，以减少无意义候选，
但该步骤只做体验优化：

- 平台级 Scope 可以直接做确定性过滤。
- 项目级操作只有在目标 `projectId` 已经由用户输入或可信工具结果确定，且授权快照明确给出该
  项目的 action 时才过滤；未知项目和未知权限不能被误判为无能力。
- 页面路径、前端隐藏状态、模型推断角色和用户口述权限都不是授权依据。
- Tool Delegation、Handler RBAC、批准、MFA 和资源归属检查必须在执行时再次完成；检索命中不
  生成权限，也不能缓存为后续 Run 的授权事实。

### 9.5 Run 循环保护

运行保护采用“语义循环检测优先、较高硬上限兜底”，不能把较低工具调用次数当成正常完成条件：

1. 平台把可配置的 `runMaxToolCalls` 通过内部 Provider 配置快照传到 Agent，Run 启动时固定；Agent
   对 proposed/executed 的计数口径分别记录并在恢复后延续。
2. 当前未接通的默认值 20 不直接沿用。DevOps 日志诊断和异步轮询需要较多调用，实施前必须用
   真实 Run 分位数确定默认值与范围；建议首版硬上限默认 256、可配置 32～2048，并继续受
   `maxModelSteps`、Run Token 预算、超时和取消共同约束。
3. 确定性失败指纹使用 `operationId + canonicalArgumentsHash + stableErrorCode`。同一
   `retryable:false` 失败不得原样重放；`retryable:true` 必须遵守 `Retry-After` 或有界退避。
4. 成功结果指纹只使用脱敏后的稳定结果摘要。相同工具和参数连续得到相同 2xx 事实且没有新增
   信息时，返回 `ai.tool_no_new_information`，提示改换证据或结束，不继续读取。
5. 异步状态轮询是例外，但必须使用显式 `async-readback` 契约、状态或 revision 变化与有界退避；
   相同 pending 状态不能被“无新信息”误判为成功，也不能毫无间隔地刷接口。
6. 达到硬上限返回结构化 `ai.run_tool_call_budget_exceeded`，保留已有证据并说明未完成原因；不能
   声称任务完成，也不能把该状态解释为只能新开会话。

如果最终产品决定不提供硬上限，则必须删除平台中的死配置、校验和 UI，而不是继续保留无效
开关。本方案推荐接通高位兜底上限，是为了终止失控循环，不是限制正常的长日志诊断。

## 10. 工具设计优化规则

语义检索不能弥补错误的工具边界。工具进入目录前同时执行以下静态审计。

### 10.1 参数复杂度

默认门禁：

- 顶层参数不超过 8 个。
- 递归业务属性不超过 20 个。
- 联合类型不超过 5 个可区分分支。
- 模型可见 Schema 不超过 8 KiB。
- 单个工具只表达一个业务动作和一个主要资源边界。

超过门禁不直接禁止平台 API，而是要求以下二选一：

1. 提供窄化的 Agent façade operation，在后端 Service 中组装既有业务请求；或
2. 记录经过评审的例外，证明参数不可分割，并补充专项检索与参数生成评测。

优先治理：

- `createDeploymentTarget` / `updateDeploymentTarget`：按运行基础、镜像/构建、网络端口、资源与
  扩缩容、存储与调度等用户任务拆分 façade，不改变底层权威 DeploymentTarget 模型。
- `createRuntimeCluster` / `updateRuntimeCluster`：拆开连接身份、运行能力和可选高级配置。
- `createGatewayRoute` / `updateGatewayRoute`：区分入口目标、后端绑定、TLS 和高级路由策略。
- `triggerBuildRun`：从可发现的仓库/凭据事实中自动填充，模型只提供本次构建真正变化的参数。
- 参数较多的列表工具：提供统一分页对象和合理默认值，模型只填写有业务意义的筛选条件。

### 10.2 描述质量

工具描述不以长度取胜。模型可见说明按固定顺序生成：

```text
用途 -> 当前适用阶段 -> 不适用边界 -> 前置事实 -> 主要参数 -> 成功回读
```

通用权限、安全和“不要编造 ID”等全局不变量保留在系统 Prompt/Skill，不在 200 个工具描述中
重复消耗上下文。Secret 等工具专属安全边界仍必须留在具体工具说明中。

### 10.3 相似工具对

每组相似工具建立 hard-negative 评测：

- list/get/create/update/delete 同资源工具；
- 业务写操作与 preview/verify 操作；
- 日志、事件、实时状态和历史记录；
- 创建 Release、观察 Release 和读取运行日志；
- 创建、更新、预检和删除项目数据卷；
- 网关路由、证书和 DNS；
- 普通配置与 Secret 安全变更。

新增工具不能只验证“自己能被搜到”，还必须验证相似但错误的工具不会排在它之前。

### 10.4 参数校验与自我修复

Agent 不再维护不完整的手写 JSON Schema 子集校验器。统一使用支持当前 OpenAPI 版本的完整 JSON
Schema validator（TypeScript 侧建议使用 Ajv 2020-12），在 Catalog 构建期编译 Schema，在调用
期返回结构化问题：

```json
{
  "code": "ai.tool_arguments_invalid",
  "retryable": true,
  "issues": [
    {
      "path": "/resourceCategory",
      "code": "enum",
      "allowedValues": ["namespaces", "workloads", "services", "configs", "storage"],
      "remediation": "使用资源分类值，不要填写 Kubernetes Kind"
    }
  ]
}
```

规则如下：

- 至少完整执行 `type`、`required`、`enum`、`const`、`pattern`、`format`、`minimum/maximum`、
  `minLength/maxLength`、`minItems/maxItems`、`uniqueItems`、`additionalProperties`、`oneOf/anyOf/allOf`
  和嵌套数组/对象约束。
- `issues.path` 使用 JSON Pointer；错误 code、allowedValues 和 remediation 来自稳定 Schema/错误
  映射，不回显 Secret、完整参数对象或后端异常。
- 只有真正缺少必填值、该值不能从可信上下文推导且需要用户决策时，才进入 `waiting_input`。
- 字段存在但类型、枚举、范围或组合非法时，把结构化错误回灌模型并允许有界自修复；确定性
  不可重试业务错误直接停止。
- 同一参数错误指纹第二次出现时中止原样重试。需要用户选择时生成 schema 驱动表单，不让模型
  猜测枚举或资源 ID。

### 10.5 写操作权威验收

删除自动生成的 `<operationId>_accepted` 和默认“任何 2xx 都成功”的 verifier。每个 Agent 写操作
必须在 `x-luna-agent.verification` 中形成显式映射：

```text
写 operation -> 响应 ID JSON Pointer -> 回读 operation 参数绑定
              -> pending 状态 -> success 状态 -> failure/cancelled 状态
```

例如 `createRelease` 应从响应提取 `release.id`，调用真实 `getRelease`，把 `pending/running` 视为
等待，把 `succeeded` 视为成功，把 `failed/cancelled` 视为失败终态。若写接口返回的 ID 无法绑定
到回读工具，Catalog 构建直接失败，不允许退回“已接受”占位符。

`previewBuildTemplate`、`testRuntimeCluster`、`testArtifactRegistry`、`testNotificationChannel`、
`checkServiceBinding`、`testAIProviderConnection` 等响应本身即为最终诊断事实的操作，可以使用
`verification.mode=response`，但副作用与重放策略必须逐项审计，不能整组套用同一模板：

```yaml
x-luna-agent:
  allowed: true
  sideEffect: none
  idempotent: true
  replaySafe: true
  risk: low
  verification:
    mode: response
    successCodes: [200]
```

上例只适合纯本地预览。运行集群、镜像站和 Service Binding 探测通常属于 `external-read`；
`testNotificationChannel` 会向外部渠道发送测试消息，应为 `external-write` 且 `replaySafe: false`；
AI Provider 测试可能产生外部请求和费用，也不能默认安全重放。只有明确 `replaySafe: true` 且后端
实现满足约束的 POST，才能按读取类瞬时故障策略自动重试。

### 10.6 运行时资源工具族

消除同名 `kind` 的双重语义，不保留旧字段兼容：

| operation | 新参数 | 语义与约束 |
| --- | --- | --- |
| `listRuntimeClusterResources` | `resourceCategory` | enum：`namespaces/workloads/services/configs/storage` |
| YAML、事件、删除单个资源 | `resourceKind` | Kubernetes 对象种类 enum；与后端 provider 支持集一致 |

列表收到对象种类时返回：

```json
{
  "code": "cluster.resource_category_invalid",
  "retryable": false,
  "allowedValues": ["namespaces", "workloads", "services", "configs", "storage"]
}
```

该修订必须同步 OpenAPI、生成类型/API Client、Web、Agent Catalog、Handler、Provider 和测试。GET
列表、YAML、事件等读取入口应使用与 `cluster:read` 一致的查看权限；修改和删除继续要求管理权限。
权限修复不能只改 Agent `requiredScopes`，必须让 Access Token Scope、Handler RBAC 和资源归属
检查一致。

### 10.7 工具准入、风险与结果契约专项

- `execReleaseRuntimeCommand`：旧直接执行入口与 session 创建/执行/关闭链路重叠，设为
  `allowed: false`；只保留能力更完整、批准/MFA/审计和生命周期更明确的新会话链路。
- `createVolumeImport`：Agent 无法读取和上传用户本地文件，设为 `allowed: false`；模型只提供 Web/
  CLI 跳转或操作说明，不把“能解释”误写成“能执行”。
- Secret、认证、AI Provider、Registry Credential 等敏感配置逐项声明 `risk`、`approval`、
  `mfaPurpose`、敏感路径和真实回读；任何一项缺失都不准入 Agent。
- `getReleaseLogs`、`getReleaseRuntimeLogs`、`getBuildJobLogs`、`getProjectHookRunLog` 分别补齐适用
  场景、禁用场景、必填标识符、时间/分页范围和结果覆盖边界，并加入互为 hard-negative 的评测。
- Catalog DTO 增加归一化 `outputSchema` 或明确的安全结果投影。Agent 可用操作不得只返回没有字段
  语义的通用 `BusinessObject/AIObject`；暂时无法细化的 operation 保持 `allowed: false`，或提交带
  专项测试和责任人的限时例外。
- 输出结果只向模型投影完成当前目标所需的稳定字段，日志正文等大结果继续走现有上下文裁剪；
  输出 Schema 用于解释结果，不能被当作权限或完成状态的替代品。

## 11. 交互卡片工具优化

### 11.1 目标边界

保留 `InteractionCardGroup`、Timeline、SSE、Web 渲染和 Direct Tool Action 作为稳定运行协议；
改变的是模型可见工具，不另造前端卡片协议。

禁止拆成 `add_card`、`add_field`、`add_action`、`finish_card` 等跨调用 UI 原子工具。此类设计会让
模型承担草稿状态、顺序、幂等和部分失败恢复，可靠性低于当前单次原子创建。

模型工具按业务意图收敛为：

| 目标工具 | 替代能力 | 平台负责的内容 |
| --- | --- | --- |
| `request_resource_choice` | `candidate_picker`、`candidate_select` | 按候选数量选择卡片/下拉框，生成 ID 与提交动作 |
| `request_tool_input` | `resource_configuration` | 根据 operation Schema 生成字段、校验、Secret 控件和绑定 |
| `review_tool_action` | `change_review` | 根据真实 operation 与参数生成变更核对，不伪造批准 |
| `present_diagnosis` | `diagnosis_report` | 固定结论、发现和证据拓扑 |
| `present_health_overview` | `health_overview` | 固定指标与分项状态拓扑 |
| 自动进度投影 | `execution_progress` | 根据受支持的权威异步任务引用创建、刷新进度 |
| 自动结果回执 | `operation_result` | 根据工具结果与 verifier 回读生成终态回执 |
| 按需结构化展示 | 通用表格、代码、Diff、关系和图表 | 仅在业务模板无法表达时通过检索加载 |

### 11.2 分阶段迁移

第一阶段复用现有业务模板编译器，把 8 个模板拆为窄模型工具，并把完整通用卡片 DSL 从常驻
工具中移除。完整 DSL 只保留为按需结构化展示的内部实现或检索候选。前端无需改协议。

第二阶段实现 `request_tool_input`：模型只提供真实 `operationId`、已知非敏感参数和需要用户
补充的参数路径；Agent 从权威 Tool Catalog 生成字段类型、范围、枚举、必填、Secret 和 JSON
Pointer 绑定。请求路径不存在、只读或不允许用户填写时 fail closed。

第三阶段由 RunExecutor 根据权威异步任务结果自动投影进度，并在 verifier 达到终态后生成结果
回执。模型不再通过展示卡片自行声称“正在运行”或“执行成功”。

第四阶段移除模型可见的 `create_interaction_cards` 大型联合 Schema，保留通用编译/渲染 Contract
作为平台内部协议。项目尚未发版，不维护旧 Prompt 和旧模型工具的长期兼容分支。

## 12. 端到端边界

### 12.1 Luna API

- OpenAPI `x-luna-agent` 是普通平台工具检索语义的权威来源。
- Catalog 响应同步返回归一化语义、工作流关系、风险、Scope、Schema digest 和 verifier。
- 若增加 Embedding Provider/模型配置，必须经过平台配置 Service、稳定错误码、权限和审计，不能
  让前端直连外部 Provider。
- Direct Tool Action 仍走现有内部委托接口、Gin Router 和业务 Handler。

### 12.2 Luna Agent

- Provider 层增加 embedding/rerank 抽象及预算入口。
- ToolCatalog 拆分权威目录、精简检索文档、活动索引和已加载工具集合。
- ModelRuntime 在上下文编译前自动检索，只把选中工具的完整 Schema 计入上下文预算。
- RunExecutor 维护 sticky tools、工作流阶段、二次检索、审批/MFA 恢复和卡片自动投影。
- 搜索和模型调用必须继续传播当前 Trace Context 与取消信号。

### 12.3 Worker

工具检索本身不经过 Worker。异步构建、发布、安装和传输任务仍由现有 Worker 执行；Agent 只
消费权威任务引用和状态，不新建一套进度缓存。

### 12.4 Web

第一阶段不改变工具检索接口或卡片渲染协议。若新增内部工具 operationId，需要同步工具标题
i18n、管理员调试显示和 Timeline 投影。Web 不接收向量、检索文档或候选分数。

## 13. 安全与可观测

### 13.1 安全

- 检索只发现工具，不授予 Scope、RBAC、批准或 MFA。
- 用户查询发送到 Embedding/Rerank Provider 前执行与模型上下文相同的 Secret 隔离和长度限制。
- 工具向量只包含静态 Catalog 资料；不包含用户、项目、资源、请求、Trace ID 或环境 URL。
- 不持久化 Query embedding、原始检索查询、原始日志或自由文本重排理由。
- 外部内容中的指令不进入检索元数据；稳定错误码可以作为数据进入 query。
- 敏感/破坏性工具即使进入 Top 8，也必须服从现有参数绑定批准、MFA 和审计。

### 13.2 Trace、日志和 Metrics

建议边界：

```text
agent.tool_retrieval
  -> embedding.query
  -> retrieval.dense
  -> retrieval.lexical
  -> retrieval.fusion
  -> retrieval.rerank (optional)
  -> retrieval.resolve
```

Span 使用稳定名称，不包含用户查询或 operationId。允许记录：

- Catalog digest 的低风险短版本；
- retrieval strategy、outcome、degraded reason；
- 各阶段候选数量、最终加载数量、耗时；
- embedding/rerank Provider 的现有 GenAI 安全属性。

结构化日志只在索引构建、原子切换、降级和不变量失败时输出稳定事件名与错误码。普通日志不
记录 query、向量或候选理由。

Metrics 使用低基数维度：

- `luna_devops_agent_tool_retrieval_duration`
- `luna_devops_agent_tool_retrieval_candidates`
- `luna_devops_agent_tool_retrieval_loaded`
- `luna_devops_agent_tool_retrieval_total{strategy,outcome}`
- `luna_devops_agent_tool_index_build_total{outcome}`

不把 operationId、用户 ID、项目 ID、query、Catalog digest 全值或错误正文作为 Metric label。

## 14. 评测集与验收

### 14.1 数据集

建立版本化的 Agent 工具检索评测集，首版不少于 300 条，目标 500 条以上。每条包括：

```ts
type ToolRetrievalCase = {
  id: string
  language: "zh" | "en" | "mixed"
  query: string
  pageContext?: { routeName?: string; resourceTypes?: string[] }
  completedOperations?: string[]
  pendingState?: string
  requiredNextTools: string[]
  acceptableTools?: string[]
  forbiddenTools: string[]
  rationale: string
}
```

覆盖范围：

- 中文口语、英文、混合缩写和同义表达；
- Kubernetes/BuildKit/Gateway/Registry 错误码；
- 日志、事件、实时状态和历史数据边界；
- 项目、应用、构建、发布、运行时、网关、通知、安全、账单、数据卷；
- 相似的 list/get/create/update/delete；
- 多步骤交付当前阶段、写后回读和异步终态；
- 无对应能力、缺权限、缺参数、等待批准/MFA 的负例；
- 字段缺失与字段非法的边界、enum/range/oneOf 修复、同一非法参数禁止原样重试；
- 相同确定性错误、相同 2xx 结果、合法异步 pending 轮询和达到 Run 兜底上限；
- `resourceCategory` 与 `resourceKind`、preview/test 与真实写操作、四类日志工具的 hard negative；
- 写操作 ID 提取、回读参数绑定、pending/success/failure/cancelled 终态；
- 明确无权限、权限未知和页面上下文声称有权限三种预过滤边界；
- Secret、提示注入和恶意日志不得改变工具策略的安全例。

生产查询只能在完成脱敏、权限与内部数据使用审批后转化为离线样本；普通遥测不得直接沉淀
用户原文。

### 14.2 指标门禁

动态裁剪正式启用前必须达到：

- Required Next Tool Recall@8 ≥ 98%。
- 关键写操作与对应读取/更新/删除工具混淆率 < 1%。
- `forbiddenTools` 进入 Top 8 的案例比例 < 0.5%。
- 写后 verifier Recall@8 ≥ 99%。
- 显式 `allowed`、用途指导、风险/副作用/重放、输入与输出契约覆盖率 100%。
- Agent 可用写操作真实 verifier 覆盖率 100%，虚构 `_accepted` verifier 数量为 0。
- 参数字段级错误完整率 100%；可修复非法参数首次修复成功率 ≥ 95%，确定性错误原样重放为 0。
- 合法异步轮询不被误杀；相同确定性失败和相同无新信息结果均在第二次原样调用前被阻止。
- 安全不变量、Secret 和越权工具选择用例 100% 通过。
- 相同 Catalog、查询和状态的排序稳定；并列结果使用 operationId 确定性排序。
- embedding/rerank 降级用例仍能返回明确能力状态，不错误宣称工具不存在。
- 动态工具注入后的端到端成功率不得低于全量目录基线。

Recall@8 是必要条件，不是唯一指标。还需记录 MRR、NDCG@8、错误动作类型、额外工具数量、
检索延迟、模型输入 Token 和最终工具执行成功率。

### 14.3 影子和灰度

1. **离线基线**：比较现有字符串检索、BM25、向量、混合召回和混合+重排。
2. **影子模式**：主模型仍收到全量目录，新检索器只记录有限枚举结果；离线关联最终成功工具，
   检查它是否进入 Top 8。
3. **灰度模式**：小比例 Run 只注入 Top 8；低置信或 degraded 自动扩到 Top 16～24，必要时回退
   全量目录。
4. **扩大流量**：按读取、低风险写入、高风险工作流逐级启用，不一次覆盖全部业务域。
5. **正式切换**：门禁连续稳定后删除全量下发测试，改为断言工具总量上限、sticky/verifier 保留
   和二次搜索真实扩张。

当前工具测试通过不等于新方案已经满足：现有测试中仍有“全量下发 Catalog”和“单 Run 无工具
调用上限”的反向断言。迁移时先把它们改成新契约测试，再以新测试通过作为完成证据；不能保留
旧断言同时依赖运行时特殊分支绕过。

任何阶段出现必需工具召回盲区，先补评测样本和工具语义，再调整算法；禁止针对单条用户文本
增加正则或 operationId 特判。

## 15. 实施里程碑

### M1：Catalog 与执行正确性底座

- 定义 OpenAPI `x-luna-agent` Schema 和手写工具等价类型。
- 准入改为显式 `allowed: true`；先为经过审计的基础工具和代表业务链路开放，未审计操作保持
  不可见，不再默认暴露。
- 生成可重复的准入、描述、指导、输入/输出 Schema、风险、副作用、重放和 verifier 覆盖报告。
- 接入完整 JSON Schema 校验和字段级可修复错误，纠正 `waiting_input` 边界。
- 接通高位 Run 工具调用兜底上限，并实现确定性失败、无新信息和合法异步轮询识别。
- 删除虚构 `_accepted` verifier；先补齐创建 Release 等代表写链路的真实回读契约。
- 修正运行时资源 `resourceCategory/resourceKind`、enum、稳定错误和读取权限一致性。
- 禁用 `execReleaseRuntimeCommand`、`createVolumeImport`，审计 preview/test/log/Secret/Provider 代表工具。
- 增加描述、引用、参数复杂度、输出契约和 hard-negative 静态测试。
- 建立首版不少于 300 条评测集和现有算法基线。

验收：显式准入工具契约覆盖 100%，无悬空回读引用；错误可修复、循环保护、写后验收和运行时
资源代表链路通过契约测试；旧“全量暴露/无限调用”反向测试已经删除或改写。

### M2：混合检索基础设施

- 为全部计划开放给 Agent 的工具补齐语义与输出契约；未完成前继续 `allowed: false`。
- 实现 EmbeddingProvider、工具向量持久化、原子索引切换和 Query 安全裁剪。
- 实现 BM25、多向量召回、RRF、sticky tools 和工作流扩展。
- `ToolCatalog.resolve()` 真正消费自动候选和 `loadedOperationIds`。
- 补齐 Trace、日志、Metrics、超时、取消和降级测试。

验收：索引重建不阻断已有索引；无向量、无 reranker 和 digest 漂移路径可预测。

### M3：语义重排与影子运行

- 实现独立 ToolReranker 与置信度校准。
- 运行全量目录影子评测，不改变用户行为。
- 对照最终成功工具、错误选择和输入 Token，补齐真实 hard negatives。

验收：离线和影子 Recall@8 达到门禁，安全用例 100% 通过。

### M4：动态加载灰度

- 初始自动 Top 8、平台工具总量上限、run sticky 和 verifier 保留生效。
- `search_tools` 复用同一检索器并真实扩张后续工具集合。
- 按读取、写入、高风险域逐级灰度，提供快速配置回退。

验收：端到端成功率不低于全量目录；模型输入 Tool Schema Token 显著下降；审批/MFA 恢复无
工具漂移。

### M5：工具与卡片重构

- 为超限平台工具提供任务型 façade 或正式例外。
- 逐域替换剩余通用 `BusinessObject/AIObject` 输出并补齐风险、可重放和 verifier 契约。
- 按业务意图拆分卡片模型工具，复用现有 Card Contract。
- 实现 schema 驱动表单、权威进度和结果自动投影。
- 删除模型可见大型通用卡片联合 Schema 和旧全量目录路径。

验收：交互卡片首次 Schema 通过率 ≥ 99%，自动修复率 < 1%，修复耗尽为 0；卡片工具参数
Token 平均下降至少 60%。

## 16. 实施时必须同步的注册点

本方案落地属于 Agent 工具行为变更，必须逐层审计：

- OpenAPI `x-luna-agent`、Catalog DTO、内部 Provider 配置与 Schema digest。
- Agent ToolCatalog、Provider、ModelRuntime、RunExecutor、Prompt、Skill 和恢复逻辑。
- 手写内部工具 operationId、内部 dispatch、工具展示/i18n 和 Timeline。
- Direct Tool Action 仍引用真实平台 operationId，requiredScopes 与后端策略完全一致。
- 检索、embedding、rerank 和工具调用处于同一 Trace Context；异步索引任务使用明确父链或独立
  维护 Trace，不使用 `context.Background()` 截断业务请求。
- 单元、Catalog 契约、检索评测、工具执行、批准/MFA 恢复、Secret 和端到端成功/失败路径。

本方案不改变 Worker 权威任务、Kubernetes 实时状态单一事实源、AI 费用个人钱包归属或现有
工具执行安全边界。
