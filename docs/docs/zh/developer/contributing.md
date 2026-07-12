# 参与开发前先看这里

## 开始修改之前

- 先读现有代码和文档，再修改。
- 不主动提交、推送或切分支，除非用户明确要求。
- 小任务一轮推进，完成后留下可追溯记录。
- 编写新功能或修改现有流程时，同步更新 `docs/` 文档站。
- 影响计划、验收或状态时，同步更新 `TODO.md`。

## 前端

- 包管理器使用 `pnpm`。
- 基础 UI 优先使用 shadcn/ui。
- 表单使用 React Hook Form + Zod。
- 用户可见文本必须走 i18n。
- 列表优先使用统一列表组件。
- 状态展示使用语义化 Badge。

## 后端

- PostgreSQL，不使用 SQLite。
- API 启动时会执行内嵌的 `migrations/*.up.sql`；已有但没有 `schema_migrations` 的旧库会先接入到 008，再继续执行后续迁移。
- 运行中的 API 会在 `/openapi.yaml` 提供内置 OpenAPI 文档，并在 `/swagger` 提供 Swagger UI。
- Secret 和 Token 不明文落业务表。
- 外部平台能力由后端 provider/service/API 适配，前端不编排第三方平台 API。
- 长耗时任务进入 worker，不在 HTTP 请求中同步执行。

## 怎样验证改动

小改动只跑与它直接相关的检查。改动跨越多个业务域，或者涉及认证、权限、Secret、数据库迁移和部署运行时，就要执行完整验证，并尽量在浏览器里走一遍真实交互。

后端开发和发布检查必须使用精确的 Go `1.26.5`；版本同时记录在 `.go-version`、`go.mod` 和 Dockerfile builder 镜像中。发布候选需要在干净 Git 工作区执行：

```bash
./scripts/release-check.sh
```

发布质量门禁会校验 Go 版本和 `gofmt`，执行全量 Go 测试、`go vet`、关键包 race test、前端测试/lint/build、文档构建、pnpm 高危依赖审计、`govulncheck`，并 lint/render Helm Chart。任一项失败都不能继续发布；脚本也会拒绝在有未提交或未跟踪文件的工作区运行，避免验证结果和待发布源码不一致。

## 文档体验

文档的作用是让用户少走弯路。动笔时先回答：

- 用户现在想完成什么。
- 最短路径是什么。
- 成功后应该看到什么。
- 失败时先看哪里。

内部架构和边界可以放在开发文档里，不要挡在用户开始使用之前。
