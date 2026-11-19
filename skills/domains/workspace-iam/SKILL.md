---
name: workspace-iam
description: 项目空间生命周期、成员角色、项目网络隔离与跨项目共享、个人安全设置与凭据吊销工作流。
---

# 项目空间与身份权限工作流

## 适用范围

处理项目空间与工作台看板（`workspace_list_projects`、`workspace_get_project`、`workspace_create_project`、`workspace_update_project`、`workspace_pin_project`、`workspace_reorder_my_dashboard_projects`、`workspace_request_billing_transfer`、`workspace_delete_project`）、项目成员管理（`workspace_list_members`、`workspace_save_member`、`workspace_remove_member`）、项目网络隔离与跨项目内网共享（`workspace_get_network_policy`、`workspace_configure_network_policy`、`workspace_list_network_shares`、`workspace_create_network_share`、`workspace_accept_network_share`、`workspace_delete_network_share`）及个人资料与凭据安全（`ops_get_my_account_security`、`ops_update_my_profile`、`ops_revoke_my_credential_or_grant`）。

## 标准工作流

1. **双模项目定位与作用域边界**：
   - 项目级工具的 `project` 参数统一支持项目主键 ID（`prj_xxx`）或唯一英文 `identifier`（不确定时先调用 `workspace_list_projects` 确认，不要把中文展示名当作 `identifier`）。
   - 若当前上下文受限于单一项目沙箱而操作涉及跨项目或全局资源（返回 `ai.project_scope_mismatch`），应如实告知用户切换至未绑定单项目的全局上下文执行。
2. **成员角色、网络隔离与计费转移**：
   - 调整成员前先调用 `workspace_list_members` 核对当前成员与候选用户，确保项目至少保留一名 `owner`。
   - 配置项目网络策略（`workspace_configure_network_policy`）时准确区分入站模式枚举：`isolated`（同项目空间内服务默认互通、隔离其他项目）、`dependencies`（仅按显式 `ServiceBinding` 或跨项目共享放行入站，对应口语“按显式依赖放行 / explicit”）。
   - 跨项目内网共享遵循提供方创建（`workspace_create_network_share`）→ 消费方确认（`workspace_accept_network_share`）双向确认机制；若提供方持有受限网段出站例外，还需集群维护者执行 `runtime_approve_cluster_network_share`。
3. **个人凭据红线（人工交接边界）**：
   - 平台永久禁止 AI 签发新个人访问令牌（PAT）、修改本人登录密码、绑定外部 OIDC 身份或创建/编辑第三方 OAuth 应用（防止回调劫持与明文密钥进入上下文）。
   - AI 仅可通过 `ops_get_my_account_security` 查看凭据与授权摘要，或通过 `ops_revoke_my_credential_or_grant` 吊销既有凭据；新增凭据请明确引导用户前往 Web 控制台“账号设置 → 安全 / 个人访问令牌 / OAuth 应用”人工完成。
