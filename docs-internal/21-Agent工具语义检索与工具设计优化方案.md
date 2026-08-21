# Agent 工具目录与按需加载规范

本文记录 Luna DevOps Agent 当前工具目录、检索和加载的长期边界。历史上的显式
`x-luna-agent.allowed` 准入、向量召回、RRF、reranker、工作流图、sticky Top K 和
`browse_tools` 方案已经退出生产设计，不再作为实现依据。

## 1. 结论

平台工具只有一条事实链：

```text
OpenAPI operation
  -> Luna API 自动生成最小 Agent Catalog
  -> Provider config 下发完整 Catalog
  -> Agent search_tools 返回分页摘要
  -> Agent get_tool_details 精确加载完整 Schema
  -> Luna API 原业务路由重新鉴权并执行
```

目录默认纳入满足结构条件的 `/api/v1` JSON operation。不能交给模型的协议、凭据、流式响应、
文件上传和其他特殊能力统一记录在后端 deny map 中，并为每个 operationId 保留稳定原因。OpenAPI
不再维护独立的 Agent 准入布尔值、风险枚举、工作流关系或回读 JSON Pointer。

检索只使用确定性 Unicode 分词与 BM25。目录规模不需要向量数据库、embedding、RRF、reranker、
影子模式或按 Top K 自动裁剪。模型只有在精确选择 operationId 并调用 `get_tool_details` 后，才会
在下一模型步骤获得该平台工具的完整定义。

## 2. Catalog 契约

每个平台 operation 由后端从 OpenAPI 自动归一化为：

```ts
type ToolOperation = {
  operationId: string
  name: string
  summary: string
  category: string
  tags: string[]
  aliases: { zh: string[], en: string[] }
  requiresApproval: boolean
  idempotent: boolean
  method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE"
  path: string
  requiredScopes: string[]
  inputSchema: JSONSchema
  outputSchema: JSONSchema
  sensitivePaths: string[]
  parameters: Array<{
    inputName: string
    wireName: string
    in: "path" | "query" | "header"
    required: boolean
  }>
  requestBody: boolean
  requestRequired: boolean
  requestType: string
}
```

字段来源遵循以下规则：

- 名称、摘要、标签、参数与成功响应 Schema 来自 OpenAPI operation。
- 中英文别名是最小检索元数据，可由 OpenAPI 稳定描述和统一分类规则生成；不得在 Agent 内再
  维护另一份 operationId 描述表。
- `requiresApproval` 是唯一审批策略字段。DELETE 和明确标记为高危的 operation 会被归一化为
  `true`；普通读取与普通写入为 `false`。
- `requiredScopes`、路径/查询参数、敏感路径和请求体约束从真实 HTTP 契约生成，不能靠 Agent
  手写 fallback 猜测。
- Catalog 只描述“模型能否发现和怎样调用”，不授予权限。最终用户、Session、会话、项目空间、
  RBAC、敏感输入模式和审批状态都由 Luna API 在真实业务路由重新校验。

## 3. 自动纳入与集中排除

满足以下条件的 operation 自动进入目录：

- 路径位于 `/api/v1`；
- 使用 Agent HTTP Executor 支持的标准 JSON 请求/响应；
- 存在稳定 operationId；
- 不属于 OAuth/OIDC 回调、登录注册、凭据读取、文件上传下载、SSE/WebSocket 或其他协议入口；
- 未被集中 deny map 排除。

deny map 使用 `operationId -> reason`，原因必须可测试、可审计。新增普通平台 API 不需要再同步
Agent 白名单；新增特殊协议或不应由模型执行的能力时，必须在同一事项中加入 deny 原因与测试。

## 4. 两阶段发现

### 4.1 `search_tools`

输入为可选 `query`、`page`、`pageSize`。空 query 按 operationId 稳定排序返回全目录分页；非空
query 使用 BM25 匹配 operationId、名称、摘要、分类、标签和中英文别名。

返回只包含：operationId、名称与一句话用途、分类、标签、中英文别名、是否需要审批以及分页字段。
摘要结果不得携带输入/输出 Schema、路由细节、权限范围或敏感字段，也不得自动把命中项加载给
模型。检索词只允许出现在受控高敏内容观测中，不得成为普通 Span 属性或 Metric label。

### 4.2 `get_tool_details`

输入为 1～8 个精确 operationId。返回选中 operation 的完整 Catalog DTO 和不存在的 ID；选中项
在下一模型步骤成为可调用工具。调用详情不执行业务操作，也不绕过审批或权限。

模型声明缺少能力前，必须先用空 query 浏览或用目标语言检索目录；检索到候选后必须再加载详情，
不能把摘要当作执行结果。

## 5. 执行与审批边界

Agent 只根据 Catalog 的 method、path、parameters 和 request body 构造请求，并携带服务身份、
runId、toolCallId 与幂等键。Luna API 从数据库权威回读 Run、会话、用户 Session、项目空间和
ToolCall，不信任 Agent 自报用户或 operationId。

`requiresApproval=false` 的工具可直接进入真实业务路由。`requiresApproval=true` 的工具支持：

- `reject`：只拒绝当前 ToolCall，Run 可继续回答或选择其他操作；
- `approve`：只批准当前 ToolCall；
- `approve_always`：批准当前 ToolCall，并创建账号级 `userId + operationId` 豁免。

豁免可列出、撤销；撤销后下一次同 operationId 高危调用必须重新等待批准。审批和豁免不能改变
原业务 Handler/Service 的 RBAC 结论。

## 6. 交互卡片与上下文

新模型只可见 `present_card`、`request_input`、`request_choice` 三个通用卡片工具，统一编译为
`InteractionCardGroup` v1。旧卡片 operationId 和 payload 只用于历史解析与渲染，不进入新模型
工具集。

上下文由近期原始消息、原始工具交互和一份滚动摘要组成。目录检索状态只在当前 Run 生效；不保存
跨 Run sticky 工具、不执行下一步预测模型，也不因历史空结果阻止新的用户请求。

## 7. 变更与验证要求

修改 OpenAPI operation 时至少验证：

1. 自动目录是否正确纳入或以集中原因排除；
2. 空 query 分页可遍历完整目录；
3. operationId、中文和英文目标都能检索到正确摘要；
4. `get_tool_details` 返回真实输入/输出 Schema 与传输参数；
5. 详情加载后模型可见、未加载工具不可见；
6. 普通、高危、拒绝、单次批准、始终批准和撤销路径；
7. 不同用户、Session、会话和项目空间无法交叉执行；
8. 至少一条真实业务副作用与权威回读闭环；
9. Trace 父子关系、失败状态、低基数指标和敏感字段不泄漏。

ProjectVolume 是目录回归基准：空 query、`ProjectVolume`、`项目数据卷` 和英文 volume 目标都应发现
对应 list/get/create/update/delete/preview/transfer 能力；被 deny 的导入、重试或特殊文件协议必须
返回确定的缺失结果，不能用测试 fixture 伪造生产可见性。
