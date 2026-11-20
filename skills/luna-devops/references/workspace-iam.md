---
name: workspace-iam
description: 管理项目空间生命周期、成员角色 RBAC、项目入站/出站网络隔离策略、跨项目服务共享双向握手、计费主体转移及个人凭据安全。当用户创建或查看项目、增删改成员权限、配置 NetworkPolicy 隔离/白名单、跨项目打通内网调用或查看/吊销个人访问凭据时加载本规程。
---

# 项目空间、网络隔离与身份权限规程

## 目录
- [1. 核心入口与自由度边界](#1-核心入口与自由度边界)
- [2. 项目定位、成员角色与计费转移 SOP](#2-项目定位成员角色与计费转移-sop)
- [3. 项目网络隔离与跨项目内网共享闭环](#3-项目网络隔离与跨项目内网共享闭环)
- [4. 个人凭据红线与控制台交接边界](#4-个人凭据红线与控制台交接边界)
- [5. 状态漂移与错误恢复速查表](#5-状态漂移与错误恢复速查表)

---

## 1. 核心入口与自由度边界

- **高频入口工具**：`workspace_list_projects`、`workspace_get_project`、`workspace_list_members`、`workspace_get_network_policy`、`workspace_list_network_shares`、`ops_get_my_account_security`（其余写工具按单域短词调 `search_tools` + `get_tool_details` 获取）。
- **自由度设定**：
  - **高自由度**：查询项目资源盘点、权限诊断与网络互通性分析（优先调用一站式 `workspace_get_project` 同时拿到资源计数、当前角色及近 7 天异常摘要）。
  - **低自由度**：成员角色降级/移除、修改项目 `NetworkPolicy` 入站/出站模式、跨项目内网共享握手（必须严格遵循前置读取与 `L2` 预检锁顺序）。

---

## 2. 项目定位、成员角色与计费转移 SOP

1. **双模项目定位（禁止拿中文名充当 `identifier`）**：
   - 所有项目级工具的 `project` 参数仅接受主键 ID（`prj_xxx`）或唯一英文 `identifier`（如 `core-backend`）。仅知中文展示名时，必须先调 `workspace_list_projects` 解析出稳定 `id` 或 `identifier`。
   - 若当前令牌或会话绑定在单一项目作用域下，访问其他项目或全局资源会返回 `ai.project_scope_mismatch`；切勿反复尝试，直接提示用户切换到未绑定单项目的全局上下文。
2. **成员角色变更与最后 Owner 保护**：
   - 执行 `workspace_save_member` 或 `workspace_remove_member` 前，先调 `workspace_list_members` 确认现有角色分布。
   - **硬门禁（Gate）**：每个项目空间必须至少保留 **1 名 `owner`**。若目标是唯一 `owner`，必须先将另一名成员提升为 `owner` 后才能降级或移除原 `owner`。
3. **计费主体转移**：
   - 发起项目计费转移使用 `workspace_request_billing_transfer`（只是发出待办请求，并未即时生效），需接收方在站内信（`ops_list_inbox` → `ops_decide_inbox_action`）确认后才完成切换。

---

## 3. 项目网络隔离与跨项目内网共享闭环

### 3.1 项目网络策略增量配置（`workspace_configure_network_policy`，`L2`）
修改前先调 `workspace_get_network_policy` 读取现行策略，严格区分枚举语义：
- **入站模式 `ingressMode`**：
  - `isolated`：同项目空间内服务默认互通，默认拒绝其他项目空间入站流量；
  - `dependencies`（口语常称“显式依赖模式 / explicit”）：即使同项目内也默认拒绝，仅放行已建立 `ServiceBinding` 或已生效跨项目共享的调用方。
- **出站模式 `egressMode`**：`internet`（允许公网出站）｜`allowlist`（仅放行 `upsertAllowlistEntries` 中的公网 CIDR，上限 64 条）｜`none`（禁止公网外联）。

```json
{
  "project": "core-backend",
  "ingressMode": "dependencies",
  "egressMode": "allowlist",
  "upsertAllowlistEntries": [{ "cidr": "203.0.113.0/24", "description": "第三方支付网关" }]
}
```

### 3.2 跨项目内网共享三步握手 Checklist
当消费方项目 A 需要内网访问提供方项目 B 的某个部署环境时，按顺序核对：
- [ ] **Step 1（提供方发起）**：在提供方项目 B 调用 `workspace_create_network_share`（指定提供方 `target` 与消费方 `consumerProjectIdentifier`）。
- [ ] **Step 2（消费方确认）**：消费方项目 A 管理员调用 `workspace_accept_network_share`（传入 `shareId`）。
- [ ] **Step 3（受限网段特批 Gate）**：若提供方项目持有集群受限出站例外规则，共享会停留在待集群审批状态，须由集群管理员调用 `runtime_approve_cluster_network_share` 放行后网络策略才会下发。

---

## 4. 个人凭据红线与控制台交接边界

- **只读与吊销权限**：AI 仅可通过 `ops_get_my_account_security` 查看当前账号已签发 PAT、OAuth 授权与安全状态，或经用户确认后调用 `ops_revoke_my_credential_or_grant` 吊销泄露/闲置凭据。
- **严禁代办边界**：平台永久禁止通过 AI 工具新建 PAT、修改密码、配置 MFA 或注册/编辑 OAuth 应用。遇到此类请求，直接引导用户前往 Web 控制台 **「账号设置 → 安全 / 个人访问令牌 / OAuth 应用」** 人工完成。

---

## 5. 状态漂移与错误恢复速查表

| 现象或错误码 | 根因 | 标准恢复动作（含禁止行为） |
| --- | --- | --- |
| `project.not_found`（传入了中文项目名） | 将项目中文 `name` 误传给了只接收 `prj_xxx` 或英文 `identifier` 的 `project` 字段 | 禁止原样重试；调用 `workspace_list_projects` 匹配项目英文 `identifier` 后再调用 |
| `ai.project_scope_mismatch` | 当前会话/令牌被锁定在单项目沙箱，无权跨项目或访问全局接口 | 禁止换参绕过；明确告知用户需在未绑定单项目的全局会话或扩大令牌项目范围后执行 |
| 移除或降级成员被拒绝（唯一 Owner） | 违反项目至少保留 1 名 `owner` 的完整性约束 | 先调 `workspace_list_members` 与用户确认继任 `owner`，完成提权后再执行原变更 |
| 切换 `ingressMode: "dependencies"` 后同项目服务调不通 | 未显式创建服务间 `ServiceBinding`，入站流量被 NetworkPolicy 拦截 | 加载 `gateway-topology` 规程，为调用双方补齐 `network_save_service_binding` |
| `ai.preview_conflict` | 预检后项目成员、网络白名单或共享状态已被他人并发修改 | 禁止复用旧 `previewLock`；重新读取最新状态并重新执行 `preview_tool` 让用户确认新 Diff |
