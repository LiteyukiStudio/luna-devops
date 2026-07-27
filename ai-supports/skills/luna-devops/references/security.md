# 安全与账号

## 机器目录

按任务分别查询：

- `luna help catalog category=auth limit=100 output=json interactive=false agent=true`
- `luna help catalog category=user limit=100 output=json interactive=false agent=true`
- `luna help catalog category=access-token limit=100 output=json interactive=false agent=true`

执行具体工具前使用
`luna help command path=<category.tool> output=json interactive=false agent=true`
确认会话类型、Scope、风险、MFA purpose 和安全输入方式。

## 工作流

1. 读取活动实例、认证状态、账号和服务端认证能力。
2. 区分 CLI 本地凭据、Web Session、OAuth Token、OIDC 绑定和个人访问令牌。
3. 用户、Provider、注册策略、密码、MFA、OAuth App 或 Token 变更前读取当前状态。
4. 采用最小 Scope，按 Help Schema 执行一次变更，再重新读取验证。
5. 用户可以在自己的终端执行 `luna login` 进入默认 Device Code 登录，也可显式选择 Help 提供的备用方式。

## MFA 与协议入口

- OIDC/OAuth callback、Device Authorization 页面和 Webhook 接收端点不是 Agent 直接调用的业务工具。
- 收到 `mfa_required` 时记录 `purpose` 与 request ID，停止并让用户在自己的终端完成 CLI 提供的用户在场验证；随后重新读取状态，最多重放一次原命令。
- Agent 不接收、转述或记录 OTP、恢复码、密码和 Token。
- 登录与 Step-up MFA 是不同事务，已有登录凭据不代表满足当前敏感操作。
- CLI 终端和数据导出要求 OAuth 登录与对应 purpose 的有效 Step-up assertion。
  个人访问令牌不能满足或绕过该协议授权。

## Secret 与权限

- 密码、Token、OTP、恢复码和 Provider Secret 不进入对话或内联参数。
- 创建访问令牌时使用最小 Scope；明文只可在一次性安全输出中交付。
- Token 吊销、MFA 重置、身份源和用户管理按机器 Help 风险处理。
- 不通过管理员账号、扩大 Scope、重新登录其他实例或通用 API 绕过用户在场要求。
