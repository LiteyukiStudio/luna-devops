# Luna Agent

Luna DevOps 内嵌 AI 助手的独立 Node.js 服务。当前实现覆盖 P0 服务骨架：内部
HTTP API、会话/Turn/Run/Timeline 持久化、可恢复事件、Run 租约执行器、
LangGraph.js `assistant-v1`、Provider 兼容层、身份验证抽象与默认脱敏。

Agent 侧 P1/P2 公共执行管线位于 `src/tools/`：严格 Tool Catalog、JSON Schema
参数约束、风险/批准/MFA 策略、缺参中断、不可变重试 attempt、工具调用上限、
最终状态验证和结构化 Timeline Event。所有业务操作只能通过
`LunaApiToolClient` 先兑换 operation-bound 委托令牌后调用 Luna API；没有
Kubernetes、业务数据库、任意 URL、Shell 或第三方平台执行器。

配置 Luna API Provider 回调后，真实 Graph 统一使用 `ManagedProvider` 拉取版本化
配置。API Key 只存在于短 TTL 进程内缓存，不进入文件、数据库、Checkpoint、
Timeline 或日志；缓存过期后的下一次真实调用会使用后台新配置。确定性 Provider
只用于显式本地开发与测试，生产启动强制要求 Luna API Provider 配置。

浏览器不能访问本服务。生产流量只能由 Luna API 通过 `/internal/v1/*` 进入；
本服务不连接 Luna 业务数据库、Kubernetes、Redis 业务队列或外部平台。

## 本地运行

服务是独立 pnpm 项目，依赖和 lockfile 都由 `luna-agent/` 自己维护。

```bash
cd luna-agent
pnpm install
pnpm dev
```

`dev` 会自动读取 `luna-agent/.env.local`（文件不存在时继续使用当前进程环境），
因此本地 Provider、模型和测试密钥可以保存在该 Git 忽略文件中，不需要写入命令行
或提交到仓库。

开发环境未填写模型配置时使用内存 Repository、确定性测试 Provider 和显式开发身份：

```bash
curl -H 'X-Luna-Dev-User: usr_local' \
  http://127.0.0.1:8091/internal/v1/capabilities
```

生产必须配置 PostgreSQL、`bff-hmac` 验证和内部根密钥，服务启动时会拒绝不安全的默认值。持久层使用
Drizzle ORM 访问 `ai` schema；Schema 只负责运行时类型与查询，数据库迁移继续由平台统一的
golang-migrate Job 管理，Agent 不会在启动时自动迁移。本地验证可使用 `sql/001_ai_schema.sql`
参考 DDL 初始化专用临时库，并通过 `AGENT_TEST_DATABASE_URL` 运行真实 PostgreSQL 集成测试：

```bash
AGENT_TEST_DATABASE_URL=postgres://devops:devops@127.0.0.1:5432/<临时库> pnpm vitest run tests/postgres-repository.test.ts
```

## 配置

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HOST` / `PORT` | `127.0.0.1` / `8091` | 内部监听地址 |
| `DATABASE_URL` | 空 | 开发为空时使用内存；生产必填 |
| `AUTH_MODE` | `development` | 生产使用 `bff-hmac` |
| `AI_INTERNAL_SECRET` | 空 | API 与 Agent 共享的稳定内部根密钥，至少 32 字节；自动派生用途隔离子密钥 |
| `LUNA_API_BASE_URL` | 空 | Luna API 内部 Service 根地址 |
| `TOOL_CATALOG_JSON` | 空 | 由 OpenAPI 生成的严格 operation metadata JSON |
| `PROVIDER_BASE_URL` | 空 | 本地直连时使用的 OpenAI-compatible API 根地址 |
| `PROVIDER_API_KEY` | 空 | 本地直连密钥，仅驻留进程内 |
| `PROVIDER_MODEL` | 空 | 本地直连模型名称 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 空 | OTLP/HTTP Collector 根地址；为空时不初始化遥测 SDK |
| `OTEL_RESOURCE_ATTRIBUTES` | 空 | 附加资源属性，例如 `deployment.environment.name=production` |
| `OTEL_EXPORTER_OTLP_HEADERS` | 空 | Collector 鉴权 Header，使用 OTel 标准逗号分隔格式 |

Provider 不需要手动选择类型。连接 Luna API 时，Agent 自动读取后台保存的 API
地址、加密 API Key 和模型名称；没有 Luna API 时，只有同时填写上述三个本地直连
变量才会连接模型。三项均为空的确定性 Provider 只用于非生产开发和测试，生产环境
不会回退到测试回复。

模型请求超时、单次 Run 超时和每个 Agent 实例的并发数由“全局设置 → AI 助手 →
高级运行设置”动态下发。配置刷新间隔、Run 轮询间隔和数据库租约属于内部一致性参数，
使用代码中的安全默认值，不再暴露为部署环境变量。

## 可观测性

只需设置 `OTEL_EXPORTER_OTLP_ENDPOINT`，Agent 就会通过 OTLP/HTTP 输出 Trace、
Metrics 和 Logs，服务名固定为 `luna-agent`。未设置端点时不会启动导出线程，也不会
尝试连接 Collector。

自动插桩覆盖 Fastify、HTTP/fetch、PostgreSQL 与 Pino；业务插桩覆盖 Run 循环、模型
流、工具执行、审批、交互卡片、Provider 配置读取和 Luna API 调用。日志与 Span 只
记录稳定错误代码、调用类型和低敏资源标识，不记录 Prompt、消息正文、工具参数、
Token、API Key、Secret 或 HTTP Body。跨服务调用使用 W3C `traceparent` /
`tracestate` 传播上下文。

指标包括运行量和耗时、活跃运行、模型调用、首个输出延迟、循环轮次和 Token、工具调用、审批决策、卡片生成、
外部请求与数据库连接池指标。指标标签只使用操作名、结果和阶段等低基数字段；Run ID、
用户 ID 与请求 ID 只进入 Trace 或结构化日志。

## 内部 API

- `GET /internal/health/live`
- `GET /internal/health/ready`
- `GET /internal/v1/health/compatibility`
- `GET /internal/v1/capabilities`
- `GET /internal/v1/provider/health`
- Conversation CRUD
- `GET .../timeline`
- `POST .../turns`
- `GET /internal/v1/runs/:runId`
- `GET /internal/v1/runs/:runId/events?after=0&stream=true`
- `POST /internal/v1/runs/:runId/cancel`

SSE 游标是单 Run 单调递增的 `event_sequence`。事件先持久化，随后才能被读取；
Redis fan-out 接入后也必须维持这一顺序。Provider 的原始 partial JSON、隐藏思维
链和续接 artifact 不进入 Timeline。

新 Run 统一使用中文系统提示 `system-v4`。系统提示会向模型提供当前会话标题、标题
来源和轮次，并加载 `skills/luna-devops-navigation` 与
`skills/luna-devops-interaction`。领域指引根据当前消息、页面上下文和可用工具从
`references/` 按需加载；页面路由清单只在读取、浏览或明确跳转意图下加载。

内建 `rename_conversation` 工具负责首轮命名与明显话题漂移后的标题修正；浏览器手动
命名会把来源持久化为 `user`，之后模型不再看到该工具，Repository 也会拒绝 Agent
覆盖。项目仅维护当前 Prompt 版本；系统提示、模型任务提示、工具描述和 Skill 后续
均使用中文编写。

内建 `create_options` 为每个下一步选项保存稳定 ID 和独立重复策略：注册路由跳转
默认可重复，发送消息与请求操作成功后只锁定自身。`navigate_to_route` 是单独的
自动 UI 工具，仅用于用户明确要求的页面切换；浏览器只消费实时 SSE 完成事件并按
Tool Call ID 去重，历史 Timeline 不会再次触发跳转。

Run 使用统一的有界 Agent Loop：模型发起 Tool Call 后，执行器完成策略与权限检查，
把带调用 ID 的工具结果按 OpenAI-compatible `assistant.tool_calls` / `tool` 消息回灌，
再继续下一轮模型判断，直到得到最终答复或进入批准、MFA、补充输入、取消、超时及
调用上限等明确终态。执行器不得把第二轮及后续平台 Tool Call 当作最终回复丢弃。

## 验证

```bash
cd luna-agent
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

尚未在本目录实现的跨模块 P0 内容包括：平台 OpenAPI 生成 Client、Go BFF、
golang-migrate 正式迁移和 Redis live fan-out。Run Actor Grant 兑换与工具执行
已经接入 Luna API 的固定内部 callback 路由；真实 Tool Catalog 必须由平台
OpenAPI 构建产物通过配置注入，不能由 Agent 猜测或按任意 URL 执行。
