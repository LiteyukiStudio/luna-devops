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

### Step 1 — 一站式诊断快照（首选锚点，识别新旧两代并存）
调用 `runtime_get_target_diagnostics`（传入稳定锚点 `project` + `application` + `target`），一次获取：
- 当前期望副本 vs 实时就绪副本、`recentReleases` 最新发版状态；
- 异常 Pod 的重启次数（`restartCount`）、等待/退出原因（如 `OOMKilled`、`CrashLoopBackOff`、`ImagePullBackOff`、`Init:Error`）；
- Pod CPU / 内存实时使用指标、近期 `Warning` 事件、数据卷 PVC 相位及关联网关路由健康摘要；
- **新旧并存滚动卡死检查（Mixed-Generation Rollout）**：即使 `readyReplicas` 看似达标，也必须同时核对 `recentReleases[0]`——滚动发布卡住时，旧版 Pod 仍 `Ready` 承接流量而新版 Pod 正处于崩溃或拉取失败状态，切勿仅凭 `readyReplicas > 0` 误判新版本已正常上线。

### Step 2 — 容器日志精准读取（防滚动漂移 + 识别多容器/InitContainer）
- **优先锚点选流，特定发版绑定 `releaseId`**：调用 `runtime_get_pod_logs` 默认开启 `preferAbnormal: true`，常规排障**只需传 `project` + `application` + `target`（省略易失的 `pod` 名）**即可免疫滚动替换报错；若新旧两代 Pod 并存且老 Pod 历史 `restartCount` 更高，传入最新 `recentReleases[0].id` 作为 `releaseId` 以精准锚定新一代实例。
- **`InitContainer` 与 `Sidecar` 显式传 `container`**：省略 `container` 时服务端默认选取主业务容器（`Containers[0]`）。若 Pod 卡在 `Init:Error` / `Init:CrashLoopBackOff`（主容器未启动，调默认日志报 `runtime.container_not_started`），或 Sidecar 容器崩溃而主容器正常，必须从 Pod 状态/事件中确认异常容器名并**显式传入 `container` 参数**读取该初始化或伴生容器日志。
- **崩溃重启必读前序容器日志**：若目标容器 `CrashLoopBackOff` 或 `restartCount > 0`，传入 `previous: true` 读取上次崩溃退出的尾部日志（若返回 `runtime.previous_log_not_found`，说明容器从未完成首次退出，立即改为 `previous: false`，**不要重复重试 `previous: true`**）。
- **并发滚动更新重对齐（Re-grounding Gate）**：若因显式传了 `pod` 名收到 `runtime.pod_not_found`，**原地重试次数强制为 `0`**，立即重调 `runtime_get_target_diagnostics` 核实新一代 Pod 是否已健康 `Ready`（若新发版已覆盖旧故障恢复正常，直接汇报结案）。

### Step 3 — 受控单次容器命令（最后手段，`L2`）
- 仅当诊断快照、事件与日志均无法定位根因，且项目与目标环境已开启控制台执行权限时，才可申请调用 `runtime_exec_command`（通过 `/bin/sh -lc` 执行单次非交互命令，30s 超时；注意 `scratch` / `distroless` 无 Shell 镜像不支持，若报 `runtime.web_console_disabled` 禁止重试）。
- 需要交互式长连接 TTY Shell 调试时，明确引导用户在 Web 控制台部署详情页打开「容器终端」。

---

## 3. 持久化数据卷（PVC）生命周期与传输边界

1. **创建与延迟绑定假象（`WaitForFirstConsumer`）**：
   - 调用 `runtime_create_volume` 前可通过 `runtime_list_volumes`（附带 `cluster`）查看目标集群可用 `storageClasses`。
   - **常见异步假象**：多数云盘/本地存储类采用 `WaitForFirstConsumer` 绑定模式——新建数据卷后在尚未挂载到部署目标并完成首次 Pod 调度前，状态停留在 `Pending` / `provisioning` **属于完全正常的等待首次消费状态，并非创建失败**！此时应继续完成 `delivery_configure_target_volumes` 挂载并执行 `delivery_deploy_target` 触发调度。
2. **访问模式约束、扩容与删除前置检查**：
   - `ReadWriteOnce`（`RWO`）数据卷仅允许单节点读写挂载：挂载在普通多副本 `Deployment` 上扩容跨节点调度会触发 `Multi-Attach error` 阻塞新 Pod 启动。
   - 执行 `runtime_update_volume`（仅托管卷支持只增不减在线扩容）、`runtime_retry_volume_operation` 或 `runtime_delete_volume` 前，必须先调 `runtime_get_volume` 核对当前状态、活跃挂载列表与最新 `revision`（已被目标挂载的数据卷须先解绑并发版释放后才可删除）。
3. **归档导出 `ready` 交接终态（防假死轮询）**：
   - AI 调用 `runtime_create_volume_export` 创建异步导出任务后，通过 `runtime_get_volume(project, transferId)` 查询进度：当传输 `state` 变为 **`ready`** 时，代表归档文件已打包完毕并等待用户在浏览器点击下载（只有人类下载流结束后才会变为 `succeeded`，超时未下变为 `expired`）。**观测到 `state: "ready"` 即刻引导用户前往控制台「数据卷」页签下载收口，严禁在无人下载时轮询死等 `succeeded`**！

---

## 4. 集群治理与项目生命周期 Hook

- **生命周期脚本（Hook）**：维护构建前/发布前后脚本使用 `runtime_save_hook` / `runtime_delete_hook`；排查某次发布前置迁移脚本（如数据库 Migration）失败时，调用 `runtime_list_hook_runs` 读取退出码与执行输出。
- **集群级管理**：注册/更新集群（`runtime_save_cluster`）、连通性探活（`runtime_test_cluster`）、集群受限网段例外（`runtime_save_cluster_egress_exception`）与孤儿 K8s 资源清理（`runtime_delete_cluster_k8s_resource`）均属高影响 `L2` 操作，执行前务必先通过 `runtime_inspect_cluster_governance` 核实配额与影响项目面。

---

## 5. 状态漂移、异步假象与错误恢复速查表

| 现象或错误码 | 根因 | 标准恢复动作（含禁止行为） |
| --- | --- | --- |
| `runtime.pod_not_found` | 排查途中发生了滚动发版、缩容或驱逐，所持旧 `pod` 快照名已消亡 | **禁止重试该 `pod` 名**；重调 `runtime_get_target_diagnostics` 对齐当前最新 Release 与 Pod，或省略 `pod` 参数调 `runtime_get_pod_logs` |
| 新发版后用户反馈未生效，或抓到了旧版 Pod 日志 | 滚动更新卡住导致新旧两代 Pod 并存，老 Pod 因高 `restartCount` 被优先选中 | 比对 `recentReleases[0]` 状态，并在 `runtime_get_pod_logs` / `runtime_list_target_pods_and_events` 中显式传入新版 `releaseId` |
| `runtime.container_not_started` | 主容器未启动（Pod 处于 `Pending`、`ImagePullBackOff` 或 `InitContainer` 报错阻塞） | **禁止对同一未启动主容器刷日志**；若为 `Init:*` 失败，将失败的 `initContainer` 名传入 `container` 参数读取其日志，其余看挂载/拉取镜像事件 |
| `runtime.previous_log_not_found` | 所选容器尚无已终止的前序退出实例 | 禁止重复带 `previous: true` 重试；改为 `previous: false` 读取当前日志，或看 `runtime_list_target_pods_and_events` 容器等待事件 |
| 新建数据卷 `Pending` / 导出任务停在 `state: "ready"` | `WaitForFirstConsumer` 等待首次挂载调度；导出 `ready` 等待人类在浏览器下载 | 新卷继续挂载并发版（不要删卷重建）；导出见 `ready` **立即停止轮询**并通知用户去控制台下载（不要死等 `succeeded`） |
| 删除/缩容数据卷被拒，或扩容后报 `Multi-Attach error` | PVC 不支持缩容、仍绑定目标，或 `RWO` 单节点卷被多副本跨节点挂载 | 删除前先通过 `delivery_configure_target_volumes` 解绑并发版；`RWO` 卷调回单副本或改用 `RWX` / `StatefulSet` |
