# 安全与账号

当前目录覆盖 `auth`、`user` 和 `access-token` 的部分 HTTP 操作，以及 CLI
本地 Access Token 登录。

## 工作流

1. 读取当前实例、认证状态和 `/meta` 能力。
2. 区分本地 CLI 凭据、Web Session、OAuth Token 和个人 Access Token。
3. 用户、Provider、注册策略、密码、MFA 或 Token 变更前读取当前状态。
4. 按 Help Schema 执行一次变更，再重新读取验证。

## 协议入口

- OIDC callback 不是 Agent 可直接调用的业务工具。
- 登录、注册和 TOTP 端点可能依赖 Cookie、CSRF、临时事务或用户在场。
- 当前 Device Code 与 OAuth Bearer Step-up MFA 未实现。
- 遇到 `mfa_required` 时停止，不索取 OTP 或恢复码，不重复使用同一 Token。

## Secret 与权限

- 密码、Token、OTP、恢复码和 Provider Secret 不进入对话或内联参数。
- 创建 Access Token 时使用最小 Scope；明文只可在一次性安全输出中交付。
- Token 吊销、MFA 重置和用户管理按 Help 风险处理。
- 不通过管理员上下文绕过用户在场要求。
