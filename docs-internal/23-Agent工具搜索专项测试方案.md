# Agent 工具目录专项测试方案

本文验收 Luna DevOps Agent 当前的自动 OpenAPI Catalog、`search_tools` 分页摘要与
`get_tool_details` 精确加载。它不再使用 Recall@8、向量模型、RRF、reranker、工作流图或固定
Top K 作为上线门禁。

## 1. 测试目标

- 所有符合结构条件的 `/api/v1` JSON operation 自动进入生产目录；
- 集中 deny map 中的 operation 有稳定排除原因，且不会被搜索或详情接口返回；
- 空 query 可按稳定顺序完整遍历目录，不受模型语义排序影响；
- operationId、中文、英文和混合术语可通过确定性 BM25 命中正确摘要；
- 搜索结果不含 Schema，详情结果包含真实输入/输出 Schema 和传输信息；
- 只有精确加载的工具在下一模型步骤可见；
- 工具发现不授予权限、不触发副作用、不创建审批豁免；
- 真实执行仍经过用户、Session、会话、项目空间、RBAC 和审批检查。

## 2. 目录契约测试

后端单元测试从真实 `openapi/openapi.yaml` 生成目录并断言：

1. 目录非空且 operationId 唯一；
2. 每个 operation 都具备最小 DTO 全部字段；
3. 不存在 `allowed`、risk 枚举、approval mode、工作流 predecessor/followup、verifier 或 readback
   JSON Pointer；
4. 输入 Schema、首个 2xx JSON 成功响应 Schema、路径/查询参数和敏感路径来自同一 OpenAPI；
5. deny map 不存在悬空 operationId，并为每个排除项提供非空原因；
6. 登录、回调、凭据、流式与文件协议不会进入目录；
7. 目录版本摘要会随 OpenAPI、模型列表或最小 Catalog DTO 变化。

## 3. 搜索与详情测试

### 3.1 空 query

- `search_tools({})` 返回第 1 页与默认 pageSize；
- 逐页读取直到 `totalPages`，operationId 无重复、无遗漏、顺序稳定；
- `total === 0` 时 `totalPages === 0`；越界页返回空 items；
- pageSize 只接受 1～100，query 最多 240 个字符；
- items 不含 `inputSchema`、`outputSchema`、`parameters`、`requiredScopes` 或 `sensitivePaths`。

### 3.2 词法检索

每个关键工具族至少覆盖精确 operationId、中文业务词、英文业务词、中英文混合或常见平台术语、
相似读写 operation 的边界，以及不存在能力与 deny operation。排序只要求确定性和正确候选可在
分页结果中发现，不设置“必须前 8”之类与目录规模耦合的门禁。

### 3.3 精确详情

- 一次接受 1～8 个 operationId；
- 按请求顺序去重返回存在项，并单独返回 missingOperationIds；
- 返回完整 Catalog DTO 与 JSON Schema；
- 调用详情前平台工具不进入模型定义，调用后只有存在项进入；
- 重复详情调用不产生业务副作用；
- 模型不能用模糊名称或索引位置代替 operationId。

## 4. ProjectVolume 基准矩阵

| 输入 | 预期 |
| --- | --- |
| 空 query 全目录分页 | 可遍历所有准入 ProjectVolume operation |
| `ProjectVolume` | 返回数据卷 list/get/create/update/delete/preview/transfer 相关摘要 |
| `项目数据卷`、`存储卷` | 中文别名可发现同一工具族 |
| `project volume storage classes` | 英文目标可发现存储类型/存储类能力 |
| 精确 `listProjectVolumes` | 详情包含真实分页/项目参数和成功响应 Schema |
| 精确 create/update/delete | `requiresApproval` 与后端归一化策略一致 |
| import/retry/文件传输协议 | 若在 deny map 中则搜索与详情都不可见，并返回稳定 missing 结果 |

禁止用人工 Catalog fixture 代替这组生产 OpenAPI 测试。

## 5. 权限与审批测试

在隔离测试数据库中至少覆盖：

1. 用户 A 的 Run 不能读取或执行用户 B 的 ToolCall；
2. 已失效 Session、会话所有者不匹配、项目空间绑定不匹配均 fail closed；
3. 普通工具无需审批但仍经过业务 RBAC；
4. 高危工具无批准时进入 waiting approval；
5. `reject` 只把当前 ToolCall 标记 rejected，Run 可继续；
6. `approve` 只允许本次调用；
7. `approve_always` 创建 `userId + operationId` 豁免；
8. 豁免不能跨用户或跨 operationId，撤销后再次调用必须审批；
9. Agent 自报 Header、用户 ID、项目 ID 或 operationId 不能覆盖数据库权威绑定；
10. 敏感参数只能由受控直接输入模式提供，不能从模型参数透传。

## 6. 真实链路与事件测试

至少选择一个无副作用读取和一个可回收的 ProjectVolume 写操作，通过真实 Web/BFF 入口创建 Run，
让 Agent 搜索、加载详情并执行。验收：

- Luna API 直接进入原业务 Handler/Service，不经过 delegation exchange、internal execute 或二次
  platform router；
- 幂等键绑定 toolCallId，重放不重复产生副作用；
- 写操作完成后从权威业务读取接口确认最终状态；
- 初始用户消息、工具调用、批准/拒绝、工具结果、卡片、标题变化、错误与终态都能从不可变事件
  重放；
- SSE 序列无缺口；连接中断后从 cursor 恢复，Run 被进程中断时形成 interrupted/failed 终态而非
  lease takeover；
- 旧卡片 payload 仍能渲染，新卡片只通过三个通用工具生成；
- AI 计费仍只归属个人钱包，所有 AI 账单记录 project_id 为空。

## 7. 可观测与发布门禁

- Web/BFF、Agent Run、模型、`search_tools`、`get_tool_details`、真实平台 HTTP 和数据库操作处于
  同一 Trace 父子链；
- 成功、失败和跨服务链各抽样一条，失败 Span 正确设置状态；
- 日志包含稳定事件名与可关联 run/toolCall 字段；Metric label 不含 query、资源名、用户 ID、
  project ID、request ID 或 trace ID；
- Authorization、Cookie、Session、API Key、Prompt、敏感参数和原始工具结果不进入普通遥测；
- 运行 Go 全量测试、Agent lint/typecheck/test/build、Web lint/build/singleton 检查、文档构建、
  OpenAPI 契约与数据库 fresh/upgrade 测试后，才可在 TODO 中标记完成。
