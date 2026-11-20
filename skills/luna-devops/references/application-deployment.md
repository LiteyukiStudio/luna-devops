---
name: application-deployment
description: 管理应用生命周期、从零上线新服务闭环、应用市场模板两阶段安装、共享配置集、部署目标规格/环境变量/加密密钥/数据卷挂载纯增量变更、副本扩缩容、合并发布与版本回滚。当用户新建或上线应用、修改环境变量或 Secret、安装数据库/中间件模板、调整资源规格或执行发版/扩缩容/回滚时加载本规程。
---

# 应用、部署配置与发布回滚规程

## 目录
- [1. 核心入口、双模寻址与自由度边界](#1-核心入口双模寻址与自由度边界)
- [2. SOP 1：从零上线新服务跨领域端到端 Checklist](#2-sop-1从零上线新服务跨领域端到端-checklist)
- [3. SOP 2：纯增量配置、密钥隔离与合并发版范式](#3-sop-2纯增量配置密钥隔离与合并发版范式)
- [4. SOP 3：应用市场模板安装、扩缩容与回滚门禁](#4-sop-3应用市场模板安装扩缩容与回滚门禁)
- [5. 状态漂移与错误恢复速查表](#5-状态漂移与错误恢复速查表)

---

## 1. 核心入口、双模寻址与自由度边界

- **高频入口工具**：`delivery_list_applications`、`delivery_get_target`、`delivery_create_application`、`delivery_create_target`、`delivery_update_target_runtime`、`delivery_configure_target_config`、`delivery_configure_target_volumes`、`delivery_deploy_target`、`delivery_scale_target`、`delivery_rollback_target`、`delivery_get_release`。
- **扁平双模寻址**：
  - `project`：项目 ID（`prj_xxx`）或唯一英文 `identifier`；
  - `application`：应用 ID（`app_xxx`）、唯一英文 `identifier` 或唯一名称；
  - `target`：目标 ID（`dplt_xxx`）、环境 `stage`（`dev | test | staging | prod`）或同应用内唯一名称；
  - `cluster`：创建目标或安装模板时省略 `cluster` 即自动选用项目默认集群，仅在多候选集群无法消歧时询问用户。
- **自由度设定**：本领域绝大部分为改变线上工作负载的**低自由度**操作，必须严格遵守“先读现行快照 → 纯增量 Patch → 合并单次发版 → 验证运行终态”序列。

---

## 2. SOP 1：从零上线新服务跨领域端到端 Checklist

当用户要求“把某个仓库或镜像部署上线并能访问”时，跨领域加载 `source-registry-build` 与 `gateway-topology`，严格按以下门禁推进，**严禁跳步**：

- [ ] **Step 1 — 项目与来源核实**：调用 `workspace_get_project` 确认项目标识与集群状态；若为源码部署，调 `build_inspect_repository_code` 确认构建方式与容器实际监听端口（如 `8080`）。
- [ ] **Step 2 — 创建应用与部署目标**：调用 `delivery_create_application` 与 `delivery_create_target`（正确设置 `servicePorts` 对齐容器真实监听端口；若仅内网中间件设 `internalOnly: true`）。
- [ ] **Step 3 — 构建镜像（仅源码模式）**：调用 `build_configure_target_pipeline` 配置流水线并调 `build_trigger_run`。
  - **Gate 1（镜像就绪门禁）**：必须等待构建终态为 `succeeded`（见 `source-registry-build` 有界等待规则）；构建失败立即转入构建日志排障，**严禁在无可用镜像时触发发布**。
- [ ] **Step 4 — 期望配置一次性攒齐**：按需调用 `delivery_configure_target_config`（环境变量/密钥）、`delivery_configure_target_volumes`（挂载数据卷）或 `network_save_service_binding`（绑定上下游依赖）。此时各工具均返回 `requiresRelease: true`，**中间步骤不要逐次发版**。
- [ ] **Step 5 — 合并触发首次发布**：所有期望配置落库后，调用**一次** `delivery_deploy_target` 创建 `Release`。
  - **Gate 2（工作负载与 K8s Service 就绪门禁）**：等待 `delivery_get_release` 成功并调 `runtime_get_target_diagnostics` 确认实时副本 `Ready` 且无 `CrashLoopBackOff`。**未完成首次发布生成 K8s Service 前，严禁提前创建公网 `GatewayRoute`**。
- [ ] **Step 6 — 暴露公网入口（仅对外服务）**：确认目标 `internalOnly: false` 后，按需调 `network_check_gateway_domain` 预检域名，再调 `network_create_gateway_route` 绑定端口与 TLS 模式，最后回读一次路由状态结案。

---

## 3. SOP 2：纯增量配置、密钥隔离与合并发版范式

1. **改前单次快照**：修改前调用一次 `delivery_get_target` 核对现行 `publicEnv`、`secretKeys`（仅键名不回显值）、`servicePorts` 与挂载列表；连续执行多步 Patch 后无需在发版前重复调用 `delivery_get_target` 反复确认同一份草稿。
2. **增量补丁与密钥物理隔离（`delivery_configure_target_config`，`L2`）**：
   - 未传入的变量、密钥、配置文件与配置集**全部原样保留**；删除普通变量显式传 `removePublicEnvKeys`（不要传空字符串冒充删除）。
   - 密码、API Key、JWT Secret 等敏感值**严禁放入 `setPublicEnv`**，一律走 `secretActions`：支持服务端随机生成（如内部中间件连接密码、签名密钥）的场景优先用 `operation: "generate"`。

```json
{
  "project": "core-backend",
  "application": "user-api",
  "target": "prod",
  "setPublicEnv": [{ "key": "LOG_LEVEL", "value": "info" }],
  "removePublicEnvKeys": ["LEGACY_FLAG"],
  "secretActions": [
    { "key": "SESSION_SECRET", "operation": "generate", "encoding": "hex", "length": 32 },
    { "key": "OLD_TOKEN", "operation": "clear" }
  ]
}
```

3. **切换内网专属（`internalOnly: true`）的前置清理门禁**：
   - 若要通过 `delivery_update_target_runtime` 将目标改为 `internalOnly: true`，必须先调 `network_list_gateway_routes` 检查并删除绑定在该目标上的所有公网网关路由，否则因安全互斥被服务端拒绝。

---

## 4. SOP 3：应用市场模板安装、扩缩容与回滚门禁

- **应用市场两阶段安装（`L2`）**：
  1. 先调 `delivery_list_templates` 检索模板，**必须再调 `delivery_get_template`** 查看参数表（区分普通参数 `values` 与敏感参数 `secretValues`、哪些支持 `autoGenerate`）及组件卷要求。
  2. 调用 `delivery_install_template` 时：**单组件模板**挂载已有卷使用顶层 `projectVolume`；**多组件模板**使用 `componentVolumes` 按组件 `key` 分别映射独立数据卷（二者互斥）。
- **扩缩容、停机与重启分流**：
  - 调整在线副本数或停机缩容至 `0` 使用 `delivery_scale_target`（**注意**：目标已开启 HPA 自动扩缩容时禁止手动设置固定副本，需先调整自动扩缩容策略）；
  - 不改配置仅滚动重建 Pod 使用 `delivery_restart_target`；
  - 同应用跨阶段复制配置优先用 `delivery_create_target` 的 `cloneFromStage`；涉及 `initContainers`、`sidecarContainers`、`tolerations`、`affinity` 或跨项目 Bundle 导入导出时，引导用户在 Web 控制台完成。
- **生产回滚闭环**：调用 `delivery_get_target` 或发布记录确认历史稳定可回滚版本后执行 `delivery_rollback_target`，并必须跟进验证实时 Pod 恢复 `Ready`。

---

## 5. 状态漂移与错误恢复速查表

| 现象或错误码 | 根因 | 标准恢复动作（含禁止行为） |
| --- | --- | --- |
| 修改了环境变量/密钥但线上容器内未生效 | 配置工具只更新数据库期望草稿（返回 `requiresRelease: true`），尚未创建新发布 | 禁止重复提交相同配置；确认全部草稿改动齐备后执行一次 `delivery_deploy_target` |
| `delivery_scale_target` 被拒绝（HPA 已启用） | 开启自动扩缩容时固定副本字段由 HPA 接管 | 禁止强行重试固定扩缩容；先调 `delivery_get_target` 查看 HPA 配置，按用户意图修改扩缩容上下限或关闭 HPA |
| 切换 `internalOnly: true` 报存在关联路由冲突 | 目标仍绑定对外暴露的 `GatewayRoute` | 先调 `network_list_gateway_routes` 定位并经确认删除关联路由，再更新 `internalOnly: true` |
| `delivery_install_template` 报 `app_template.value_required` 或参数分类错误 | 必填参数缺失，或把 `secret=true` 参数塞进了明文 `values`（反之亦然） | 调用 `delivery_get_template` 对照每个参数的 `secret` 与 `required` 标记，拆分至 `values` 与 `secretValues` |
| `ai.preview_conflict` | 审批期间目标配置、镜像或最新 Release 已被他人并发更改 | 禁止重放旧锁；调用 `delivery_get_target` 核对最新草稿后重新 `preview_tool` |
