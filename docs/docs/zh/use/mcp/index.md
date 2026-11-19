# MCP 接入

Luna DevOps 在 `https://<实例域名>/api/v1/mcp` 内置原生 Streamable HTTP MCP Server。你可以将 Claude Code、Cursor、Antigravity、Windsurf 或任何兼容 Streamable HTTP 的 MCP 客户端直连到平台，查询资源、排查故障并受控执行交付操作。

即使自托管实例未开启网页内置的 AI 助手（`AI_ASSISTANT_AVAILABLE=false`），`/api/v1/mcp` 同样默认可用，由外部客户端自身的模型运行时驱动调用。

## 前置条件与认证方式

- 实例已配置可访问的 HTTPS 地址（本地开发可使用 `localhost` HTTP）。
- 准备以下任一 Bearer 身份凭据；服务端在每次调用时同时校验令牌 Scope 与当前账号的实时平台/项目角色权限：

### 1. 个人访问令牌（PAT，最快上手）

在控制台右上角进入**账号设置 → 个人令牌**（个人访问令牌），按最小权限原则勾选所需 Scope（例如日常排障只勾选只读 Scope，需要变更部署时再勾选对应写 Scope），创建形如 `lyd_...` 的令牌，并在客户端通过请求头传入：

```text
Authorization: Bearer <YOUR_PAT>
```

### 2. OAuth 2.0 应用

在**账号设置 → 开发者应用**（OAuth 应用）创建应用，登记精确的回调地址（生产环境必须使用 HTTPS，本机开发允许 `localhost` HTTP）与允许申请的 Scope。支持 MCP OAuth 规范的客户端会自动读取以下元数据端点并完成 `authorization_code` + PKCE（`S256`）浏览器授权：

- 受保护资源元数据：`/.well-known/oauth-protected-resource`
- 授权服务器元数据：`/.well-known/oauth-authorization-server`

## 最短接入步骤

将示例中的 `https://devops.example.com` 替换为你的实例地址，`<YOUR_PAT>` 替换为刚签发的 `lyd_...` 令牌。

### Claude Code

```bash
claude mcp add --transport http luna-devops https://devops.example.com/api/v1/mcp \
  --header "Authorization: Bearer <YOUR_PAT>"
```

### Cursor / Antigravity / 标准 `mcpServers` 配置

在客户端的 MCP 配置文件中添加：

```json
{
  "mcpServers": {
    "luna-devops": {
      "url": "https://devops.example.com/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <YOUR_PAT>"
      }
    }
  }
}
```

### 预期结果

保存配置后，客户端 MCP 列表会显示 `luna-devops` 已连接并加载 7 个渐进式元工具。可直接发送“列出我有权限的项目空间”验证连通性；推荐进一步阅读[配套 Skills](./skills) 了解领域工作流加载方式。

## 渐进式工具发现与安全门禁

为避免一次性向模型上下文注入上百个 Schema，MCP Server 通过 **7 个内置渐进式元工具**按需暴露覆盖 7 大业务域的 **166 个平台操作**：

| 元工具 | 用途 |
| --- | --- |
| `search_tools` | 按单一业务域短语检索候选平台工具。 |
| `get_tool_details` | 查看指定工具的完整输入 Schema、风险分级（`L0` / `L1` / `L2`）与所需令牌 Scope。 |
| `preview_tool` | 对 `L2` 高风险变更执行只读预检，返回脱敏 `before -> after` 差异、风险提示与签名锁 `previewLock`。 |
| `execute_tool` | 在令牌 Scope 与实时 RBAC 校验下执行目标工具。 |
| `load_skill` | 按需加载业务域工作流规程，见[配套 Skills](./skills)。 |
| `search_docs` | 检索随当前平台版本打包的官方公开文档。 |
| `read_doc` | 按章节读取官方文档原文，核对字段格式、模板变量与限制。 |

### 三级风险门禁与 `L2` 两阶段预检锁

所有工具调用均受平台分级门禁约束：

- **`L0`（只读查询与诊断）**：直接调用 `execute_tool(toolName, arguments)`。
- **`L1`（轻量可逆元数据或个人偏好变更）**：核对参数后调用 `execute_tool(toolName, arguments, confirmed: true)`。
- **`L2`（高风险与生产变更，强制两步锁）**：涵盖生产环境（`stage=prod`）、部署配置规格/环境变量/密钥/卷挂载、触发构建或发布、扩缩容与重启、访问入口、成员角色、删除资源及平台治理：
  1. 先调用 `preview_tool(toolName, arguments)` 获取脱敏 `before -> after` Diff 与绑定当前参数和资源状态的 `previewLock`；
  2. 向用户展示变更目标与差异并取得确认；
  3. 携带完全一致的参数与 `previewLock` 调用 `execute_tool(toolName, arguments, previewLock)`。审批期间若底层资源已被并发修改，服务端会拒绝执行以防止覆盖他人变更（TOCTOU 保护）。

### 保留在 Web 控制台完成的操作

出于凭据边界或流式协议限制，以下操作不通过 MCP 工具开放，需在浏览器控制台完成：

- 签发个人访问令牌（PAT）、修改登录密码、管理 OAuth 应用，以及完成 Git / OIDC 浏览器授权绑定；
- 容器交互式 WebSocket TTY 终端（AI 客户端仅可在已开启运行终端策略时执行单次非交互命令）；
- 本地归档文件上传导入数据卷，以及下载已导出的归档文件；
- 部署配置的高级 Kubernetes 调度字段（`initContainers`、`sidecarContainers`、`tolerations`、`affinity` 等）与跨项目 JSON Bundle 导入向导。

## 常见问题排障

- **`401 Unauthorized`（响应含 `WWW-Authenticate` 头）**：未携带有效的 `Authorization: Bearer <token>`，令牌已过期/被吊销，或 OAuth 客户端尚未完成授权。检查令牌配置或重新执行 OAuth 登录。
- **`403`（`auth.token.scope_insufficient`）**：当前账号即使具备对应角色，所持 PAT 或 OAuth 授权也缺少该工具要求的 Scope。先用 `get_tool_details` 查看 `requiredScopes`，再签发或重新授权包含对应 Scope 的凭据。
- **`ai.tool_preview_lock_required`**：调用 `L2` 写工具时未先执行 `preview_tool`，或在获取 `previewLock` 后改动了参数。用最终参数重新调用 `preview_tool`，向用户确认后原样传入返回的 `previewLock`。
- **`ai.preview_conflict`**：目标资源在 `preview_tool` 与 `execute_tool` 之间已被其他操作修改。重新调用 `preview_tool` 拉取最新状态并确认差异后再执行。
