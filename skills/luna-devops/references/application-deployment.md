---
name: application-deployment
description: 管理应用生命周期、从零上线新服务闭环、应用市场模板两阶段安装、共享配置集、部署目标规格/环境变量/加密密钥/数据卷挂载纯增量变更、副本扩缩容、合并发布与版本回滚。当用户新建或上线应用、修改环境变量或 Secret、安装数据库/中间件模板、调整资源规格或执行发版/扩缩容/回滚时加载本规程。
---

# 应用、部署配置与发布回滚规程

## 目录
- [1. 核心入口、双模寻址与自由度边界](#1-核心入口双模寻址与自由度边界)
- [2. SOP 1：从零上线新服务跨领域端到端 Checklist](#2-sop-1从零上线新服务跨领域端到端-checklist)
- [3. SOP 2：纯增量配置、密钥隔离与合并发版范式](#3-sop-2纯增量配置密钥隔离与合并发版范式)
- [4. SOP 3：模板安装、运行控制与回滚/取消高频陷阱](#4-sop-3模板安装运行控制与回滚取消高频陷阱)
- [5. 状态漂移与错误恢复速查表](#5-状态漂移与错误恢复速查表)

---

## 1. 核心入口、双模寻址与自由度边界

- **高频入口工具**：`delivery_list_applications`、`delivery_get_target`、`delivery_create_application`、`delivery_create_target`、`delivery_update_target_runtime`、`delivery_configure_target_config`、`delivery_configure_config_set`、`delivery_configure_target_volumes`、`delivery_deploy_target`、`delivery_scale_target`、`delivery_restart_target`、`delivery_rollback_target`、`delivery_cancel_release`、`delivery_get_release`。
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

3. **共享运行配置集（`ConfigSet`）绑定模式与冲突门禁**：
   - 调用 `delivery_configure_config_set` 修改项目共享配置集后，以 **`live` 模式**绑定该配置集的部署目标**不会自动热更新线上容器**，需对受影响目标执行 `delivery_deploy_target` 下发；以 **`snapshot` 模式**绑定的目标仍锁定旧快照，重发版前需同步更新目标的 `configSetBindings`。
   - 任何普通/加密变量名不得与活跃 `ServiceBinding` 自动注入变量撞名（否则报 `service_binding.env_conflict`）；明文 `configFiles`、加密 `secretFiles` 与数据卷 `mountPath` 之间严禁同路径或父子目录前缀嵌套（否则报 `deployment.runtime_config_mount_conflict`）。
4. **切换内网专属（`internalOnly: true`）的前置清理门禁**：
   - 若要通过 `delivery_update_target_runtime` 将目标改为 `internalOnly: true`，必须先调 `network_list_gateway_routes` 检查并删除绑定在该目标上的所有公网网关路由，否则因安全互斥被服务端拒绝。

---

## 4. SOP 3：模板安装、运行控制与回滚/取消高频陷阱

- **应用市场两阶段安装（`L2`）**：
  1. 先调 `delivery_list_templates` 检索模板，**必须再调 `delivery_get_template`** 查看参数表（区分普通参数 `values` 与敏感参数 `secretValues`、哪些支持 `autoGenerate`）及组件卷要求。
  2. 调用 `delivery_install_template` 时：**单组件模板**挂载已有卷使用顶层 `projectVolume`；**多组件模板**使用 `componentVolumes` 按组件 `key` 分别映射独立数据卷（二者互斥）。
- **四类运行控制动作严格分流（切勿混用）**：
  - **停机释放算力 / 调整副本（`delivery_scale_target`）**：传 `replicas: 0` 才会真正停止线上 Pod 释放计费算力；而修改 `governance.enabled: false` **仅禁止新建发布，绝不会停止已在运行的 Pod**！注意：目标已启用 HPA，或挂载了 `ReadWriteOnce`（`RWO`）单节点云盘的无状态 `Deployment` 不宜盲目扩至多副本（跨节点调度会触发 `Multi-Attach` 死锁）。
  - **原地滚动重启 vs. 下发草稿（`delivery_restart_target` vs. `delivery_deploy_target`）**：`delivery_restart_target` 仅向集群已有工作负载写入 `restartedAt` 注解重建旧配置 Pod，**绝不会下发数据库中未发布的配置/密钥改动**；要让新配置生效必须调 `delivery_deploy_target`。
  - **浮动镜像 Tag 重拉取**：外部重推同名浮动标签（如 `:latest`）要求更新线上实例时，调用 `delivery_deploy_target` 必须显式传 `forceImagePull: true`，否则节点命中本地缓存会导致“发布成功但仍跑旧代码”。
  - **跨阶段克隆卷隔离（`cloneFromStage`）**：调 `delivery_create_target` 克隆已有环境时，若源环境含持久化卷，必须传 `volumeOverrides` 绑定新环境独占卷或设 `omitProjectVolumes: true`，严禁两套环境争抢同一 `RWO` 数据卷。
- **回滚与取消发布的核心边界（高频生产事故点）**：
  - **`delivery_rollback_target` 只回滚镜像，不回滚配置！** 该工具仅恢复历史成功 Release 的 `imageRef` 与 `buildRunId`，**不会回退当前的环境变量、Secret、ConfigSet、数据卷或端口规格**。若故障是由刚改坏的配置/密钥引起，单调 `delivery_rollback_target` 会带着错误配置重发旧镜像继续崩溃——必须先增量修回正确配置再调 `delivery_deploy_target`。
  - **`delivery_cancel_release` 不等于回滚集群**：取消已进入 `running` 阶段（K8s 清单已下发集群）的发布，仅终止平台侧 Rollout 等待任务，**不会自动将线上 Deployment 退回旧镜像**；取消后必须调 `runtime_get_target_diagnostics` 核实实时 Pod，若新坏实例已下发需补调 `delivery_rollback_target`。

---

## 5. 状态漂移与错误恢复速查表

| 现象或错误码 | 根因 | 标准恢复动作（含禁止行为） |
| --- | --- | --- |
| 修改了配置/配置集，或调了 `delivery_restart_target` 后线上仍未生效 | 配置工具仅改数据库草稿，`delivery_restart_target` 不下发草稿，`ConfigSet` 不热更新运行容器 | 禁止反复提交配置或反复重启；对目标（或 `live` 绑定该配置集的目标）执行一次 `delivery_deploy_target` |
| 执行 `delivery_rollback_target` 后新 Pod 依然报配置错误崩溃 | `delivery_rollback_target` 仅回退镜像引用，保留了当前损坏的环境变量/密钥/规格 | **禁止继续往更早版本盲目回滚**；先调 `delivery_get_target` 定位损坏配置并增量修正，再调 `delivery_deploy_target` |
| 取消 `running` 中的发布后线上仍存在崩溃的新版 Pod | `delivery_cancel_release` 只停止平台等待工作流，不回退已 Apply 到 K8s 的模板 | 调 `runtime_get_target_diagnostics` 核实；若集群已切至问题版本，继续执行 `delivery_rollback_target` 恢复 |
| 重推 `:latest` 等同名 Tag 并 `deploy` 成功，但线上代码未变 | 默认拉取策略命中节点本地旧镜像缓存 | 重新调用 `delivery_deploy_target` 并显式传入 `"forceImagePull": true` |
| `delivery_scale_target` 被拒（HPA 冲突）或扩容后新 Pod 卡 `Multi-Attach` | 开启 HPA 时禁止手动固定副本；`Deployment` 挂载 `RWO` 卷跨节点多副本互斥 | HPA 场景改调 `delivery_update_target_runtime` 调整 `autoScaling`；`RWO` 卷保持单副本或改用 `RWX` / `StatefulSet` |
| `service_binding.env_conflict` 或 `deployment.runtime_config_mount_conflict` | 变量名与出站 `ServiceBinding` 注入键冲突，或挂载文件/卷路径重叠及父子前缀嵌套 | 更换变量名或调整 `ServiceBinding`；确保 `configFiles`、`secretFiles` 与卷 `mountPath` 路径互不重叠嵌套 |
| `delivery_install_template` 报 `app_template.value_required` 或参数分类错误 | 必填项缺失，或把 `secret=true` 参数误放进明文 `values`（反之亦然） | 调用 `delivery_get_template` 对照每个参数的 `secret` 与 `required` 标记，拆分至 `values` 与 `secretValues` |
| `ai.preview_conflict` 或 `billing.insufficient_balance` | 审批期间目标状态被并发修改，或项目计费主体余额不足触发门禁冻结 | `preview_conflict` 重读 `delivery_get_target` 后重新预检；欠费**禁止重试**，引导核查 `ops_get_billing_summary` |
