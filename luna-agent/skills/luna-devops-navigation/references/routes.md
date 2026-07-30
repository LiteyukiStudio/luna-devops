# 已注册的 Luna DevOps 路由

## 顶级页面

| 目标 | 路由 |
| --- | --- |
| 看板 | `/dashboard` |
| 项目空间 | `/projects` |
| 事件 | `/events` |
| 代码仓库 | `/code-repositories` |
| 镜像站 | `/registries` |
| 运行时集群 | `/clusters` |
| 应用市场 | `/app-templates` |
| 账单 | `/billing` |
| 账号 | `/settings/account` |
| 身份认证 Provider | `/settings/auth-providers` |
| 通知 | `/settings/notifications` |
| 运营面板 | `/settings/operations` |
| 全局设置 | `/settings/site` |
| 用户 | `/settings/users` |

## 事件页筛选

事件页支持通过查询参数初始化筛选。用户明确要求打开某类事件、某种结果或特定资源的事件时，应直接把可信筛选条件放入 `navigate_to_route`，不要先跳到无筛选的 `/events` 后让用户手动操作。

- 分类：`categories=build|release|hook|gateway|certificate|security|service_binding|other`
- 结果：`statuses=in_progress|succeeded|failed|canceled`
- 级别：`severities=info|warning|error`
- 事件类型：`types=:eventType`
- 项目空间：`projectIds=:projectId`
- 应用：`applicationIds=:applicationId`
- 部署配置：`deploymentTargetIds=:deploymentTargetId`

同一字段可重复出现，也可使用逗号分隔多个值。资源 ID 和事件类型必须来自页面上下文、用户明确输入或工具结果。

示例：

- 正在构建：`/events?categories=build&statuses=in_progress`
- 构建失败：`/events?categories=build&statuses=failed`
- 某项目空间的错误事件：`/events?projectIds=:projectId&severities=error`

## 资源路由

- 项目空间：`/projects/:projectId`
- 项目空间 Tab：`/projects/:projectId?tab=:tabId`
- 应用：`/projects/:projectId/apps/:applicationId`
- 应用 Tab：`/projects/:projectId/apps/:applicationId?tab=:tabId`
- 构建运行：`/projects/:projectId/apps/:applicationId?tab=builds#tab=builds&buildRunId=:buildRunId`

项目空间 Tab ID：`overview`、`apps`、`members`、`build-variables`、`runtime-configs`、`hooks`、`topology`。

应用 Tab ID：`overview`、`repositories`、`builds`、`deployments`、`gateway`、`topology`、`settings`。

只能使用可信 ID 替换占位符。对每个路径段、查询参数和 Hash 值进行 URL 编码；不得编码 `/` 分隔符。
