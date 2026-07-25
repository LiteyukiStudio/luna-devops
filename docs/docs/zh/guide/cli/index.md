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

CLI 目前处于 `0.1.0` 开发阶段。源码已经可以运行和验证，当前命令目录共有 131 条命令，其中 21 条由 CLI 本地实现，110 条由 OpenAPI 契约生成。源码已经包含：

- 多实例、上下文和默认项目空间的配置模型；
- Access Token 登录、校验和本地凭据存储基础能力；
- `key=value`、JSON、文件和标准输入参数解析；
- 人类可读输出与版本化 JSON Envelope；
- 本地帮助、上下文、项目空间和 Completion 命令注册；
- 根据 OpenAPI 契约注册全部 110 个已登记操作；
- npm 包与 Bun 独立二进制的统一入口、CI、打包、全局安装 smoke 和发布门禁。

共享契约和 API Client 会被打包进 npm 与 Bun 制品，不要求用户安装 monorepo 工作区。项目尚未完成首次公开发布，因此文档中的安装命令要等 `cli-v*` 版本发布后才可使用。

从仓库运行 CLI、更新 OpenAPI 命令和执行验证的方法见[源码开发与验证](./development)。

## 设计边界

- CLI 只调用 Luna DevOps 后端 API，不直接编排 Kubernetes、GitHub、Gitea 或镜像站接口。
- 自动化应设置 `output=json interactive=false`，只解析 `stdout` 的 JSON；诊断信息写入 `stderr`。
- 本地配置默认放在 `~/.luna/`。测试和 CI 必须使用临时 `LUNA_HOME`，不能读取真实用户凭据。
- 中风险操作在交互终端中使用统一确认；非交互模式必须显式传入 `yes=true`。
- 高风险 API 在服务端执行计划协议完成前直接拒绝执行，`yes=true` 也不能绕过。
- CLI 和平台使用独立版本。兼容性由服务端能力协商决定，不只比较版本号。
- `help catalog` 中存在命令只代表当前 CLI 已登记该命令；服务端能力协商完成前，`serverSupported` 可能为 `null`。
- Agent 必须为每条命令传入 `agent=true`。该模式会固定 JSON 输出、关闭交互与颜色，并对分页、轮询和输出体积应用安全限制。
- context 配置默认项目后，项目级命令可省略必填的
  `project`、`projectId` 或 `projectID`；CLI 会注入该不可变项目 ID，但不会因此扩大权限。
- `api request` 只供人类诊断已知相对 API 路径，并在 Agent 模式下固定禁用；它不能拿来伪装平台尚未进入 OpenAPI 或尚未完成专用传输的业务能力。

## Agent Skills

配套 Skills 位于仓库的 [`ai-supports/skills`](https://github.com/LiteyukiStudio/luna-devops/tree/main/ai-supports/skills) 目录。Skills 负责意图路由、操作顺序和安全边界，具体命令、参数、风险与输出结构始终以机器 Help 为准。

```bash
luna help catalog query=project limit=20 agent=true
luna help command path=project.get-projects agent=true
```

CLI 命令或能力边界变化后，必须同步更新 Skills，并运行：

```bash
node scripts/cli/verify-skills-sync.mjs
```

## 下一步

首次公开发布前还需要完成：

1. 补齐尚未进入 OpenAPI 的公开后端路由，并完成完整命令覆盖率测试。
2. 接入服务端能力协商、Authorization Code + PKCE、Device Code 和 Bearer Step-up MFA。
3. 完成 SSE、WebSocket、下载和服务端执行计划协议。
4. 在 npm 配置 Trusted Publisher，并保护 GitHub `npm` Environment。
5. 接入 Apple Developer ID 和公证后，再把 macOS 二进制加入稳定版本；Windows 继续使用 npm/pnpm 安装。

具体安装方式见[安装与使用](./installation)，源码开发见[源码开发与验证](./development)，制品校验见[发布与制品验证](./release-security)。
