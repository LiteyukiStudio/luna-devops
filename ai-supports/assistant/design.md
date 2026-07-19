# 平台内嵌 AI 助手设计

## 目标

Luna DevOps 需要提供一个内嵌在前端里的 AI 助手，以小窗形式呈现。助手用于帮助用户查看项目空间、理解构建和部署状态、准备操作、诊断问题，并在用户确认后执行安全操作。

这是平台内部产品功能。第一版不需要通过 MCP 调用平台能力，而是由后端 Agent 直接调用共享 Tool Kernel。这样可以更好接入 session、RBAC、MFA step-up、确认弹窗、审计、trace 和前端跳转。

## 方向

第一版 Agent runtime 使用 ADK Go，但业务逻辑不能和 ADK API 绑定死。

```text
前端 AI 小窗
  -> /api/v1/assistant/sessions
  -> /api/v1/assistant/sessions/{sessionId}/messages
  -> /api/v1/assistant/sessions/{sessionId}/events
  -> API 进程内 assistant module
  -> AgentRuntime interface
  -> ADK Go runtime adapter
  -> shared Tool Kernel
  -> 现有 Luna DevOps services/API
```

ADK 可以负责 Agent 执行、tool calling、session state 和模型交互。Luna DevOps 必须自己负责租户隔离、用户身份、项目空间权限、高危确认、MFA step-up、审计和输出脱敏。

## 部署形态

第一阶段作为 API 进程内的逻辑模块运行：

```text
cmd/api
  internal/assistant
  internal/ai/tools
  internal/ai/policy
  internal/ai/confirmation
  internal/ai/audit
  internal/ai/adkadapter
```

不要一开始就拆成独立服务。助手需要直接访问当前浏览器 session、CSRF 上下文、项目空间 RBAC、确认状态和审计逻辑。过早拆服务会带来服务间身份委托、SSE 转发、跨服务确认等复杂度。

代码仍然要按未来可拆服务来写：

```go
type AgentRuntime interface {
    Run(ctx context.Context, req RunRequest) (<-chan AgentEvent, error)
}
```

未来如果要拆出独立 `assistant` 服务，可以把 `AgentRuntime` 替换成 REST client。

## 会话和多租户隔离

不要把 ADK session 字段当成安全边界。平台助手会话应存储在 Luna DevOps 自己的数据表里。

建议表：

```text
ai_agents
ai_agent_sessions
ai_agent_messages
ai_tool_calls
ai_agent_project_scopes
ai_confirmations
```

`ai_agent_sessions` 至少包含：

```text
id
agent_id
user_id
project_id nullable
title
adk_app_name
adk_user_id
adk_session_id
created_at
updated_at
```

ADK 参数可以从平台状态派生：

```text
appName   = "luna-devops-assistant"
userId    = user.ID
sessionId = ai_agent_sessions.id
```

写入 ADK state 的 projectId 只能作为模型上下文，不能作为权限依据。每次 tool call 都必须重新检查当前用户、项目空间成员关系、session/token 有效性和 tool policy。

## 前端交互

前端 AI 小窗应支持：

- assistant 响应流式输出
- 展示 tool-call 步骤
- 跳转到关联的项目、应用、构建、发布页面
- 展示紧凑的构建日志片段
- 展示内嵌确认卡片
- 需要时触发 MFA step-up
- 支持 retry/cancel pending assistant run

建议 REST endpoint：

```text
POST /api/v1/assistant/sessions
GET  /api/v1/assistant/sessions
GET  /api/v1/assistant/sessions/{sessionId}
POST /api/v1/assistant/sessions/{sessionId}/messages
GET  /api/v1/assistant/sessions/{sessionId}/events
POST /api/v1/assistant/confirmations/{confirmationId}/approve
POST /api/v1/assistant/confirmations/{confirmationId}/reject
```

第一版事件流使用 SSE。只有在确实需要双向实时流时，再考虑 WebSocket。

## Shared Tool Kernel

Tool Kernel 是内部助手和未来外部 MCP 共用的核心。

```text
tool descriptor
  -> input validation
  -> actor/session/token resolution
  -> authz scope check
  -> project RBAC check
  -> risk policy
  -> confirmation guard
  -> service/API adapter
  -> output projection/redaction
  -> audit
```

内部 ADK tools 直接调用 Tool Kernel。未来 MCP tools 通过 MCP adapter 调用同一个 Tool Kernel。

## Skill 加载策略

内部助手不要在每次会话里一次性加载全部平台能力。第一步只加载 `luna-devops-router` 和很短的系统约束，由 router 根据用户意图选择后续模块。

推荐流程：

```text
用户消息
  -> 加载 luna-devops-router
  -> 判断模块和风险等级
  -> 加载 1 到 3 个相关模块 skill
  -> 选择 Tool Kernel tools
  -> 执行 read / preflight / confirmation / mutation
```

模块 skill 只描述操作边界、必要入参、风险分级和诊断顺序，不承载业务权限。实际权限仍由 Tool Kernel 和后端 service 判断。

当用户目标跨多个模块时，优先加载：

| 场景 | 推荐 skill |
| --- | --- |
| 从代码到上线 | `luna-devops-source`、`luna-devops-build`、`luna-devops-deployment`、`luna-devops-gateway` |
| 构建失败 | `luna-devops-build`、`luna-devops-debugging` |
| 应用不能访问 | `luna-devops-deployment`、`luna-devops-gateway`、`luna-devops-runtime`、`luna-devops-debugging` |
| 服务间调用失败 | `luna-devops-topology`、`luna-devops-runtime`、`luna-devops-debugging` |
| 费用异常 | `luna-devops-billing`、`luna-devops-deployment`、`luna-devops-debugging` |
| 登录或权限异常 | `luna-devops-security`、`luna-devops-debugging` |

## 确认交互

内部助手的确认应发生在平台 UI 内。

流程：

```text
assistant 准备执行 risky tool
  -> Tool Kernel 创建 pending intent
  -> assistant stream 返回 confirmation_required
  -> 前端渲染 confirmation card
  -> 用户点击 approve/reject
  -> 后端重新校验 intent、actor、session、RBAC、resource digest
  -> 后端执行或取消
```

前端按钮不是安全边界，后端 pending intent 才是安全边界。

## 风险策略

第一版推荐策略：

| 风险等级 | 内部助手策略 |
| --- | --- |
| `read` | 直接执行并审计 |
| `low` | 直接执行并审计 |
| `medium` | 需要内嵌确认 |
| `high` | 需要内嵌确认；按配置要求 MFA step-up |
| `critical` | 仅 browser session，要求 MFA step-up 和明确确认，或直接禁用 |

第一版不要开放 runtime exec、terminal、data export、data retention cleanup、raw secret read 或 security settings mutation。

## 推进计划

### 阶段 1：只读助手

- 实现 assistant sessions 和 SSE events。
- 增加 dashboard、projects、applications、build runs、releases、gateway routes、events、billing summary 等只读 tools。
- 增加输出脱敏和 tool-call 审计。

### 阶段 2：带确认的构建动作

- 增加 build trigger 和 build cancel。
- 实现 pending intent 和内嵌确认卡片。
- 记录 confirmation audit。

### 阶段 3：发布计划

- 增加 release plan dry-run 输出。
- 只在确认流程稳定后开放 release create 和 rollback。
- 增加专用 step-up purposes，例如 `deployment_release` 和 `deployment_rollback`。

### 阶段 4：复用到外部 MCP

- 通过 `/api/v1/mcp` 暴露同一个 Tool Kernel。
- 外部 MCP auth 使用现有 Access Token scopes。
- 外部 MCP 高危 tool call 返回平台 confirmation URL。
