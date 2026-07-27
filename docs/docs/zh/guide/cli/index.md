# Luna CLI

Luna CLI 是 Luna DevOps 的命令行客户端，既服务于日常终端操作，也为自动化 Agent 提供稳定的 JSON 输入输出契约。

命令使用固定的两级结构：

```text
luna <工具分类> <具体工具> key=value
```

例如，机器可读帮助目录采用：

```bash
luna help catalog query=project limit=5 output=json interactive=false
```

## 当前开发状态

CLI 目前处于预发布阶段。命令目录由以下三类受控来源组成，实时数量以
`luna help catalog` 和平台覆盖门禁的输出为准，文档不手工维护容易漂移的数量：

- 普通业务命令：从 OpenAPI 契约生成，OpenAPI 是唯一事实源；
- 协议适配命令：只处理 SSE、WebSocket、文件下载等普通 JSON HTTP 命令无法保留语义的传输；
- 本地命令：只处理登录、本地配置、帮助和 Shell Completion 等不对应平台业务 API 的能力。

源码已经包含：

- 单一活动实例、账号凭据和默认项目空间的配置模型；
- 默认使用 OAuth Device Code 登录、自动刷新与尽力吊销，并支持显式的个人访问令牌备用登录；
- `key=value`、JSON、文件和标准输入参数解析；
- 人类可读输出与版本化 JSON Envelope；
- 本地帮助、项目空间和 Completion 命令注册；
- `login`、`logout`、`whoami`、`doctor` 人类友好顶层短命令；
- 检查当前登录、认证、服务端版本、OpenAPI 契约和能力开关的 `health doctor`；
- 每个 OpenAPI 业务命令执行前自动校验 API 代际、最低 CLI 版本和 OpenAPI 摘要；
- 根据 OpenAPI 契约注册已登记的普通业务操作；
- npm 包与 Bun 独立二进制的统一入口、CI、打包、全局安装 smoke 和发布门禁。

共享契约和 API Client 会被打包进 npm 与 Bun 制品，不要求用户安装 monorepo 工作区。预发布版本已经可以从 npm 的 `beta` 通道安装。

## 自带帮助与语言

CLI 不依赖 Skills 也能完成命令发现和基本操作：

```bash
luna --help
luna login
luna login server=https://devops.example.com
printf '%s' "$LUNA_TOKEN" | luna login mode=access-token token=@-
luna whoami
luna doctor
luna logout
luna project --help
luna project get-projects --help
```

顶层短命令与 `auth login`、`auth status`、`health doctor`、`auth logout`
共用处理器，不维护第二套行为。它们只用于人类交互；脚本和 Agent 使用稳定的两级
canonical 命令，`agent=true` 会拒绝顶层别名。

帮助会逐级展示分类、工具、权限、风险、参数、输入来源和示例。语言优先级为：

1. 命令行 `--lang`；
2. 环境变量 `LUNA_LANG`；
3. 本地配置的 `language`；
4. `LC_ALL`、`LC_MESSAGES`、`LANG` 和运行时语言；
5. 英文回退。

```bash
LUNA_LANG=zh-CN luna --help
luna --lang zh-CN project get-projects --help
```

从仓库运行 CLI、更新 OpenAPI 命令和执行验证的方法见[源码开发与验证](./development)。

## 设计边界

- CLI 只调用 Luna DevOps 后端 API，不直接编排 Kubernetes、GitHub、Gitea 或镜像站接口。
- 普通业务 HTTP 命令只能从 OpenAPI 生成，不允许在 CLI 中维护第二套手写路由或参数定义。
- SSE、WebSocket、二进制下载和授权后传输等能力必须使用显式协议适配器。适配器不得复制普通 JSON HTTP API，并必须进入逐路由覆盖审计。
- 浏览器回调、外部 Webhook 接收器和启动前引导端点不暴露为业务命令，但必须按精确的 `method + path` 登记分类和原因，不能使用路径前缀静默排除。
- 自动化应设置 `output=json interactive=false`，只解析 `stdout` 的 JSON；诊断信息写入 `stderr`。
- 本地配置默认放在 `~/.luna/`。测试和 CI 必须使用临时 `LUNA_HOME`，不能读取真实用户凭据。
- `high` 和 `critical` 操作在交互终端中逐次确认；非交互或 Agent 模式必须显式
  传入 `--yes`。`--yes` 只关闭 CLI 提示，不能绕过后端权限、Scope、Step-up
  MFA 或其他服务端策略。
- CLI 和平台使用独立版本。兼容性由服务端能力协商决定，不只比较版本号。
- 普通 OpenAPI 业务命令会在首次访问实例时读取 `/api/v1/meta`，校验 API
  代际、最低 CLI 版本和 OpenAPI digest；同一进程内会缓存通过结果。
- `luna health doctor output=json` 会显式展示当前登录、认证、兼容性和服务端功能开关，适合主动诊断。
- `help catalog` 中存在命令只代表当前 CLI 已登记该命令；`serverSupported`
  仍可能为 `null`，具体请求是否兼容以执行前协商结果为准。
- Agent 必须为每条命令显式传入
  `output=json interactive=false agent=true`，不得依赖本地默认输出或交互状态。
  Agent 模式还会关闭颜色，并对分页、轮询和输出体积应用安全限制。
- 使用 `luna project use project=<id>` 配置默认项目后，项目级命令可省略必填的
  `project`、`projectId` 或 `projectID`；CLI 会注入该不可变项目 ID，但不会因此扩大权限。
- CLI 只保存一个活动登录。`luna login` 未指定 `server` 时固定使用
  `https://devops.liteyuki.org`；显式登录其他地址或账号会覆盖现有凭据和默认项目空间。
- `api request` 只供人类诊断已知相对 API 路径，并在 Agent 模式下固定禁用；它不能拿来伪装平台尚未进入 OpenAPI 或尚未完成专用传输的业务能力。
- 终端和数据导出由专用 WebSocket/下载协议适配器执行。它们要求 CLI OAuth
  登录并完成对应 purpose 的 Step-up MFA；个人访问令牌不能满足或绕过该要求。

机器或 Agent 应通过机器目录发现能力，不解析人类帮助文本：

```bash
luna help catalog all=true limit=100 output=json interactive=false agent=true
luna help command path=project.get-projects output=json interactive=false agent=true
```

Agent 命令固定使用 `output=json interactive=false agent=true`。调用方只读取
`stdout` 的 JSON Envelope，把 `stderr` 视为诊断通道。

## Agent Skill

配套 `luna-devops` Skill 位于仓库的 [`ai-supports/skills/luna-devops`](https://github.com/LiteyukiStudio/luna-devops/tree/main/ai-supports/skills/luna-devops)
目录。根 `SKILL.md` 负责意图路由、通用操作顺序和安全边界，领域资料放在
`references/` 并按任务需要加载。具体命令、参数、风险与输出结构始终以机器 Help
为准。

Skill 跟随 CLI 一起发布，版本必须与 CLI 完全相同。每个 `cli-v*` GitHub
Release 只包含一个 `luna-devops-<version>.skill`；版本不一致时不得加载。

```bash
luna help catalog query=project limit=20 output=json interactive=false agent=true
luna help command path=project.get-projects output=json interactive=false agent=true
```

CLI 命令、参数、风险或能力边界变化后，必须在同一变更中同步 Skill，并运行：

```bash
pnpm check:cli-skills
```

## 下一步

稳定版发布前还需要完成：

1. 让平台路由、OpenAPI、CLI 命令和协议适配器覆盖门禁持续通过；门禁输出是覆盖数量与比例的唯一统计。
2. 补充 Authorization Code + PKCE 的 CLI 入口；Device Code、刷新、吊销和 OAuth Bearer Step-up MFA 已可用。
3. 完成干净实例上的全操作与关键旅程总验证，包括终端、数据导出和 Step-up MFA。
4. 在 npm 配置 Trusted Publisher，并保护 GitHub `npm` Environment。
5. 接入 Apple Developer ID 和公证后，再把 macOS 二进制加入稳定版本；Windows 继续使用 npm/pnpm 安装。

具体安装方式见[安装与使用](./installation)，源码开发见[源码开发与验证](./development)，制品校验见[发布与制品验证](./release-security)。
