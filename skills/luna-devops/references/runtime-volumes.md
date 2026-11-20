---
name: runtime-volumes
description: 管理 Kubernetes 运行集群状态与治理、部署目标一站式实时诊断、Pod/事件/容器尾部日志读取、受控单次非交互容器命令、持久化数据卷（PVC）生命周期与导入导出传输以及项目生命周期脚本（Hook）。当用户查看容器运行状态与资源指标、抓取崩溃日志、管理集群或数据卷扩容挂载时加载本规程。
---

# Kubernetes 运行时、数据卷与生命周期脚本规程

## 目录
- [1. 核心入口与实时观测权威口径](#1-核心入口与实时观测权威口径)
- [2. 容器排障顺序与防旧 Pod 漂移 SOP](#2-容器排障顺序与防旧-pod-漂移-sop)
- [3. 持久化数据卷（PVC）生命周期与传输边界](#3-持久化数据卷pvc生命周期与传输边界)
- [4. 集群治理与项目生命周期 Hook](#4-集群治理与项目生命周期-hook)
- [5. 状态漂移、异步假象与错误恢复速查表](#5-状态漂移异步假象与错误恢复速查表)

---

## 1. 核心入口与实时观测权威口径

- **高频入口工具**：`runtime_get_target_diagnostics`、`runtime_list_target_pods_and_events`、`runtime_get_pod_logs`、`runtime_exec_command`、`runtime_list_clusters`、`runtime_list_volumes`、`runtime_get_volume`、`runtime_create_volume`、`runtime_update_volume`、`runtime_list_hooks`、`runtime_list_hook_runs`。
- **实时观测权威口径**：
  - 集群连通性、工作负载在线副本、Pod 阶段、CPU/内存实时指标及 PVC 绑定状态均在请求时直读 Kubernetes 与监控上游，响应不走缓存。
  - 当返回 `status: unavailable` 与稳定 `observationCode` 时，代表目标集群 API 或指标组件暂不可达——**必须原样如实告知不可达原因，严禁将其解读为“当前运行 0 副本”或“无异常事件”**。

---

## 2. 容器排障顺序与防旧 Pod 漂移 SOP

排查工作负载异常时严格按由粗到细的三级顺序推进，**严禁一上来盲目遍历全集群或抓取全量日志**：

### Step 1 — 一站式诊断快照（首选锚点）
调用 `runtime_get_target_diagnostics`（传入稳定锚点 `project` + `application` + `target`），一次获取：
- 当前期望副本 vs 实时就绪副本、最新 `Release` 状态；
- 异常 Pod 的重启次数（`restartCount`）、等待/退出原因（如 `OOMKilled`、`CrashLoopBackOff`、`ImagePullBackOff`）；
- Pod CPU / 内存实时使用指标、近期 `Warning` 事件以及关联网关路由健康摘要。

### Step 2 — 容器日志精准读取（优先锚点选流，防滚动更新漂移）
- **优先省略 `pod` 参数**：调用 `runtime_get_pod_logs` 时默认开启 `preferAbnormal: true`，**只需传 `project` + `application` + `target`（不硬编码具体 `pod` 名）**，服务端会自动定位当前异常或最新一代 Pod，天然免疫滚动更新期间旧 Pod 消亡导致的报错。
- **崩溃重启必读前序容器日志**：若诊断快照显示 `CrashLoopBackOff` 或 `restartCount > 0`，新拉起的容器可能刚启动尚无报错输出，此时传入 `previous: true` 读取上一次崩溃退出的尾部日志（若返回 `runtime.previous_log_not_found`，说明容器从未成功完成首次退出，立即去掉 `previous` 改读当前日志或查事件，**不要重试 `previous: true`**）。
- **并发滚动更新重对齐（Re-grounding Gate）**：若因显式指定了某个 `pod` 名而收到 `runtime.pod_not_found`，说明排查期间发生了滚动发布、HPA 缩容或节点驱逐，旧 Pod 已销毁——**原地重试次数强制为 `0`**，立即重调 `runtime_get_target_diagnostics` 检查新一代 Pod 是否已健康 `Ready`（若新版本已恢复正常，直接汇报结案）。

### Step 3 — 受控单次容器命令（最后手段，`L2`）
- 仅当诊断快照、事件与日志均无法定位根因，且项目与目标环境已开启控制台执行权限时，才可申请调用 `runtime_exec_command`（非交互单次命令，有超时与输出截断限制，遵循 `L2` 预检锁）。
- 需要交互式长连接 TTY Shell 调试时，明确引导用户在 Web 控制台部署详情页打开「容器终端」。

---

## 3. 持久化数据卷（PVC）生命周期与挂载边界

1. **创建与延迟绑定假象（`WaitForFirstConsumer`）**：
   - 调用 `runtime_create_volume` 前可通过 `runtime_list_clusters` 或存量卷核对目标集群可用 `storageClass`。
   - **常见异步假象**：多数云盘/本地存储类采用 `WaitForFirstConsumer` 绑定模式——新建数据卷后在尚未挂载到部署目标并完成首次 Pod 调度前，状态停留在 `Pending` **属于完全正常的等待首次消费状态，并非创建失败**！此时应继续完成 `delivery_configure_target_volumes` 挂载并执行 `delivery_deploy_target` 触发调度。
2. **扩容、重试与删除前置检查**：
   - 执行 `runtime_update_volume`（K8s PVC 仅支持只增不减扩容）、`runtime_retry_volume_operation` 或 `runtime_delete_volume` 前，必须先调 `runtime_get_volume` 核对当前状态、是否仍被部署目标挂载以及最新 `revision`（已被目标挂载的数据卷须先解绑并发版释放后才可删除）。
3. **归档传输边界**：
   - AI 可调用 `runtime_create_volume_export` 创建异步归档导出任务，或调 `runtime_cancel_volume_transfer` 取消进行中的传输；
   - 本地归档包上传导入、以及导出完成后归档文件的浏览器下载，引导用户在 Web 控制台项目空间 **「数据卷」** 页签操作。

---

## 4. 集群治理与项目生命周期 Hook

- **生命周期脚本（Hook）**：维护构建前/发布前后脚本使用 `runtime_save_hook` / `runtime_delete_hook`；排查某次发布前置迁移脚本（如数据库 Migration）失败时，调用 `runtime_list_hook_runs` 读取退出码与执行输出。
- **集群级管理**：注册/更新集群（`runtime_save_cluster`）、连通性探活（`runtime_test_cluster`）、集群受限网段例外（`runtime_save_cluster_egress_exception`）与孤儿 K8s 资源清理（`runtime_delete_cluster_k8s_resource`）均属高影响 `L2` 操作，执行前务必先通过 `runtime_inspect_cluster_governance` 核实配额与影响项目面。

---

## 5. 状态漂移、异步假象与错误恢复速查表

| 现象或错误码 | 根因 | 标准恢复动作（含禁止行为） |
| --- | --- | --- |
| `runtime.pod_not_found` | 排查途中发生了滚动发版、缩容或驱逐，所持旧 `pod` 快照名已消亡 | **禁止重试该 `pod` 名**；重调 `runtime_get_target_diagnostics` 对齐当前最新 Release 与 Pod，或省略 `pod` 参数调 `runtime_get_pod_logs` |
| `runtime.previous_log_not_found` | 当前容器尚无已终止的前序退出实例 | 禁止重复带 `previous: true` 重试；改为 `previous: false` 读取当前日志，或看 `runtime_list_target_pods_and_events` 容器等待事件 |
| `runtime.container_not_started` | Pod 处于 `Pending` / `Init` / `ImagePullBackOff`，业务容器尚未启动 | 禁止刷日志接口；查看 `runtime_get_target_diagnostics` 中的拉取镜像、挂载卷或 Init 容器事件 |
| 新建数据卷状态一直显示 `Pending` | 存储类为 `WaitForFirstConsumer`，等待首个挂载它的 Pod 调度后才分配物理卷 | **不要当故障删除重建**；将该卷通过 `delivery_configure_target_volumes` 挂载到目标并执行 `delivery_deploy_target` |
| 删除或缩容数据卷被拒绝 | PVC 不支持缩容，或数据卷仍绑定在部署目标上 | 核对 `runtime_get_volume` 的挂载列表；需删除时先通过 `delivery_configure_target_volumes` 的 `removeMounts` 解绑并发版 |
