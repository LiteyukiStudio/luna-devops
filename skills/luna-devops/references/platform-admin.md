---
name: platform-admin
description: 面向平台管理员（role=admin，未绑定单项目的全局上下文）提供系统全局设置、邮件与系统组件、用户与 OIDC 身份源策略、全局构建环境、平台应用市场模板、AI 模型集成与 Agent 遥测诊断、跨租户财务调账及数据保留两阶段安全清理规程。
---

# 平台管理员全局治理规程

## 目录
- [1. 全局作用域门禁与高频入口](#1-全局作用域门禁与高频入口)
- [2. 系统配置、身份认证与 AI 模型连通性闭环](#2-系统配置身份认证与-ai-模型连通性闭环)
- [3. SOP：数据保留两阶段安全清理（强制 Preview）](#3-sop数据保留两阶段安全清理强制-preview)
- [4. 跨租户账单审计、人工调账与 Agent 遥测诊断](#4-跨租户账单审计人工调账与-agent-遥测诊断)
- [5. 错误恢复速查表](#5-错误恢复速查表)

---

## 1. 全局作用域门禁与高频入口

- **双重准入门禁（Gate）**：所有 `admin_*` 工具仅对平台管理员（`role=admin`）开放，且**必须在未绑定单一项目的全局上下文执行**（单项目沙箱会话调用 `admin_*` 会直接拒绝）。非管理员账号切勿检索或尝试调用本规程工具。
- **高频入口工具**：
  - 系统与身份：`admin_get_system_configs`、`admin_list_system_components`、`admin_list_users_and_auth`、`admin_list_git_providers`；
  - AI 与可观测：`admin_list_ai_models`、`admin_test_ai_integration`、`admin_inspect_agent_observability`；
  - 构建模板与全局通知：`admin_get_global_build_env`、`admin_list_platform_app_templates`、`admin_list_platform_notifications`；
  - 财务与数据保留：`admin_inspect_billing`、`admin_preview_data_retention`、`admin_execute_data_retention_cleanup`（运行集群注册与配额治理见 `runtime-volumes` 中的 `runtime_save_cluster` / `runtime_inspect_cluster_governance`）。

---

## 2. 系统配置、身份认证与 AI 模型连通性闭环

1. **先读现行键值再增量写入**：
   - 修改系统参数（`admin_update_system_configs`）、邮件服务（`admin_configure_platform_mail`）、注册准入与认证策略（`admin_configure_auth_policy`）或 OIDC/Git 身份源（`admin_save_oidc_provider` / `admin_save_git_provider`）前，必须先调对应读取工具核对当前生效值与影响范围。
   - **敏感凭据增量保留**：SMTP 密码、OIDC Client Secret、Git OAuth Secret 与 AI Provider API Key 均采用只写不回显增量语义——更新其他非敏感字段时省略密钥字段即保持原已存密钥不变，切勿传占位符（如 `"***"`）覆盖真实密钥。
2. **AI 模型变更必测连通性**：
   - 通过 `admin_save_ai_model` 新增或更新模型端点、名称或费率后，紧接着调用 `admin_test_ai_integration` 进行真实探测，确认连通正常后再通知管理员投入使用。

---

## 3. SOP：数据保留两阶段安全清理（强制 Preview）

历史事件、投递记录、构建/发布/Hook 日志、运行资源采样记录与历史扣费明细的物理清理不可逆，必须严格执行以下低自由度序列：

- [ ] **Step 1 — 目录与范围预检（`admin_preview_data_retention`，`L0`）**：
  - 先调用 `admin_preview_data_retention` 查看支持的 8 类数据集目录（`platform_events | notification_deliveries | build_logs | release_logs | hook_run_logs | expired_auth_data | runtime_observations | billing_records`）及拟清理时间窗口（`startAt` ～ `endAt`，RFC3339）内的预计命中行数与时间水位。
- [ ] **Step 2 — 影响面确认（Gate）**：
  - 将命中的数据集名称、起止时间窗口与预计删除记录条数清晰呈现给管理员，若命中条数为 `0` 则直接结束无需执行清理。
- [ ] **Step 3 — 预检加锁并执行清理（`admin_execute_data_retention_cleanup`，`L2`）**：
  - 经 `preview_tool` 获取签名锁并获管理员确认后执行清理：

```json
{
  "datasets": ["build_logs", "release_logs", "platform_events"],
  "startAt": "2026-01-01T00:00:00Z",
  "endAt": "2026-06-01T00:00:00Z"
}
```

---

## 4. 跨租户账单审计、人工调账与 Agent 遥测诊断

1. **跨租户财务审查与调账红线**：
   - 调用 `admin_inspect_billing` 时注意聚合口径：未指定单一目标用户时返回全平台汇总（`platform_total`），指定用户时返回该用户钱包余额与明细。
   - 人工充值或扣减调账（`admin_adjust_user_wallet`，`L2`）必须传入真实明确的审计原因 `reason`（写入不可变账本与审计日志）。
   - 调整计费费率规则（`admin_update_billing_rate_rules`，`L2`）仅对后续新产生的计量结算周期生效，不会回溯篡改历史已出账流水。
2. **内置 AI 助手运行质量诊断**：
   - 排查平台内置 AI 助手响应慢、模型报错率升高或特定工具高频失败时，调用 `admin_inspect_agent_observability` 按时间窗口下钻运营概览、异常轮次与工具错误码分布（注意工具参数与对话敏感正文按零信任脱敏规范不暴露明文）。

---

## 5. 错误恢复速查表

| 现象或错误码 | 根因 | 标准恢复动作（含禁止行为） |
| --- | --- | --- |
| `forbidden` 或找不到 `admin_*` 工具 | 当前用户非平台 `admin`，或当前处于绑定单一项目的受限会话 | 禁止重试或尝试拼凑底层接口；如实说明需要平台管理员身份并在全局未绑定项目的上下文执行 |
| 更新 AI 模型或 SMTP 后鉴权失败（误传了 `"***"`） | 将脱敏占位符当作新明文密钥写入覆盖了原密钥 | 通知管理员重新提供真实密钥并通过敏感字段重设；后续非密钥修改务必省略敏感字段 |
| `admin_execute_data_retention_cleanup` 时间窗口校验失败 | `endAt` 不晚于 `startAt`，或时间格式非合法 RFC3339 | 修正为带时区的标准 RFC3339 时间戳并先通过 `admin_preview_data_retention` 核验范围 |
| 删除全局通知渠道/模板报仍被规则引用 | 存在关联全局通知规则尚未解绑或删除 | 先调 `admin_list_platform_notifications` 核查引用关系，使用 `admin_delete_platform_notification_resource` 按先规则后渠道/模板顺序清理 |
