---
name: diagnostics-governance
description: 跨领域故障排障漏斗、全局资源搜索与平台事件流、个人账单钱包、站内信待办及个人与项目通知工作流。
---

# 跨领域诊断与日常运营工作流

## 适用范围

处理跨领域故障排障、全局搜索与事件流（`ops_search_global`、`ops_list_events`）、个人账单与站内信（`ops_get_billing_summary`、`ops_list_billing_records`、`ops_list_inbox`、`ops_mark_inbox_read`、`ops_archive_inbox_message`、`ops_decide_inbox_action`）、个人与项目通知（`ops_get_my_notifications`、`ops_list_project_notifications`、`ops_save_my_notification_channel`、`ops_delete_my_notification_channel`、`ops_update_my_notification_preferences`、`ops_test_or_retry_my_notification`、`ops_save_project_notification_channel`、`ops_save_project_notification_rule`、`ops_delete_project_notification_resource`、`ops_test_or_retry_project_notification`）及受控公开网页检索（`ops_web_search`、`ops_web_fetch`）。平台管理员全局治理工具请加载 `platform-admin`。

## 标准工作流

1. **先聚合概览、再精准下钻（禁止碎片化盲查）**：
   - 项目级排障先调用 `workspace_get_project`，一次获取项目资源计数、当前角色权限及近 7 天失败/进行中的构建、发布与异常网关入口诊断摘要。
   - 定位到具体异常部署环境后，立即调用 `runtime_get_target_diagnostics`，一站式获取工作负载实时副本、Pod CPU/内存实时指标、最近发布、异常 Pod 重启/等待原因、Warning 事件与绑定路由状态。
   - 找到交付链上**第一个异常边界**（代码源 → 构建 → 镜像 → 发布 → 运行副本 → 网关路由）后，仅对该异常节点调用 `build_get_run`、`delivery_get_release` 或 `runtime_get_pod_logs` 读取尾部日志。
2. **事实、推断与最小化修复分离**：
   - 关联同一时间窗口内的状态、`failureCode`、`observationCode` 与 `ops_list_events` 事件记录，明确区分“已观测事实”与“根因推断”。
   - 给出最小可回滚增量修复动作（严禁为修一个变量重传整套配置），执行修复并发布后沿原诊断入口复验终态。
3. **个人账单、站内信审批与通知排障**：
   - AI 模型调用费用（`ai.*`）只归属发起用户个人钱包（`project_id` 为空），构建、运行、存储与网关流量归属对应项目计费主体；查询本人余额与流水使用 `ops_get_billing_summary` / `ops_list_billing_records`。
   - 处理跨项目网络共享或计费转移待办时，先用 `ops_list_inbox` 核对消息详情，再调用 `ops_decide_inbox_action` 决策。
   - 排查通知未送达时依次核对通知偏好、渠道绑定、规则事件过滤条件与最近投递记录，按需调用 `ops_test_or_retry_my_notification` 或 `ops_test_or_retry_project_notification` 验证。
