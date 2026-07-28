# 源码开发

Luna CLI 已迁移到独立仓库
[`LiteyukiStudio/luna-cli`](https://github.com/LiteyukiStudio/luna-cli)。CLI
仓库是客户端源码、API Client、OpenAPI 契约副本、配套 Skill、测试和发布流程的
唯一事实来源；Luna DevOps 仓库继续维护后端实现和平台 OpenAPI。

两个仓库没有 submodule、subtree 或 subrepo 关系。CLI 的 CI 会只读检出指定平台
版本，用覆盖门禁检查平台路由、OpenAPI 和 CLI 命令是否同步。

## 本地克隆

Luna DevOps 仓库根目录预留了被 `.gitignore` 忽略的 `/cli/` 目录，可用于本地联调：

```bash
cd /path/to/luna-devops
git clone git@github.com:LiteyukiStudio/luna-cli.git cli
pnpm --dir cli install --frozen-lockfile
```

该目录不属于平台 pnpm workspace，不会进入平台提交或发布制品。也可以把 CLI
仓库克隆到任意其他目录。

## 目录职责

| CLI 仓库路径 | 职责 |
| --- | --- |
| `src/commands` | 命令注册、参数解析、风险门禁和执行器 |
| `src/auth` | Device Code、Access Token、本地凭据和认证状态 |
| `src/config` | 活动实例、凭据与默认项目空间 |
| `src/input` | `key=value`、JSON、文件和标准输入 |
| `src/output` | 人类输出、JSON Envelope 和脱敏 |
| `packages/api-contract` | 从 OpenAPI 生成的环境无关契约 |
| `packages/api-client` | CLI 使用的统一 HTTP 客户端 |
| `skills/luna-devops` | 与 CLI 配套发布的单一 Agent Skill |
| `scripts/cli` | 契约校验、打包、制品验证与发布工具 |
| `openapi` | 从平台同步并经 digest 校验的契约副本 |

## 本地运行

```bash
cd /path/to/luna-cli
pnpm install --frozen-lockfile

export LUNA_HOME="$(mktemp -d)"
pnpm exec tsx src/entry.ts version show
pnpm exec tsx src/entry.ts help catalog query=project limit=10 output=json interactive=false
```

开发、测试和 CI 必须使用临时 `LUNA_HOME`，避免读取或覆盖真实用户凭据。

## 同步平台契约

本地把 CLI 克隆到平台仓库的 `/cli/` 时，可执行：

```bash
pnpm --dir cli sync:openapi
LUNA_PLATFORM_ROOT=. pnpm --dir cli check:platform-coverage
```

如果 CLI 位于其他位置，显式指定平台仓库：

```bash
LUNA_PLATFORM_ROOT=/path/to/luna-devops pnpm check:platform-coverage
```

普通 JSON HTTP 业务命令以 OpenAPI 为唯一事实源。SSE、WebSocket 和文件下载等
特殊传输使用显式协议适配器；浏览器回调和 Webhook 接收器必须按精确路由登记，
不能用通配规则隐藏缺失能力。`api request` 只用于人工诊断，不计入覆盖率。

## Agent 与 Skill

Agent 固定使用机器输出：

```bash
luna help catalog all=true limit=100 output=json interactive=false agent=true
luna help command path=registry.get-registries output=json interactive=false agent=true
```

Agent 只解析 `stdout` 的 JSON，把 `stderr` 作为诊断通道。Skill 只描述意图路由、
任务顺序和安全边界，具体命令和参数始终以机器 Help 为准。

修改命令、参数、风险、能力边界或 Skill 后运行：

```bash
pnpm check:skills
```

## 验证

在 CLI 仓库执行：

```bash
pnpm check
pnpm check:release-scripts
pnpm check:contract
pnpm check:skills
LUNA_PLATFORM_ROOT=/path/to/luna-devops pnpm check:platform-coverage
```

完整技术规格见
[`docs/cli-spec.md`](https://github.com/LiteyukiStudio/luna-cli/blob/main/docs/cli-spec.md)。
