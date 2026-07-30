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

## 资源路由

- 项目空间：`/projects/:projectId`
- 项目空间 Tab：`/projects/:projectId?tab=:tabId`
- 应用：`/projects/:projectId/apps/:applicationId`
- 应用 Tab：`/projects/:projectId/apps/:applicationId?tab=:tabId`
- 构建运行：`/projects/:projectId/apps/:applicationId?tab=builds#tab=builds&buildRunId=:buildRunId`

项目空间 Tab ID：`overview`、`apps`、`members`、`build-variables`、`runtime-configs`、`hooks`、`topology`。

应用 Tab ID：`overview`、`repositories`、`builds`、`deployments`、`gateway`、`topology`、`settings`。

只能使用可信 ID 替换占位符。对每个路径段、查询参数和 Hash 值进行 URL 编码；不得编码 `/` 分隔符。
