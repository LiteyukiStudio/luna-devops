---
name: luna-devops-security
description: Luna DevOps 认证、用户和令牌操作。用于 local login、MFA、OIDC、OAuth applications、OAuth grants、users、Access Tokens、scope、admission policy 和 step-up 安全策略。
---

# 安全与身份 Skill

## 适用能力

- 当前用户、用户列表、用户创建和更新。
- MFA 状态、绑定、确认、验证、恢复码、禁用、管理员重置。
- Auth providers 和 OIDC admission policy。
- OAuth applications、grants、authorize、token、revoke。
- Access Token scopes、创建、撤销。

## 操作流程

1. 先确认 actor 是否是本人操作、项目成员操作还是平台管理员操作。
2. 用户管理前确认平台管理员权限。
3. OIDC/Auth provider 变更前检查 callback URL 和 step-up。
4. Access Token 创建时使用最小 scopes 和过期时间。
5. MFA 管理优先通过 browser session，不使用 Bearer token。

## 风险边界

- MFA、Auth provider、用户管理、security settings 都是 high risk。
- 二次验证只支持 browser session，Access Token 不能满足 step-up。
- Access Token 只显示一次，之后不回显。
- 不输出 recovery codes，除非处于用户刚生成后的安全展示流程。

