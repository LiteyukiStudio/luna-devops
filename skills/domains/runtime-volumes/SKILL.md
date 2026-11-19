---
name: runtime-volumes
description: 运行集群状态与治理、目标一站式诊断/Pod/事件/日志、单次非交互命令执行、数据卷与生命周期脚本工作流。
---

# 运行时与数据卷工作流

## 适用范围

处理运行集群观察与治理（`runtime_list_clusters`、`runtime_inspect_cluster_governance`、`runtime_save_cluster`、`runtime_test_cluster`、`runtime_delete_cluster`、`runtime_save_cluster_egress_exception`、`runtime_delete_cluster_egress_exception`、`runtime_approve_cluster_network_share`、`runtime_delete_cluster_k8s_resource`）、工作负载诊断与日志命令（`runtime_get_target_diagnostics`、`runtime_list_target_pods_and_events`、`runtime_get_pod_logs`、`runtime_exec_command`）、持久化数据卷（`runtime_list_volumes`、`runtime_get_volume`、`runtime_create_volume`、`runtime_update_volume`、`runtime_retry_volume_operation`、`runtime_delete_volume`、`runtime_create_volume_export`、`runtime_cancel_volume_transfer`）及项目生命周期脚本（`runtime_list_hooks`、`runtime_list_hook_runs`、`runtime_save_hook`、`runtime_delete_hook`）。

## 标准工作流

1. **实时状态权威观察**：
   - 集群、副本数、Pod、PVC 与监控指标均以实时 Kubernetes 观测结果为准。当返回 `status: unavailable` 与稳定 `observationCode` 时，如实说明上游集群或指标服务不可达，绝不伪装成“0 副本”或“无异常”。
2. **容器排障读取顺序**：
   - 首选调用 `runtime_get_target_diagnostics`，一站式获取工作负载实时副本、Pod CPU/内存实时指标、最近发布、异常 Pod 重启/等待原因、Warning 事件与网关入口状态，避免盲目遍历集群或历史构建。
   - 需要查看完整 Pod 列表或标准输出时，再按需调用 `runtime_list_target_pods_and_events` 或 `runtime_get_pod_logs`（`CrashLoopBackOff` 优先读取崩溃退出容器 `previous: true` 日志）。
   - 仅当结构化状态、事件与日志仍不足以定位根因，且项目与环境已允许运行终端访问时，才申请单次非交互 `runtime_exec_command`（`L2`）；需要交互式 TTY Shell 时引导用户在 Web 控制台打开容器终端。
3. **持久化数据卷与传输边界**：
   - 新建数据卷前可通过 `runtime_list_volumes` 或 `runtime_list_clusters` 核对目标集群可用 StorageClass（如 `WaitForFirstConsumer` 存储类会在首次挂载发布 Pod 调度后进入 `Bound`）。
   - 扩容、重试或删除数据卷前先调用 `runtime_get_volume` 核对当前状态、挂载关系与 `revision`。
   - AI 可创建归档导出任务（`runtime_create_volume_export`）或取消传输（`runtime_cancel_volume_transfer`）；本地归档上传导入与已就绪导出包的浏览器下载，请引导用户在 Web 控制台项目空间“数据卷”页签完成。
