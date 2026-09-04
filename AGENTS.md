# Luna DevOps 工程约束

本文件只保存跨目录硬约束。前端增量规则见 `web/AGENTS.md`，内部文档索引见
`docs-internal/README.md`，代码检查方法见 `docs-internal/代码检查流程.md`。

## 1. 开工与范围

- 修改前阅读 `README.md`、`TODO.md`、`docs-internal/README.md`、`docs-internal/产品概要.md`
  以及任务涉及的代码、契约和文档。
- 先确认工作树，保留用户已有改动；除非用户明确要求，不提交、推送、切换或创建分支。
- 只实现当前要求及真实调用链的必要后果。不要预建未来字段、策略层、兼容层、fallback、插件或缓存。
- 根因是职责堆积、旧模型或重复逻辑时，以有边界的小重构消除根因，不追加特殊 case。
- 分析、审查和诊断默认只读；变更任务也不授权无关清理。

## 2. 跨链路硬约束

- 修改用户行为时，逐层确认 Web、API、Worker、Agent 是否参与；所有参与层的 Schema、OpenAPI、
  API Client、任务载荷、事件协议、错误码、权限、审计、幂等和可观测字段必须同步。
- 至少验证一条真实入口到最终副作用或权威回读的成功链路；有失败、异步或取消路径时验证对应终态。
- 前端可本地化的内容由稳定 code、枚举或状态 key 映射 i18n 文案。后端不返回面向用户的本地化 UI 文案。
- 用户可见品牌固定为 `Luna DevOps`；项目运行标识只使用 `luna-devops`、`luna.devops`、
  `luna-gateway` 或 `luna_devops_`。仓库、文档和镜像地址继续使用真实 Liteyuki Studio 资源。
- GitHub、Gitea、GitLab、Harbor、DockerHub、OIDC、Kubernetes、Traefik、AI Provider 等外部平台
  由后端 provider/service/API 适配；浏览器不直连或编排第三方 API。
- Secret、Token、密码和 Registry Credential 不明文落业务表、不回显、不进入 URL、日志、遥测或测试产物。

## 3. 实时状态与可观测

- 数据库只保存期望配置、资源引用、工作流结果和不可变历史。健康度、当前数量和实时指标在请求时读取
  Kubernetes 或对应外部平台；不可达时返回 `unavailable` 和稳定 `observationCode`。
- 实时响应使用 `Cache-Control: no-store`；前端使用公共实时查询策略，不以缓存冒充当前事实。
- HTTP、SSE、WebSocket、数据库、Redis、异步任务、外部 Provider、模型和工具调用必须处于有效
  Trace Context；业务调用链继续传递 `context.Context` 或 W3C trace headers，不能用
  `context.Background()` 截断父链路。
- Span 名、日志事件和 Metric label 使用稳定低基数模板。用户输入、资源名、URL 查询、身份/请求/Trace ID
  以及任何敏感正文不得成为高基数属性。
- 新增业务边界时按 `docs-internal/可观测和插桩规范.md` 验证父子传播、失败 Span、关联日志和敏感字段排除。

## 4. 文档边界

- `docs/docs/{zh,en}` 只写用户与部署管理员完成任务所需的现行行为；`docs-internal/` 保存长期内部规范。
- 公开文档采用“用途、前置条件、最短步骤、预期结果、必要排障”，高级参考渐进披露；不写研发计划、
  内部架构、发布门禁、迁移历史或完成流水。
- 配置说明必须同时交代用途、允许值/格式/范围和影响；安全、恢复与不可逆限制不能因精简而隐藏。
- 删除功能、字段或兼容路径时，同步删除代码、测试、Schema、示例、导航和中英文现行文档，不保留纪念性说明。
- OpenAPI、CLI help 和 Release 分别是 API、命令和版本事实源；不要在公开文档复制完整契约。
- 文档站变更至少执行 `pnpm --dir docs build`；导航或主要旅程变化还需浏览器验收。

## 5. 目录与技术栈

- 仓库为模块化单体 monorepo：Go API/Worker 在根目录，React 管理台在 `web/`，Agent 在 `luna-agent/`，
  文档站在 `docs/`，嵌套 CLI 在 `cli/luna-cli/`。
- 后端使用 Go、Gin、GORM、PostgreSQL、Redis/Asynq、golang-migrate、client-go 和 OpenAPI。
- 前端使用 Vite、React、TypeScript、Tailwind、shadcn/ui、TanStack Query、React Hook Form、Zod、
  i18next 与 Sonner；包管理器只用 pnpm。Python 项目只用 uv。
- 本地 Compose 只承载 PostgreSQL 与 Redis；API、Worker、Agent 和 Web 在宿主机启动。
- 环境文件不提交；根配置默认读取进程环境和 `.env`，临时文件用 `ENV_FILE` 显式指定。

## 6. 后端与数据

- `cmd/api` 负责 HTTP、Webhook、OAuth、CRUD、权限和任务投递；长耗时构建、部署、证书和清理进入 Worker。
- Handler 只处理传输、权限入口和响应；业务规则放 service，数据访问放 repository，外部系统放 provider。
- `internal/api` 根包只保留装配、全局中间件、路由组合和跨领域 HTTP 基础设施；领域 Handler、DTO 与测试
  放 `internal/api/<domain>api`，且不得反向导入根包。
- 平台角色与项目角色复用 `internal/authz`、`web/src/lib/roles.ts` 和 OpenAPI 共享 schema，不散落字面量。
- 数据库结构与一次性数据修复只由 `migrations/` 交付；启动路径不得 AutoMigrate、Force 或重复 backfill。
- 实际危险操作必须写 AuditLog；权限由后端最终判断，前端隐藏按钮只改善体验。
- 第一方 Luna CLI 登录权限始终等同当前用户的实时平台/项目权限，不接受、展示、持久化或预检用户可选 Scope。
  PAT、第三方 OAuth 应用和 Agent 服务身份仍按服务端 Scope 限权。
- 普通 Agent 平台工具以 OpenAPI operation 为唯一事实源；operation 变化必须同步真实路由、Schema、
  `requiredScopes`、审批、敏感字段和用途说明，并验证 `internal/aitool.PlatformCatalog()`。
- 工具目录按 `search_tools` / `get_tool_details` 渐进加载；检索只做发现，不授权或执行。Agent Prompt、
  工具描述和配套 Skill 使用中文，并要求模型按用户当前语言回复。

## 7. 前端边界

- 具体规则只在 `web/AGENTS.md` 维护。根级要求仅为：所有 UI 文案和可访问名称走 i18n；共享模块使用
  `@/` 导入；基础 UI 优先 shadcn/ui；表单使用 React Hook Form + Zod；服务端状态使用 TanStack Query。
- React、React DOM、CodeMirror 等依赖对象身份的运行时库必须解析为单一兼容版本；依赖变更执行
  `pnpm --dir web check:singletons`。
- 列表、分页、状态、页面框架和错误页复用公共组件；不在业务页重造基础组件或硬编码状态色。
- 未来可能超过 100 条的列表 API 必须分页排序，响应统一为
  `items/page/pageSize/sortBy/sortOrder/total/totalPages`，排序字段由后端白名单映射。

## 8. 集成与安全

- 构建主路径是平台 Builder + BuildKit rootless；不挂载宿主机 Docker socket，不默认 privileged。
- 构建网络默认 restricted egress，禁止元数据地址、Kubernetes API、Service CIDR 与私网非 443；
  内网 registry 可通过明确白名单或私网 TCP 443 放行。
- Webhook 校验签名且只接受已绑定仓库事件；OIDC Provider 由平台后台配置；平台不开放自由注册。
- AI 费用只归发起用户钱包：`ai.*` 的 usage/ledger `project_id` 必须为空；构建、运行、存储和网关费用
  仍可归属项目空间。

## 9. 验证与提交

- 小改动执行相关 Go 包测试、前端测试或文档检查。超过 8 个文件、跨 3 个业务域，或涉及认证、权限、
  Secret、SSRF、迁移、构建部署运行时、跨服务协议时执行完整验证并优先浏览器验收前端。
- 完整验证至少包括 `go test ./...`、Web test/lint/singletons/build、Agent lint/test/typecheck、
  OpenAPI/迁移/Helm/文档与相关 release gate；以各 `package.json` 和 CI 脚本为命令事实源。
- 前端 lint/build 不得新增 error 或 warning；不得通过关闭规则、放宽类型或扩大白名单掩盖问题。
- `TODO.md` 只记录真实未完成工作；完成记录由测试、提交和 PR 历史保存。
- 仅用户明确要求时提交或推送。提交消息使用
  `<type> <gitmoji>: <summary>`，例如 `feat ✨: 新增项目空间管理页面`。

## 10. 不要做

- 不为“更通用”增加无消费者字段、配置、状态、接口、Provider、缓存或抽象。
- 不保留未发布旧行为的兼容分支、空成功、吞错 wrapper 或伪可选开关。
- 不用前端缓存、数据库列或进程内状态冒充外部平台当前事实。
- 不以过度防御为由接受无效依赖、空对象或不可能状态；真实外部输入、并发、安全和资源释放边界仍须防守。
