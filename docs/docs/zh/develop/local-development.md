# 本地开发指南

这套本地环境适合修改代码、调试接口和验证前端交互。如果只是想试用平台，请直接使用“开始”里的 Docker Compose 部署。

## 推荐拓扑

日常开发可以按下面的方式分工：

- PostgreSQL、Redis 和 worker 用 `docker-compose-dev.yaml`。
- API 在宿主机运行，方便调试 Go 代码。
- Web 在宿主机运行，享受 Vite 热更新。

```bash
docker compose -f docker-compose-dev.yaml up -d --build
go run ./cmd/api
pnpm --dir web install
pnpm --dir web dev
```

## 后端入口

- `cmd/api`：HTTP API、Webhook、OAuth 回调、权限入口和任务投递。
- `cmd/worker`：构建、部署、状态同步、证书申请和资源清理等异步任务。
- `internal/api`：HTTP handler 和响应模型。
- `internal/authz`：平台角色、项目空间角色、权限 action 和 Access Token scope 的集中授权规则。
- `internal/model`：GORM 数据模型。
- `internal/provider`：Git、Registry、Kubernetes、DNS 等外部平台适配。
- `internal/worker`：异步任务运行器。

Handler 负责接收参数、进入权限检查并返回响应。业务规则放到 service，数据库访问放到 repository，Git、Registry 和 Kubernetes 等外部调用放到 provider。新增权限时先复用 `internal/authz` 的 action 和角色矩阵，不要在各个 Handler 里重复判断角色。

## 前端入口

- `web/src/pages`：页面级模块。
- `web/src/components/ui`：shadcn/ui 基础组件。
- `web/src/components/common`：跨页面业务组件。
- `web/src/api`：API client 和 DTO 类型。
- `web/src/i18n`：中英文文案。

共享模块必须使用 `@/` 根目录导入。用户可见文案必须进入 i18n，不要直接写死在组件里。

生产镜像会把前端构建产物嵌入 API。`index.html` 使用可重验证缓存，Vite `assets/` 产物使用一年强缓存和 `immutable`，非 hash 的公开资源使用短缓存。

## 文档站

`docs/` 是 Rspress 文档站。新增功能、改动流程或调整用户体验时，需要同步更新这里的用户文档。
