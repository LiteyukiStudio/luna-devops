# 源码开发

Luna CLI 位于仓库的 `cli/` 工作区，并复用 `packages/api-contract` 与 `packages/api-client`。命令目录由本地命令和 OpenAPI 操作共同组成，不在 CLI 内维护第二份手写 API 清单。

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

`LUNA_HOME` 默认是 `~/.luna`。开发、测试和 CI 必须使用临时目录，避免读取或覆盖真实实例上下文。

## 目录职责

| 路径 | 职责 |
| --- | --- |
| `cli/src/commands` | 命令注册、参数解析、风险门禁和执行器 |
| `cli/src/auth` | Access Token、本地凭据和认证状态 |
| `cli/src/config` | 多实例 context 与默认项目空间 |
| `cli/src/input` | `key=value`、JSON、文件和标准输入 |
| `cli/src/output` | 人类输出、JSON Envelope 和脱敏 |
| `packages/api-contract` | 从 OpenAPI 生成的环境无关契约 |
| `packages/api-client` | Web 与 CLI 可复用的 HTTP 客户端 |
| `scripts/cli` | 契约校验、打包、制品验证与发布工具 |
| `ai-supports/skills` | 只通过 CLI 工作的 Agent Skills |

## 增加平台命令

公开控制面 API 应先进入 OpenAPI，并提供稳定 `operationId`、Scope 和 `x-luna-cli` 元数据。生成契约后，CLI 会注册对应的两级命令：

```text
luna <category> <tool> key=value
```

上下文、帮助、Completion 等纯本地能力在 `cli/src/commands/local.ts` 注册。不要为已经存在的 HTTP API 再写一个旁路命令实现。

修改 OpenAPI 后执行：

```bash
pnpm --filter @luna-devops/api-contract generate
node scripts/cli/verify-contract-drift.mjs
```

## 机器输出与 Skills

自动化和 AI Agent 必须显式传入 `agent=true`。该模式强制 JSON、禁用交互和颜色，并要求使用规范命令名：

```bash
luna help catalog query=registry limit=20 agent=true
luna help command path=registry.get-registries agent=true
```

机器可读 Help 是命令、参数、风险和输出结构的事实来源。`ai-supports/skills` 只描述任务顺序和安全边界，不复制完整命令手册。修改命令或 Skill 后必须运行：

```bash
node scripts/cli/verify-skills-sync.mjs
```

校验会拒绝不存在的具体命令、缺少 `agent=true` 的 Agent 命令，以及已知的过时能力声明。

context 设置默认项目后，执行器会为必填的 `project`、`projectId` 或
`projectID` 参数注入该项目的不可变 ID；命令显式传值时以显式值为准。可选项目参数不会被自动注入，避免污染 global 等跨项目请求。

`api request` 只保留给人类诊断已知相对 API 路径。Agent 模式固定拒绝该命令，不提供配置或参数绕过入口。

## 当前能力边界

当前源码包含 21 个本地命令和 110 个 OpenAPI 命令。没有进入机器可读目录的能力不能由 Skill 猜测或用 `api request` 冒充正式支持。

以下能力仍在发布前工作中：

- 尚未进入 OpenAPI 的公开后端路由；
- 服务端能力协商和版本兼容门禁；
- Authorization Code + PKCE 与 Device Code；
- OAuth Bearer Step-up MFA；
- SSE、WebSocket 和二进制下载适配；
- 高风险操作的服务端短时执行计划。

远程 high/critical 命令在服务端计划完成前会直接拒绝执行，`yes=true` 不能绕过。

## 验证

```bash
pnpm --filter @liteyuki/luna-cli typecheck
pnpm --filter @liteyuki/luna-cli lint
pnpm --filter @liteyuki/luna-cli test
pnpm --filter @liteyuki/luna-cli build
node --test scripts/cli/tests/*.test.mjs
node scripts/cli/verify-skills-sync.mjs
```

完整技术决策和后续计划见仓库中的 `notes/cli-spec.md` 与 `TODO.md`。
