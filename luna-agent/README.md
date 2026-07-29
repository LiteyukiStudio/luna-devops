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

服务属于根 pnpm workspace，不维护自己的 lockfile。

```bash
pnpm install
pnpm --dir luna-agent dev
```

`dev` 会自动读取 `luna-agent/.env.local`（文件不存在时继续使用当前进程环境），
因此本地 Provider、模型和测试密钥可以保存在该 Git 忽略文件中，不需要写入命令行
或提交到仓库。

开发默认使用内存 Repository、确定性 Provider 和显式开发身份：

```bash
curl -H 'X-Luna-Dev-User: usr_local' \
  http://127.0.0.1:8091/internal/v1/capabilities
```

生产必须配置 PostgreSQL 和 JWT 验证，服务启动时会拒绝不安全的默认值。用于本地
PostgreSQL 验证的参考 DDL 位于 `sql/001_ai_schema.sql`；生产迁移仍由平台统一的
golang-migrate Job 管理，Agent 不会在启动时自动迁移。

## 配置

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HOST` / `PORT` | `127.0.0.1` / `8091` | 内部监听地址 |
| `DATABASE_URL` | 空 | 开发为空时使用内存；生产必填 |
| `AUTH_MODE` | `development` | `bff-hmac`（当前 BFF 契约）或 `jwt` |
| `API_SERVICE_TOKEN` | 空 | BFF 独立服务 Bearer，至少 32 字符 |
| `ACTOR_CONTEXT_SIGNING_KEY` | 空 | Actor Context HMAC key，至少 32 字符 |
| `RUN_GRANT_ENCRYPTION_KEY_BASE64` | 空 | 32 字节 AES-256-GCM key 的 Base64；持久存储时必填 |
| `LUNA_API_BASE_URL` | 空 | Luna API 内部 Service 根地址 |
| `AI_AGENT_CALLBACK_SERVICE_TOKEN` | 空 | Agent 调用 delegation/tool callback 的独立服务身份 |
| `TOOL_CATALOG_JSON` | 空 | 由 OpenAPI 生成的严格 operation metadata JSON |
| `API_SERVICE_JWT_PUBLIC_KEY` | 空 | 验证 `aud=luna-agent` 的 API 服务 JWT |
| `ACTOR_CONTEXT_PUBLIC_KEY` | 空 | 验证签名 Actor Context |
| `PROVIDER_TYPE` | `deterministic` | `deterministic` 或 `openai-compatible` |
| `PROVIDER_CONFIG_TTL_MS` | `300000` | 后台 Provider 配置的短时内存缓存 TTL |
| `PROVIDER_BASE_URL` | 空 | OpenAI-compatible HTTPS API 根地址 |
| `PROVIDER_API_KEY` | 空 | 仅驻留进程内，不写数据库、事件或日志 |
| `PROVIDER_MODEL` | 空 | Provider 模型 |
| `RUN_LEASE_SECONDS` | `30` | PostgreSQL Run 租约 |
| `RUN_MAX_WALL_MS` | `300000` | 单 Run 墙钟上限 |
| `MAX_INPUT_BYTES` | `48000` | Web 能力契约公布的单次输入字节上限 |
| `MAX_CONCURRENT_RUNS` | `2` | Web 能力契约公布的单用户并发 Run 上限 |

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

系统提示 `system-v2` 会向模型提供当前会话标题、标题来源和轮次。内建
`rename_conversation` 工具负责首轮命名与明显话题漂移后的标题修正；浏览器手动
命名会把来源持久化为 `user`，之后模型不再看到该工具，Repository 也会拒绝
Agent 覆盖。该双层保护不能被 Prompt 遵循情况替代。

新 Run 使用 `system-v3`。它在 `system-v2` 基础上加载
`skills/luna-devops-navigation/SKILL.md`，要求模型在回答引用平台注册页面或可信
资源 ID 时输出根相对 Markdown 链接。Skill 随 Agent 镜像发布；前端仍会独立校验
平台注册路径，Prompt 或 Skill 不能把任意 URL 提升为站内导航。

内建 `create_options` 为每个下一步选项保存稳定 ID 和独立重复策略：注册路由跳转
默认可重复，发送消息与请求操作成功后只锁定自身。`navigate_to_route` 是单独的
自动 UI 工具，仅用于用户明确要求的页面切换；浏览器只消费实时 SSE 完成事件并按
Tool Call ID 去重，历史 Timeline 不会再次触发跳转。

## 验证

```bash
pnpm --dir luna-agent lint
pnpm --dir luna-agent typecheck
pnpm --dir luna-agent test
pnpm --dir luna-agent build
```

尚未在本目录实现的跨模块 P0 内容包括：平台 OpenAPI 生成 Client、Go BFF、
golang-migrate 正式迁移和 Redis live fan-out。Run Actor Grant 兑换与工具执行
已经接入 Luna API 的固定内部 callback 路由；真实 Tool Catalog 必须由平台
OpenAPI 构建产物通过配置注入，不能由 Agent 猜测或按任意 URL 执行。
