---
name: application-deployment
description: 应用管理、应用市场模板安装、部署目标规格/配置/卷挂载、扩缩容、发布与回滚工作流。
---

# 应用、部署与发布工作流

## 适用范围

处理应用 CRUD（`delivery_list_applications`、`delivery_get_application`、`delivery_create_application`、`delivery_update_application`、`delivery_delete_application`）、应用市场模板安装（`delivery_list_templates`、`delivery_get_template`、`delivery_install_template`）、共享运行配置集（`delivery_list_config_sets`、`delivery_get_config_set`、`delivery_create_config_set`、`delivery_configure_config_set`、`delivery_delete_config_set`）、部署目标增量配置（`delivery_get_target`、`delivery_create_target`、`delivery_update_target_runtime`、`delivery_configure_target_config`、`delivery_configure_target_volumes`、`delivery_delete_target`）及副本扩缩容、发布、回滚、重启与取消发布（`delivery_scale_target`、`delivery_deploy_target`、`delivery_rollback_target`、`delivery_restart_target`、`delivery_cancel_release`）。

## 标准工作流

1. **扁平双模资源定位**：
   - `project` 支持项目主键 ID（`prj_xxx`）或唯一英文 `identifier`；`application` 支持应用 ID（`app_xxx`）或同项目唯一 `identifier`；`target` 支持 `targetId`（`dplt_xxx`）、`stage`（`dev | test | staging | prod`）或同应用唯一 `name`。
   - 修改部署目标前先调用 `delivery_get_target` 获取当前期望配置、已配置密钥键名列表 `secretKeys` 及实时副本状态（后续合并发版前无需重复调用 `delivery_get_target` 反复确认同一份草稿）；`cluster` 省略代表使用平台默认集群，仅在存在多个候选集群且无法消歧时向用户确认。
2. **应用市场两阶段安装（`L2`）**：
   - 先调用 `delivery_list_templates` 按关键词或分类检索摘要，再用 `delivery_get_template` 读取普通参数 `values`、敏感参数 `secretValues` 与数据卷要求。
   - 执行 `delivery_install_template` 时：单组件模板使用顶层 `projectVolume`，多组件模板使用 `componentVolumes` 按组件 `key` 绑定独立数据卷。
3. **纯增量补丁与密钥物理隔离**：
   - 所有配置修改工具均为纯增量 Patch（未传字段原样保留，绝不全量覆盖）：普通环境变量与配置文件走 `setPublicEnv / removeEnvKeys / upsertConfigFiles / removeConfigFilePaths`，运行时密钥走 `secretActions`（`set | generate | clear`），绝不在普通环境变量中写入明文密钥。
   - 若需要切换目标为 `internalOnly: true`（仅集群内网访问），必须先检查并清理绑定在该目标上的对外网关路由，避免互斥冲突。
   - 多个连续配置变更（如修改环境变量、挂载数据卷、建立 `ServiceBinding`）均返回 `requiresRelease: true` 时，应在所有期望配置落库后**合并触发一次 `delivery_deploy_target`**，避免冗余多次滚动发版。
4. **停机、扩缩容、发布与高级字段边界**：
   - 调整在线副本数或停机缩容至 `0` 使用独立工具 `delivery_scale_target`（开启 HPA 自动扩缩容时不可手动改固定副本）；原地滚动重启使用 `delivery_restart_target`。
   - `delivery_deploy_target` 或 `delivery_rollback_target` 返回 `releaseId` 仅代表发布任务已入队，只有权威终态与实时工作负载就绪才代表发布成功。
   - 同应用跨环境克隆优先使用 `delivery_create_target(cloneFromStage)`；涉及 `initContainers`、`sidecarContainers`、`tolerations`、`affinity` 等高级 Kubernetes 字段或跨项目 JSON Bundle 导入导出时，引导用户在 Web 控制台部署编辑页完成。
