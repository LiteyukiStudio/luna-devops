# 源码开发

Luna CLI 位于仓库的 `cli/` 工作区，并复用 `packages/api-contract` 与
`packages/api-client`。普通业务命令以 OpenAPI 为唯一事实源，CLI 不维护第二份
手写 API 清单。

## 本地运行

在仓库根目录安装锁定依赖：

```bash
pnpm install --frozen-lockfile
```

使用临时目录隔离本机凭据，然后运行源码：

```bash
export LUNA_HOME="$(mktemp -d)"
pnpm --silent --dir cli exec tsx src/entry.ts version show
pnpm --silent --dir cli exec tsx src/entry.ts help catalog query=project limit=10 output=json interactive=false
```

`LUNA_HOME` 默认是 `~/.luna`。开发、测试和 CI 必须使用临时目录，避免读取或覆盖真实实例凭据。

## 目录职责

| 路径 | 职责 |
| --- | --- |
| `cli/src/commands` | 命令注册、参数解析、风险门禁和执行器 |
| `cli/src/auth` | Access Token、本地凭据和认证状态 |
| `cli/src/config` | 活动实例、凭据与默认项目空间 |
| `cli/src/input` | `key=value`、JSON、文件和标准输入 |
| `cli/src/output` | 人类输出、JSON Envelope 和脱敏 |
| `packages/api-contract` | 从 OpenAPI 生成的环境无关契约 |
| `packages/api-client` | Web 与 CLI 可复用的 HTTP 客户端 |
| `scripts/cli` | 契约校验、打包、制品验证与发布工具 |
| `ai-supports/skills` | 只通过 CLI 工作的 Agent Skills |

## 命令来源与边界

命令只有三种来源：

1. **OpenAPI 普通业务命令**：公开控制面 JSON HTTP API 必须先进入 OpenAPI，
   并提供稳定 `operationId`、Scope、输入输出 Schema 和 `x-luna-cli` 元数据。
2. **协议适配器**：仅用于 SSE、WebSocket、文件下载、短时授权后的传输等无法
   由普通 JSON HTTP 命令正确表达的协议。
3. **本地命令**：登录、本地配置、帮助和 Completion 等不对应平台业务 API 的能力。

生成契约后，普通业务操作会注册为对应的两级命令：

```text
luna <category> <tool> key=value
```

认证、项目空间、帮助、Completion 等纯本地能力在
`cli/src/commands/local.ts` 注册。不要为已经存在的普通 HTTP API 再写一个
旁路命令实现。

协议适配器必须满足以下约束：

- 指向真实存在的平台路由，不直接调用 Kubernetes、Git、Registry 等外部平台；
- 不复制 OpenAPI 普通业务命令，只负责流式、双向或二进制传输；
- 对应路由在 OpenAPI 中标记为隐藏协议操作，并提供排除原因；
- 在平台覆盖门禁中按精确 `method + path` 登记并验证。

浏览器回调、Webhook 接收器和启动前引导端点可以不生成 CLI 命令，但同样必须
逐路由登记分类和原因。禁止使用目录、前缀或业务域通配规则掩盖新接口。

修改 OpenAPI 后执行：

```bash
pnpm --filter @luna-devops/api-contract generate
node scripts/cli/verify-contract-drift.mjs
```

## 机器输出与 Skills

AI Agent 必须为每条命令显式传入
`output=json interactive=false agent=true`，不得依赖本地默认输出或交互状态。
该模式还会禁用颜色并要求使用规范命令名：

```bash
luna help catalog all=true limit=100 output=json interactive=false agent=true
luna help command path=registry.get-registries output=json interactive=false agent=true
```

Agent 只解析 `stdout` 的 JSON，把 `stderr` 作为诊断通道；不得解析人类表格、
帮助文案或终端颜色。机器可读 Help 是命令、参数、风险和输出结构的事实来源。
`ai-supports/skills` 只描述意图路由、任务顺序和安全边界，不复制完整命令手册。

修改命令、参数、风险、能力边界或 Skill 后必须运行：

```bash
pnpm check:cli-skills
```

校验会拒绝不存在的具体命令、未固定
`output=json interactive=false agent=true` 的 Agent 命令，以及已知的过时能力声明。

使用 `project use` 设置默认项目后，执行器会为必填的 `project`、`projectId` 或
`projectID` 参数注入该项目的不可变 ID；命令显式传值时以显式值为准。可选项目参数不会被自动注入，避免污染 global 等跨项目请求。

`api request` 只保留给人类诊断已知相对 API 路径。Agent 模式固定拒绝该命令，不提供配置或参数绕过入口。

## 覆盖门禁

平台公开业务功能是否被 CLI 覆盖，由仓库根目录的门禁统一判断：

```bash
pnpm check:platform-cli-coverage
```

该命令会：

- 从 Gin Router 提取 `/api/v1` 路由；
- 读取 OpenAPI 操作及其 CLI 分类；
- 分页读取 CLI 机器目录；
- 逐业务域比较平台路由、OpenAPI、CLI 命令和显式排除项；
- 拒绝缺少 OpenAPI 的普通业务路由、缺少 CLI 命令的 OpenAPI 操作、未登记的
  协议/回调/Webhook 路由，以及没有原因的排除项；
- 要求普通业务命令覆盖率为 100%。

门禁退出码为 `0` 才表示当前提交满足覆盖要求。命令打印的实时总数、业务域明细
和覆盖比例是唯一统计来源；文档和 TODO 不复制这些数字。

这里的“全覆盖”是指每条公开平台路由都满足以下条件之一：

1. 普通业务 API 已进入 OpenAPI，并在机器目录中存在对应规范命令；
2. 特殊传输已由协议适配器消费，并通过 OpenAPI 和精确路由分类共同审计；
3. 非 CLI 入口按精确路由登记为浏览器回调、Webhook 接收器或显式排除，并说明原因。

`api request` 是人类诊断工具，不计入覆盖率，也不能作为缺失业务命令的替代品。
远程 `high`/`critical` 命令在交互终端中逐次确认；非交互或 Agent 模式必须显式
传入 `--yes`。CLI 确认只表达调用意图，覆盖命令也不能绕过服务端权限、Scope、
Step-up MFA 或资源一致性策略。终端和数据导出还要求 CLI OAuth 登录与对应
purpose 的有效 Step-up，个人访问令牌不可替代。

## 验证

```bash
pnpm --filter @luna-devops/api-contract generate
node scripts/cli/verify-contract-drift.mjs
pnpm --filter @liteyuki/luna-cli typecheck
pnpm --filter @liteyuki/luna-cli lint
pnpm --filter @liteyuki/luna-cli test
pnpm --filter @liteyuki/luna-cli build
node --test scripts/cli/tests/*.test.mjs
pnpm check:platform-cli-coverage
pnpm check:cli-skills
```

其中契约 drift、平台覆盖和 Skill 同步三项共同防止平台、OpenAPI、CLI 与 Agent
说明各自演进。新增或修改公开业务接口时，至少需要运行这三项；发布前执行完整
命令组。

完整技术决策和后续计划见仓库中的 `notes/cli-spec.md` 与 `TODO.md`。
