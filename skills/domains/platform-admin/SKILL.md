---
name: platform-admin
description: 平台管理员专属系统配置、AI Provider 与遥测审计、用户与 OIDC/邮件认证策略、全局构建环境、应用市场模板、跨租户计费调账与数据保留清理工作流。
---

# 平台管理员全局治理工作流

## 适用范围

仅面向平台管理员（`role=admin`，且需在未绑定单一项目的全局上下文执行），涵盖：
- 系统配置、邮件与系统组件：`admin_get_system_configs`、`admin_update_system_configs`、`admin_configure_platform_mail`、`admin_list_system_components`、`admin_install_system_component`；
- 用户、注册准入与 OIDC 身份源：`admin_list_users_and_auth`、`admin_save_user`、`admin_configure_auth_policy`、`admin_save_oidc_provider`、`admin_list_git_providers`、`admin_save_git_provider`、`admin_delete_git_provider`；
- AI 模型配置与 Agent 可观测诊断：`admin_list_ai_models`、`admin_save_ai_model`、`admin_delete_ai_model`、`admin_test_ai_integration`、`admin_inspect_agent_observability`；
- 全局构建环境、应用市场模板与全局通知：`admin_get_global_build_env`、`admin_configure_global_build_env`、`admin_list_platform_app_templates`、`admin_save_platform_app_template`、`admin_delete_platform_app_template`、`admin_list_platform_notifications`、`admin_save_global_notification_channel`、`admin_save_global_notification_rule`、`admin_save_notification_template`、`admin_delete_platform_notification_resource`、`admin_test_or_retry_global_notification`；
- 跨租户财务审计、费率与数据保留清理：`admin_inspect_billing`、`admin_update_billing_rate_rules`、`admin_adjust_user_wallet`、`admin_list_data_retention_catalog`、`admin_preview_data_retention`、`admin_execute_data_retention_cleanup`（集群级注册与配额策略配合 `runtime-volumes` 中的 `runtime_save_cluster` 等工具）。

## 标准工作流

1. **先读现行配置、增量变更与连通性验证**：
   - 修改系统配置、认证策略、邮件或 AI 模型前，先调用 `admin_get_system_configs`、`admin_list_users_and_auth` 或 `admin_list_ai_models` 核对现行键值与影响面；更新 AI Provider 或模型后调用 `admin_test_ai_integration` 实测连通性。
   - 涉及 OIDC Client Secret、SMTP 密码、Git OAuth Secret 与 AI API Key 时一律走敏感写字段增量更新，留空/省略代表保持已保存密钥不变，严禁回显。
2. **数据保留两阶段安全清理（强制先 Preview）**：
   - 执行历史构建、事件或观测记录清理前，必须先调用 `admin_list_data_retention_catalog` 与 `admin_preview_data_retention` 获取预计命中行数与时间水位，向管理员确认影响范围后再执行 `admin_execute_data_retention_cleanup`。
3. **跨租户账单审计、调账与 Agent 遥测排障**：
   - 审查全平台余额或特定用户消费时使用 `admin_inspect_billing`（注意未指定单一用户时汇总为 `platform_total`）；执行人工充值或扣减调账（`admin_adjust_user_wallet`）必须填写清晰的审计原因 `reason`，修改计费费率规则（`admin_update_billing_rate_rules`）只影响后续新产生周期的计量结算，不篡改历史已结算账单。
   - 排查平台内置 AI 助手异常轮次、工具错误率或模型延迟时，使用 `admin_inspect_agent_observability` 按时间窗口下钻概览、慢轮次与失败工具码。
