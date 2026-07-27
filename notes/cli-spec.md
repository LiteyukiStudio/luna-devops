# Luna CLI 规格

> 状态：设计稿
>
> 目标版本：CLI v0.1.0
>
> 最后更新：2026-07-27
>
> 命令名：`luna`

## 1. 背景

Luna DevOps 需要一个同时面向开发者、平台管理员、CI 和 AI Agent 的命令行客户端。CLI 应覆盖平台公开 API，提供稳定、可脚本化的输出，并使用一次活动登录表示当前 Luna DevOps 实例和账号。切换实例或账号时重新登录，不引入额外的命名上下文。

CLI 与 Web 控制台共用后端能力和 API 契约，但不直接复用浏览器环境中的 `web/src/api`。现有前端 Client 依赖 `import.meta.env`、Cookie、i18next 单例和 MFA Dialog，不适合直接运行在命令行环境。CLI 与 Web 应共同依赖一个环境无关的 API Client 包。

## 2. 目标与非目标

### 2.1 目标

- 使用 TypeScript 开发，并放入现有仓库的 pnpm workspace。
- 使用 Bun 编译为无需预装 Node.js/Bun 的单文件可执行程序。
- 同时发布公开 npm 包，支持通过 npm、pnpm 全局安装或一次性执行。
- 支持从 GitHub Release 直接下载二进制并放入用户自己的 `PATH`。
- 支持 macOS、Linux 和 Windows 的主流 CPU 架构。
- 支持登录任意 Luna DevOps 实例，并保存一个活动实例、账号凭据和项目空间默认值。
- 支持 OAuth 浏览器登录、OAuth Device Authorization Grant 和个人访问令牌登录。
- 支持 OAuth Token 刷新、吊销、登出和敏感操作的 Step-up MFA。
- 覆盖所有面向用户和管理员的公开平台 API。
- 提供完整的分层帮助、示例、Shell Completion 和机器可读命令目录。
- 支持中文和英文，并根据系统语言自动选择。
- 对人类提供清晰的表格、提示和交互，对 AI/脚本提供稳定的 JSON/YAML/JSONL。
- 保持 stdout、stderr、退出码和错误结构稳定。

### 2.2 非目标

- 不把 Web 控制台完整搬到终端。
- 不通过 CLI 直接调用 GitHub、Gitea、Registry 或 Kubernetes API。外部平台仍由 Luna DevOps 后端适配。
- 不在第一版提供动态插件系统。动态插件会增加单二进制分发、权限和供应链风险。
- 不让 CLI 绕过后端权限、审计、MFA、Secret 脱敏或资源隔离。
- 不保证 Webhook、OAuth 回调、探针上报等服务端入口可作为用户命令调用。

## 3. 当前平台能力核对

| 能力 | 当前状态 | CLI 落地要求 |
| --- | --- | --- |
| OAuth Authorization Code + PKCE | 已有 | 增加适合原生 CLI 的公共客户端和 loopback redirect |
| OAuth Refresh Token / Revoke | 已有 | CLI 自动刷新并在登出时尽力吊销 |
| OAuth Device Code | 已实现 | `luna login` 默认使用内置 `luna-cli` 公共客户端和 RFC 8628 轮询 |
| 个人访问令牌 | 已有 | 支持从 stdin 或环境变量读取 |
| Step-up MFA | 已有统一错误与 Web 交互 | 增加绑定 OAuth 授权会话的 CLI 验证流程 |
| OpenAPI | 已有但覆盖不完整 | 除内部可观测白名单外，补齐全部 HTTP API、`operationId` 和 CLI 元数据 |
| 前端 API Client | 已模块化但依赖浏览器环境 | 抽取环境无关的共享 Client |
| 活动实例与账号 | 未实现 | 由 CLI 本地配置保存一个活动登录，重新登录时整体替换 |

当前路由数量明显高于 OpenAPI 已记录的接口数量，并且 OpenAPI 缺少稳定的 `operationId`。因此，在 OpenAPI 成为完整契约前，不能宣称 CLI 已覆盖全部接口。

### 3.1 2026-07-24 可实施性审计快照

本次审计直接对照 `internal/api/router.go` 与当前 OpenAPI：

- Gin Router 当前注册 222 个唯一 HTTP 路由；
- OpenAPI 当前记录 110 个 operation。按 `method + normalizedPath` 核对后，110 个 operation 均能对应现有路由，另有 113 个 Gin 路由尚未进入 OpenAPI，没有孤立 operation；当前实际路由覆盖率约为 49.3%；
- 113 个缺失 operation 主要分布在项目空间及其子资源、通知、计费、OAuth、运行集群，其余分布在构建、事件、镜像站、Git、应用模板、认证元数据和系统组件；因此 Phase 0 必须按业务域批量补契约，不能依靠实现 CLI 命令时顺手补接口；
- 当前 OpenAPI 没有稳定 `operationId`，也没有 `x-luna-cli` 元数据；
- 公开接口中同时存在普通 JSON、SSE、WebSocket、二进制下载和 OAuth 协议端点，不能共用一个普通 CRUD 调用模板；
- 当前错误响应由多种 helper 和直接 JSON 写入共同产生，尚未形成覆盖全部业务与协议接口的稳定错误 Envelope；仅生成成功响应类型不足以保证 CLI 的机器输出稳定；
- npm 制品运行在 Node.js，独立二进制运行在 Bun；代理、自定义 CA、SSE 和 WebSocket 的运行时能力并不完全等价，必须经过统一 Transport 接口适配，不能假设浏览器或某个运行时的全局对象在另一端行为一致；
- 当前 OAuth Token Endpoint 仅支持 `client_secret_basic` 与 `client_secret_post`，还不能安全支持没有 Client Secret 的原生 CLI 公共客户端；
- 当前后端尚未实现 Device Authorization Grant；
- Step-up MFA assertion 已支持绑定浏览器 Session 或内置 CLI OAuth Grant；个人访问令牌仍被明确拒绝。
- 终端预授权、终端存活监控和数据导出一次性票据已使用统一认证上下文，可绑定浏览器 Session 或 CLI OAuth Grant。
- 当前 OAuth 授权复用个人访问令牌的可创建 Scope 规则，而数据导出等必须 Step-up 的 Scope 被个人令牌目录刻意排除；不拆分 OAuth 与 PAT Scope 策略时，CLI 无法取得这些敏感能力的授权。
- 当前 Bearer 鉴权通过 `RequiredAccessTokenScope` 维护独立的路由到 Scope 映射，未映射路由统一得到 `system:unmapped` 并被拒绝；通知、看板、数据保留、构建模板等现有路由仍有落入该分支的风险。只补 OpenAPI 和命令而不补 Scope 映射，CLI 仍会稳定返回 403。

以上数字是设计审计快照，不作为长期硬编码基线。实现阶段必须由测试代码直接读取 Gin `router.Routes()` 和 OpenAPI 生成最新覆盖报告。

### 3.2 第一版完成定义

本文中的“第一版”统一指可以发布的 `v0.1.0`，不是只完成 CLI 外壳的内部里程碑。`v0.1.0` 发布前必须同时完成：

1. 全部 HTTP 路由 100% 分类；
2. 除明确登记的内部可观测白名单外，全部 HTTP API 100% 进入 OpenAPI；
3. 全部面向用户或管理员的公开控制面能力 100% 具有高层 CLI 命令或专用协议适配器；
4. 全部允许 Bearer 调用的业务与协议路由 100% 映射到稳定 Scope，OpenAPI、OAuth consent、运行时鉴权和 CLI Help 使用同一份 Scope 定义；
5. OAuth 公共客户端、PKCE、Device Code、Refresh、Revoke、Bearer Step-up MFA 与受保护交互操作的 OAuth 认证上下文后端契约可用；
6. 全部业务与协议接口使用稳定错误 Envelope，OpenAPI 声明成功和错误响应，CLI 不依赖本地化 message 判断流程；
7. 每项公开业务或协议能力的成功主路径 100% 通过，关键用户旅程 100% 通过，其余完整操作场景矩阵通过率不低于 95%；
8. npm 包和目标平台二进制完成相同协议测试、真实安装与 smoke test。

Phase 0 至 Phase 4 只是实施顺序，任何一个 Phase 未完成时都不能以“已经覆盖全部 API”的名义发布 `v0.1.0`。

### 3.3 可落地性结论与发布阻断项

本方案没有依赖无法实现的技术能力，但它不是一个只在 `packages/cli` 中编写命令即可完成的任务。第一版要达到“全部平台 API 可调用且 95% 以上可用”，必须先完成服务端契约、认证和协议能力改造。

以下项目是 `v0.1.0` 的 P0 发布阻断项：

1. 路由、OpenAPI、`operationId`、Scope 和 CLI 元数据形成单一可校验清单，消除当前 113 个未进入 OpenAPI 的路由缺口。
2. 所有公开 JSON 与协议握手错误收敛到稳定 Error Envelope，并从语言无关的错误目录生成或校验 Go、OpenAPI、Web 和 CLI 定义。
3. OAuth consent、OpenAPI security、Bearer 路由鉴权和 CLI Help 使用同一份 Scope 目录；任何允许 Bearer 调用的路由不得落入 `system:unmapped`。
4. 服务端提供不含 Client Secret 的内置第一方公共 CLI Client、PKCE、Device Code、Refresh 和 Revoke 完整流程。
5. Step-up MFA、终端预授权、终端存活监控和数据导出票据支持 OAuth Bearer 认证上下文，不再仅绑定浏览器 Session。
6. Node.js npm 包与 Bun 单二进制通过同一 `HttpTransport` 契约，代理、自定义 CA、重定向、取消、SSE、WebSocket 和二进制下载行为一致。
7. 每个路由完成 `business-command`、`protocol-adapter`、`client-entry`、`server-entry` 或 `internal-observability` 分类，并由 CI 拒绝未分类的新路由。

以下项目属于 P1，但缺失时对应能力不得宣称可用：

- Git Provider OAuth 使用短时授权事务闭环，而不是通过账号列表变化猜测结果。
- macOS 稳定二进制进入正式下载矩阵前具备对应签名、安装和真实 smoke test；Windows 与 Alpine/musl 第一阶段使用 npm/pnpm + Node.js。
- 第三方 Provider 每个 operation 具备确定性 fixture，每个 Provider 家族至少保留一个真实环境 smoke。

因此，推荐实施顺序是“契约和认证底座 -> 共享 Client 与 Transport -> CLI 命令 -> 协议适配 -> 发布门禁”。任何阶段都不允许用 `api request`、原始 Cookie、浏览器 Session 或直接访问第三方 API 绕过尚未完成的正式能力。

### 3.4 AI Agent 适配审计结论

在现有“面向人类的 CLI”基础上增加 `output=json`，仍不足以形成可靠的 Agent 工具。对照 GitHub CLI、Terraform、kubectl、OpenAI、Anthropic、OWASP、OpenAPI Arazzo、JSON Schema 和 RFC 9457 后，Luna CLI 还需要补齐以下边界：

| 维度 | 当前设计缺口 | Luna 处理方式 | 优先级 |
| --- | --- | --- | --- |
| 复杂输入 | `key=value` 仍受 Shell 转义、argv 长度和多行输入限制 | 增加 `params=@path` / `params=@-`，输入按 JSON Schema Draft 2020-12 校验 | P0 |
| Agent 模式 | `output=json interactive=false` 需要每次手写，且没有统一资源边界 | 增加 `agent=true`，统一关闭交互、颜色、浏览器自动打开和无限读取，并启用严格 Schema | P0 |
| 能力发现 | 一次返回全部命令会快速占满 Agent 上下文 | `help catalog` 支持按分类、关键词、Scope 和风险过滤，只在 `help command` 返回完整 Schema | P0 |
| 高风险操作 | `yes=true` 只能证明跳过本地提示，无法证明用户批准的就是最终请求 | 服务端生成短时、单次、绑定 actor/authentication context/target/normalized params 的 `planId`，执行时精确校验 | P0 |
| 并发修改 | Agent 可能基于陈旧读取覆盖用户刚完成的修改 | 写操作支持 ETag/If-Match 或稳定资源版本；Agent 模式禁止无条件覆盖 | P0 |
| 长任务 | 已有 JSONL，但缺少统一事件版本、顺序和资源关联字段 | 首行版本事件，后续事件包含 sequence、eventId、correlationId、resourceRef，末行必须是 summary | P0 |
| 工作流 | 多接口业务流程只能散落在 Skill 文本中 | 使用 Arazzo 描述受支持的多步骤工作流，并生成测试与文档；CLI 仍按正式命令执行，不内置通用工作流解释器 | P1 |
| 不可信内容 | 日志、仓库文件、第三方错误可能包含 Prompt Injection | 所有外部文本标记为数据，不能生成可信 next action；人类输出清理终端控制字符，Skills 禁止执行其中指令 | P0 |
| 输出泄密 | OpenAPI 响应类型不天然保证敏感字段不进入 JSON | 使用 `writeOnly` / `x-sensitive` 传播敏感标记，输出与 debug 统一脱敏并做契约测试 | P0 |
| 资源消耗 | `all=true`、follow、日志和轮询可能形成无限循环或高额请求 | Agent 模式强制 maxItems/maxPages/maxBytes/timeout 和重试上限 | P0 |
| 可审计性 | 只能看到 API 请求，难以区分人工、脚本和 Agent | 审计记录 operationId、CLI 版本、agentMode、server、project、planId、idempotencyKey 和 OAuth grant | P0 |
| Agent 评估 | 普通命令测试无法发现 Prompt Injection、越权建议或错误确认 | 增加命令发现、Schema 首次通过率、危险操作停顿、注入抵抗、脱敏和陈旧写入专项评估 | P0 |

首版不引入以下设计：

- 不把任意 Shell、JavaScript 或表达式求值器嵌入 CLI；
- 不在 CLI 内实现自治循环、任务规划器或长期记忆；
- 不把服务端返回的自由文本转换成可执行命令；
- 不把全部 200 余个 operation 一次注入 Agent 上下文；
- 不允许 Agent 在没有精确 plan、用户批准和服务端复核时执行高风险动作。

CLI 的定位保持为“受 Schema、权限和服务端策略约束的确定性执行层”，Skills 才是薄编排层，Agent 负责理解用户目标但不拥有额外权限。

## 4. 技术选型

### 4.1 最终选择

| 层级 | 选择 | 用途 |
| --- | --- | --- |
| 语言 | TypeScript | CLI、共享 API Client 和命令元数据 |
| 工作区与依赖 | pnpm workspace | 遵循项目包管理规范并统一锁文件 |
| 单二进制编译 | Bun `build --compile` | 生成包含运行时的独立可执行程序 |
| 命令解析 | Commander.js | 子命令、参数校验、帮助和别名 |
| 交互提示 | `@inquirer/prompts` | 选择、确认、密码和 OTP 输入 |
| API 类型 | `openapi-typescript` | 从 OpenAPI 生成 TypeScript 类型 |
| HTTP Client | `openapi-fetch` | 环境无关、轻量、类型安全的 Fetch Client |
| 传输层 | `HttpTransport` + `ws` + `eventsource-parser` | 统一 Node/Bun 的 HTTP、代理、自定义 CA、WebSocket 与 SSE 行为 |
| 运行时校验 | Zod | 本地配置、命令输入和关键响应边界校验 |
| 国际化 | i18next | 内置中英文资源和语言回退 |
| YAML | `yaml` | YAML 输入与输出 |
| 测试 | Vitest | 单元测试、契约测试和命令快照测试 |

### 4.2 为什么选择 Commander.js

Commander.js 提供稳定的嵌套命令、选项解析、自动帮助和 TypeScript 类型，依赖和运行时模型简单，适合编译为单二进制。

oclif 适合大型插件化 CLI，但其插件发现、动态加载和 Node.js LTS 运行时模型与第一版的静态单二进制目标不匹配。Luna CLI 暂时不需要插件系统，因此不采用 oclif。

命令必须通过统一注册表声明，不允许在各文件中分别手写帮助、Completion 和 AI Schema。命令注册表同时驱动：

- Commander 命令树；
- `--help` 文本；
- Shell Completion；
- `luna help catalog agent=true`；
- API 覆盖检查；
- 中英文文案键。

### 4.3 双分发模型

Bun 官方支持通过 `bun build <entry> --compile` 生成包含 Bun 运行时的单文件程序，并支持交叉编译 macOS、Linux 和 Windows 的主要架构。因此 Luna CLI 可以采用 Bun 发布单二进制。

CLI 使用同一份 TypeScript 源码生成两类制品：

| 分发渠道 | 制品 | 运行要求 | 适用场景 |
| --- | --- | --- | --- |
| npm Registry | ESM JavaScript CLI 包 | Node.js `>=22.14.0` | 已有 Node.js 工具链、希望通过 npm/pnpm 更新 |
| GitHub Release | Bun 编译的单二进制 | 无需 Node.js 或 Bun | 服务器、容器、CI、最小运行环境 |

不采用 npm `postinstall` 下载 GitHub Release 二进制，也不为每个平台发布一组 `optionalDependencies` 子包。前者容易被 `--ignore-scripts`、代理和安装期网络策略阻断，后者会放大包数量、版本同步与 Trusted Publisher 配置成本。npm 包直接运行已构建的 JavaScript，独立二进制由 GitHub Release 分发，两者共用命令注册表、API Client、版本号和测试。

约束如下：

- pnpm 仍是仓库唯一依赖管理和 workspace 工具，Bun 只负责 CLI 编译与必要的运行时兼容测试。
- npm 包仅作为发布制品和用户安装入口；仓库开发、依赖更新与锁文件维护仍统一使用 pnpm。
- Bun 版本必须在 CI 中精确固定，不能使用浮动 `latest`。
- CLI 依赖避免原生 Node Addon、运行时下载、动态 `require`、动态插件扫描和依赖源码目录。
- i18n 资源、OpenAPI 类型、模板和版本信息在编译期打入二进制。
- 每个平台的产物必须实际执行 `luna version show agent=true`、`luna help catalog query=project limit=5 agent=true` 和配置读写 smoke test。
- npm tarball 必须分别通过 npm 和 pnpm 安装 smoke test，并执行与二进制相同的版本与 Help 契约测试。
- `luna version show output=json` 必须额外返回 `distribution` 和 `runtime`，便于诊断 npm 包与独立二进制差异；业务能力和 JSON Schema 不得因分发渠道变化。
- 单二进制包含运行时，体积会大于 Go CLI，但换来 TypeScript 共享契约和零运行时安装，第一版可以接受。

## 5. Monorepo 结构

仓库升级为统一 pnpm workspace，建议结构如下：

```text
.
├── package.json
├── pnpm-workspace.yaml
├── pnpm-lock.yaml
├── cli/
│   ├── package.json
│   ├── README.md
│   ├── bin/
│   │   └── luna.js
│   ├── dist/
│   ├── src/
│   │   ├── entry.ts
│   │   ├── commands/
│   │   ├── auth/
│   │   ├── config/
│   │   ├── output/
│   │   ├── i18n/
│   │   └── errors/
│   └── tests/
├── packages/
│   ├── api-contract/
│   │   ├── generated/
│   │   └── command-catalog.ts
│   └── api-client/
│       ├── client.ts
│       ├── auth.ts
│       └── errors.ts
├── web/
├── docs/
└── tests/
```

职责边界：

- `packages/api-contract`：OpenAPI 生成类型、Scope 目录、命令覆盖目录，以及从语言无关错误目录生成的 TypeScript 错误类型。
- `packages/api-client`：依赖抽象的 `HttpTransport`，不读取浏览器、终端或本地文件；普通 JSON Client 不感知具体 Node/Bun 网络实现。
- `web`：提供 Cookie、浏览器 MFA Dialog 和 UI 错误映射适配器。
- `cli`：提供 Node/Bun Transport、Bearer Token、本地凭据、终端 MFA 和输出适配器。

禁止 CLI 直接导入 `web/src/api`。共享逻辑应从 Web 中抽取到 `packages/`，浏览器专属逻辑继续留在 Web。

## 6. 命令设计

### 6.1 命名规则

所有可执行命令采用固定两级结构：

```text
luna <tool-category> <tool> [key=value ...] [global-flags]
```

其中：

- `tool-category` 是稳定的工具分类，例如 `project`、`build`、`gateway`；
- `tool` 是分类下的具体工具，例如 `list`、`get`、`create`、`logs`；
- 业务输入统一使用无序 `key=value`，不使用依赖位置的业务参数；
- 全局 flag 只控制实例覆盖、项目空间、输出、语言、交互、超时和调试，不承载 API 业务字段；
- `luna --help` 和 `luna --version` 是根级快捷入口，不属于业务命令。
- 人类交互额外提供 `luna login`、`luna logout`、`luna whoami` 和
  `luna doctor`，分别委托给 `auth login`、`auth logout`、`auth status` 和
  `health doctor` 的同一处理器，不复制业务逻辑。

示例：

```bash
luna project list page=1 pageSize=20
luna application get id=my-app project=my-space
luna build run-trigger application=my-app target=production
luna release runtime-logs id=release-id follow=true
luna gateway route-create application=my-app hostname=example.com
```

分类名使用单数，并可提供常用短别名：

- `application`，别名 `app`；
- `deployment`，别名 `deploy`；
- `data-retention`，别名 `retention`；
- 分类别名只用于人工输入，不得出现在脚本示例、机器可读 Help、审计记录或 AI Skills 中；
- ID、固定标识和唯一名称均可作为资源定位值；存在歧义时返回候选项并要求使用 `id=<stable-id>`。

同一个 API operation 只能有一个 canonical command path。别名只用于人工输入，机器可读 Help、审计和 Skills 始终返回 canonical path。

顶层短命令同样只允许人工输入。启用 `agent=true` 时必须返回
`agent_alias_forbidden`；自动化、文档中的机器示例、审计和 Skills 始终记录
canonical 两级命令。

### 6.2 `key=value` 输入语法

解析规则：

1. 每个业务参数必须包含第一个 `=`，第一个 `=` 左侧是 key，右侧完整保留为 value。
2. key 必须存在于该工具的输入 Schema；未知 key 直接返回退出码 `2`。
3. 参数顺序不影响语义。
4. 同一 key 仅在 Schema 声明为数组时允许重复。
5. 字符串中的空格、通配符、`$`、引号和换行仍遵循当前 Shell 的转义规则。
6. key 区分大小写并统一使用 camelCase；只允许 `[A-Za-z][A-Za-z0-9]*`。

示例：

```bash
luna project create identifier=my-team name="My Team"
luna access-token create name=ci scope=project:read scope=build:trigger expiresAt=null
luna gateway route-create application=my-app hostname=app.example.com path=/
```

类型由命令注册表和 OpenAPI Schema 决定，不通过猜测转换：

| Schema 类型 | CLI 输入 |
| --- | --- |
| `string` | 原样 UTF-8 字符串 |
| `boolean` | 仅接受 `true`、`false` |
| `integer` / `number` | 十进制，拒绝 `NaN` 和无穷值 |
| `enum` | 只接受 Help 中列出的稳定枚举值 |
| `date-time` | RFC 3339 |
| `duration` | Go 风格 duration，例如 `30s`、`5m`、`2h` |
| `array` | 重复 key，或使用 JSON 数组文件 |
| `object` | JSON 字符串、JSON/YAML 文件或 stdin |
| `binary` | 只允许文件输入，不允许内联 |

#### 多行和复杂输入

支持三种 value source：

```text
key=value   内联值
key=@path   从文件读取
key=@-      从 stdin 读取
key=@@value 以 @ 开头的字面字符串，解析结果为 @value
```

示例：

```bash
luna notification template-create name=build-failed body=@template.md
cat template.md | luna notification template-create name=build-failed body=@-
luna deployment target-update id=deploy-prod runtimeConfig=@runtime-config.yaml
luna api request method=POST path=/api/v1/example body=@payload.json
```

行为要求：

- `@path` 读取原始字节；按 Schema 和文件扩展名解析 JSON/YAML，字符串字段保留换行。
- `@-` 每条命令最多只能出现一次，避免多个字段争用 stdin。
- `@@` 只用于转义内联字符串开头的 `@`，不能绕过 Secret 或二进制字段的输入限制。
- stdin 非 TTY 时读取到 EOF；TTY 中使用 `Ctrl-D` 结束，不自行打开编辑器。
- Secret、Token、密码、恢复码和 kubeconfig 不允许使用内联 `key=value`，只能使用安全提示、`@-` 或权限合规的文件。
- 二进制字段必须通过 `@path` 读取，JSON 输出只返回元数据，不回显内容。
- 跨平台脚本中，内联 value 超过 4 KiB 或包含换行时必须改用文件或 stdin，避免 Windows、macOS 和 Linux 的 argv 上限差异；CLI 已启动并收到超限值时返回 `inline_value_too_large`。
- 每个字段的 `maxLength`、`maxItems`、`maximum` 和请求体大小上限必须进入机器可读 Help，并在发请求前校验。

#### 面向 Agent 的完整参数对象

`key=value` 适合人类输入简单参数。AI Agent、复杂对象和包含大量可选字段的写操作优先使用完整参数对象：

```bash
luna application create params=@payload.json agent=true
printf '%s' '{"project":"prj_example","identifier":"api","name":"API"}' \
  | luna application create params=@- agent=true
```

规则如下：

- `params` 只接受 JSON object；文件扩展名为 `.yaml` / `.yml` 时允许 YAML，并在校验前转换成 JSON 数据模型。
- 参数对象使用 `help command` 返回的 JSON Schema Draft 2020-12 Schema 校验，Schema 必须具有稳定 `$id`、`schemaVersion` 和 digest。
- `params` 与业务 `key=value` 不得混用，避免同一字段出现两套覆盖顺序；`server`、`project`、`output`、`agent`、`timeout` 等全局控制仍可同时使用。
- `params=@-` 占用 stdin 后，Schema 中的 Secret、二进制或第二个多行字段必须改用文件引用，不能再次读取 stdin。
- JSON 中的未知字段默认拒绝；只有 Schema 显式声明 `additionalProperties: true` 的开放映射允许额外 key。
- 命令历史、进程列表和错误不得包含参数对象原文。含 `writeOnly` 或 `x-sensitive: true` 字段的对象在 debug、plan 和审计中统一脱敏。
- `help command path=<category.tool>` 同时返回 `inputSchema`、`outputSchema`、`errorSchema`、Schema digest 和最小示例；Agent 不从自然语言帮助反推字段类型。

第一版不内置 `jq`、JMESPath 或通用表达式求值器。Agent 直接消费 JSON；人类需要筛选时可使用外部 `jq`。CLI 可在 P1 增加由输出 Schema 验证的 `fields=` 投影，但不得让表达式执行影响请求权限或副作用。

### 6.3 参数校验

校验分四层执行：

1. **词法校验**：两级命令、`key=value` 格式、重复 key 和 value source。
2. **Schema 校验**：必填、类型、枚举、格式、长度、范围、数组项和对象结构。
3. **跨字段校验**：互斥、至少一个、条件必填和组合约束。
4. **服务端校验**：权限、资源状态、唯一性和并发条件。

CLI 必须一次返回当前输入中全部可确定的本地校验错误，而不是一次只报一个。机器输出示例：

```json
{
  "error": {
    "code": "invalid_arguments",
    "status": 400,
    "fields": [
      {
        "key": "replicas",
        "code": "minimum",
        "expected": 1,
        "actual": 0
      }
    ]
  }
}
```

错误不得包含 Secret 原值。服务端返回的字段错误继续使用稳定 code，并映射回对应 key。

### 6.4 全局参数

全局控制既支持 `key=value` canonical 形式，也提供常用 flag 作为人工快捷方式：

| Canonical 参数 | 快捷 flag | 含义 |
| --- | --- | --- |
| `server=<url>` | `--server <url>` | 临时覆盖实例地址 |
| `project=<id-or-identifier>` | `--project <value>` | 临时覆盖项目空间 |
| `output=<format>` | `-o, --output <format>` | `table`、`json`、`raw-json`、`yaml`、`jsonl`、`name` |
| `lang=<locale>` | `--lang <locale>` | 临时覆盖语言 |
| `color=false` | `--no-color` | 禁用颜色 |
| `interactive=false` | `--no-interactive` | 禁止任何交互提示 |
| `yes=true` | `-y, --yes` | 确认危险操作 |
| `quiet=true` | `--quiet` | 关闭非必要 stderr 信息 |
| `agent=true` | `--agent` | 启用严格、非交互、有界的 Agent 执行策略 |
| `dryRun=<mode>` | `--dry-run <mode>` | `client` 生成本地请求预览，`server` 请求服务端校验且不持久化 |
| `timeout=<duration>` | `--timeout <duration>` | 请求或等待超时 |
| `debug=true` | `--debug` | 输出脱敏后的请求调试信息 |
| `requestId=<id>` | `--request-id <id>` | 为链路关联和诊断指定请求 ID，不提供幂等语义 |
| `idempotencyKey=<key>` | `--idempotency-key <key>` | 为支持幂等的写操作指定键 |
| `insecureSkipTlsVerify=true` | `--insecure-skip-tls-verify` | 临时跳过 TLS 证书校验并持续警告 |

优先级固定为：

```text
命令内 canonical 参数或快捷 flag > LUNA_* 环境变量 > 活动登录配置 > TTY 自动模式 > 内置默认值
```

同一个全局控制同时使用 canonical 参数和 flag 且值不一致时，直接报参数冲突，不按顺序覆盖。

这些 canonical key 是全局保留字，业务工具不能以不同语义重新定义。`project` 始终表示当前项目空间覆盖值；需要表达其他项目字段时必须使用更具体的名称。

`agent=true` 是一组安全策略，不是新的输出格式：

- 普通命令固定使用 `output=json`，流式命令固定使用 `output=jsonl`；
- 固定 `interactive=false`、`color=false`、`quiet=true`，不打开浏览器、不读取 `/dev/tty`；
- 变更项目级资源时必须显式传入不可变 `project` ID，不允许仅依赖持久活动登录的默认项目；
- `all=true`、follow、日志、轮询和异步等待必须设置或接受命令元数据声明的硬上限；
- 禁止 `raw-json`，禁止把 debug 输出当作机器契约；
- 成功输出、错误输出或事件流出现不符合声明 Schema 的内容时 fail closed；
- `LUNA_AGENT=1` 可为受控运行环境设置默认值，但 Skills 仍应在每条命令显式传入 `agent=true`。

`dryRun=client` 只验证本地 Schema 并输出标准化请求预览，不能声称服务端接受该请求。`dryRun=server` 对应类似 kubectl Server Dry Run 的权威服务端校验；每个 operation 必须声明支持 `none`、`client`、`server` 或 `both`。高风险变更不能用 client dry-run 替代服务端 plan。

`requestId` 只用于链路关联、日志和审计查询，不能替代 `idempotencyKey`。重试写操作时，CLI 只能在服务端明确声明支持幂等且已生成或收到 `idempotencyKey` 时自动重试。

项目空间单独使用以下确定性解析顺序：

```text
project=<value> / --project <value>
> LUNA_PROJECT
> 活动登录的 project.id
> project_required
```

`LUNA_PROJECT` 只覆盖当前进程，不写入配置。CLI 不得自动选择“第一个项目空间”或“唯一项目空间”，避免项目列表变化后命令静默作用于其他资源。

### 6.5 工具分类与完整目录

复合资源使用 `<subject>-<verb>` 作为一个 tool 名，例如 `git provider-list` 和 `notification channel-create`。单一资源分类可直接使用 `list`、`get`、`create`、`update`、`delete`。任何业务能力都不得继续追加第三级子命令。

第一版业务目录如下。最终可执行项以 OpenAPI 生成的机器目录为准，但实现不得缩减这里列出的公开控制面能力。

本目录已按 2026-07-24 的 `internal/api/router.go` 公开路由快照校对。进入实现阶段后，OpenAPI 与 `x-luna-cli` 元数据是唯一事实来源；路由、OpenAPI 和命令目录发生漂移时 CI 必须失败。

| 分类 | 具体工具 |
| --- | --- |
| `auth` | `login`、`logout`、`status`、`refresh`、`bootstrap-status`、`bootstrap-admin`、`registration-status`、`registration-code-request`、`registration-complete`、`registration-settings-get`、`registration-settings-update`、`mfa-status`、`mfa-enroll`、`mfa-confirm`、`mfa-verify`、`mfa-recovery-regenerate`、`mfa-disable`、`provider-list`、`provider-callback-url`、`provider-create`、`provider-update`、`admission-get`、`admission-update` |
| `user` | `me`、`profile-update`、`password-update`、`identity-list`、`identity-unbind`、`list`、`create`、`update`、`mfa-reset` |
| `oauth` | `app-list`、`app-create`、`app-update`、`app-secret-rotate`、`app-delete`、`grant-list`、`grant-revoke` |
| `config` | `public-get`、`definition-list`、`get`、`update` |
| `data-retention` | `catalog`、`preview`、`cleanup` |
| `git` | `provider-list`、`provider-create`、`provider-update`、`provider-delete`、`authorize`、`account-list`、`account-create`、`account-update`、`account-delete`、`account-refresh`、`repository-list`、`branch-list`、`build-options`、`content-list`、`file-read`、`binding-list`、`binding-create`、`binding-update`、`binding-delete`、`webhook-create`、`webhook-reconfigure` |
| `registry` | `list`、`create`、`update`、`delete`、`test`、`image-template-default`、`repository-search`、`tag-list`、`credential-list`、`credential-list-all`、`credential-create`、`credential-update`、`credential-delete` |
| `image` | `list`、`record` |
| `build` | `variable-set-list`、`variable-set-create`、`variable-set-update`、`variable-set-delete`、`environment-get`、`environment-update`、`template-list`、`template-preview`、`run-list`、`run-trigger`、`run-get`、`run-retry`、`run-cancel`、`run-delete`、`job-list`、`job-get`、`job-logs`、`job-logs-follow` |
| `cluster` | `list`、`create`、`update`、`delete`、`test`、`resource-list`、`resource-delete`、`resource-yaml`、`resource-events`、`pod-terminal` |
| `component` | `list`、`install` |
| `notification` | `preset-list`、`channel-from-preset`、`channel-list`、`channel-create`、`channel-update`、`channel-delete`、`channel-test`、`template-list`、`template-create`、`template-update`、`template-delete`、`rule-list`、`rule-create`、`rule-update`、`rule-delete`、`delivery-list` |
| `event` | `list`、`catalog`、`get` |
| `dashboard` | `get` |
| `app-template` | `list`、`install` |
| `project` | `current`、`use`、`unset`、`list`、`get`、`create`、`update`、`delete`、`pin-list`、`pin`、`unpin`、`order-update`、`default-registry`、`topology`、`member-list`、`member-candidate-search`、`member-create`、`member-update`、`member-delete`、`service-binding-list`、`service-binding-create`、`service-binding-update`、`service-binding-delete`、`service-binding-check`、`topology-edge-list`、`topology-edge-create`、`topology-edge-update`、`topology-edge-delete` |
| `runtime-config` | `list`、`create`、`update`、`delete` |
| `hook` | `config-list`、`config-create`、`config-update`、`config-delete`、`run-list`、`run-logs` |
| `application` | `list`、`get`、`create`、`update`、`delete`、`topology` |
| `deployment` | `target-list`、`target-create`、`target-update`、`target-delete`、`target-restart`、`metrics-follow`、`data-export` |
| `release` | `list`、`create`、`image-candidate-list`、`logs`、`runtime-logs`、`exec`、`terminal`、`rollback` |
| `gateway` | `route-list`、`route-create`、`route-update`、`route-delete`、`domain-check` |
| `billing` | `summary`、`deployment-spend`、`ledger-list`、`usage-list`、`rate-list`、`rate-update`、`wallet-transaction-create`、`external-transaction-create`、`gateway-traffic-status` |
| `access-token` | `scope-list`、`list`、`create`、`revoke` |
| `api` | `request` |
| `completion` | `bash`、`zsh`、`fish`、`powershell` |
| `help` | `catalog`、`command` |
| `version` | `show` |

管理员工具仍放在对应资源分类下，通过服务端 Scope 和角色鉴权，不复制一套 `admin` API，也不提供能绕开统一权限判断的管理员别名。

每个分类至少提供一个具体工具。`help catalog`、`help command`、`version show` 等本地工具也遵守两级结构。

### 6.6 命令生成规则

- HTTP method 和路径不能直接决定命令名；`x-luna-cli.category` 与 `x-luna-cli.tool` 是唯一命令映射。
- 命令注册表的来源必须标记为 `openapi`、`protocol` 或 `local`：
  - `openapi`：一个业务命令绑定一个 OpenAPI operation；
  - `protocol`：OAuth 登录、SSE 跟随、WebSocket 终端、二进制下载等由多个 operation 或协议步骤组成的高层命令；
  - `local`：项目默认值、help、completion 和 version 等不调用服务端的命令。
- 同一 `category.tool` 只能有一个注册项；`openapi` 来源必须绑定唯一 operation，`protocol` 来源必须列出其消费的全部 operation，发生冲突时生成阶段直接失败。
- path、query、header 和 body 字段统一扁平映射为 `key=value`；重名字段必须在 OpenAPI 元数据中显式重命名。
- 请求体对象可按字段展开，也可由一个声明为 object 的参数通过 `@file` 或 `@-` 整体传入，两种方式不能混用。
- 流式、下载和终端类接口必须在元数据中声明专用输出适配器，不能伪装成普通 JSON 响应。
- Tool 的参数 Schema、Help、Completion、Client 调用和测试样例都从同一注册表生成，不维护手写副本。
- `dryRun`、幂等键、自动重试和自动分页只能在 operation 元数据明确支持时启用。客户端本地生成的请求预览必须叫 `requestPreview`，不能冒充服务端校验。
- 每个 operation 必须声明认证模式、权限、资源作用域、危险级别、响应适配器、分页、异步等待、幂等和重试能力；生成器不能依赖命令名猜测这些行为。

### 6.7 全接口覆盖

覆盖报告先从 Gin 路由生成完整清单，再把每条路由分类为：

| 路由分类 | 定义 | CLI 要求 |
| --- | --- | --- |
| `business-command` | 面向用户或管理员的公开控制面 API | 必须进入 OpenAPI，并映射高层命令 |
| `protocol-adapter` | OAuth、SSE、WebSocket、下载等用户会实际消费的协议端点 | 必须进入 OpenAPI，并由高层协议命令完整消费 |
| `client-entry` | 浏览器登录、登出、OIDC 发起/回调等客户端入口 | 必须进入 OpenAPI 和共享低层 Client，由 Web 或 CLI 的等价认证流程消费；不得为 Cookie/浏览器专属步骤伪造高层命令 |
| `server-entry` | 第三方系统或探针主动调用的接收端点 | 必须进入 OpenAPI 和共享低层 Client，登记用途、鉴权、Owner 和不提供 CLI 命令的理由，并执行协议与安全测试 |
| `internal-observability` | 健康检查、metrics 或仅内部进程使用的端点 | 必须进入显式 allowlist，并由测试防止误分类公开业务 API |

“支持所有 API”采用三层口径：

1. **契约层**：除 `internal-observability` 白名单外，所有 HTTP API 都进入 OpenAPI 和共享低层 Client。
2. **能力层**：所有 `business-command` 与 `protocol-adapter` 都有高层 CLI 命令或协议适配器；同一用户目标允许由多个 operation 共同完成。
3. **入口层**：`client-entry` 与 `server-entry` 不伪造成用户命令，但必须具备完整契约、Owner、鉴权和协议/安全测试。

以下端点不作为独立用户命令，但仍必须被路由清单分类：

- 第三方 Webhook 接收端点；
- Web 本地账号登录、浏览器 Session 登出、OIDC 发起与回调等浏览器入口；CLI 使用 OAuth/Device Code 提供等价认证能力，不暴露 Cookie Session 命令；
- OAuth 回调和授权确认页；
- OAuth token、revoke 和 authorize 等协议端点；它们由 `auth` / `oauth` 高层工具内部使用；
- 探针上报；
- 健康检查和 metrics；
- 只供内部 Worker 调用的接口。

这里的“没有独立用户命令”不等于 Client 遗漏：除显式 allowlist 中的内部可观测性端点外，所有 HTTP API 都必须进入 OpenAPI 和环境无关的共享低层 Client。CLI 的高层命令层只对用户有意义的业务与协议能力负责，不能为了数字覆盖给 Webhook 接收端或 OAuth callback 伪造误导性的人工命令。

OpenAPI 中每个操作必须具有：

```yaml
operationId: listProjects
x-luna-cli:
  source: openapi
  category: project
  tool: list
  visibility: public
  routeClass: business-command
  authentication:
    mode: required
  projectContext:
    mode: optional
    inject:
      source: query
      name: projectId
  parameters:
    page:
      source: query
      schema:
        type: integer
  output: ProjectListResponse
  requiredScopes:
    - project:read
  transport:
    kind: json
  pagination:
    kind: page
  idempotency:
    supported: false
```

`x-luna-cli.projectContext.mode` 必须显式声明：

| 值 | 行为 |
| --- | --- |
| `required` | 命令必须解析出项目空间；没有显式值、环境变量或活动登录默认值时返回 `project_required` |
| `optional` | 有项目空间时作为筛选或作用域注入，没有时保持接口原本的跨项目语义 |
| `none` | 命令不接受项目空间上下文；传入 `project` 时返回参数不支持错误，避免制造已限定作用域的错觉 |

`projectContext` 不能只声明枚举，还必须声明解析结果注入到 `path`、`query`、`body` 的具体字段，或者声明为 `resolve-only`。命令生成器根据该元数据统一注入项目参数、Help 和校验，不允许各业务命令自行读取活动登录配置，也不能向原本没有项目参数的接口凭空追加 query。

`api request` 只用于诊断和新接口开发期间的临时验证：

```bash
luna api request method=GET path=/api/v1/example page=1
luna api request method=POST path=/api/v1/example body=@payload.json
cat payload.json | luna api request method=POST path=/api/v1/example body=@-
```

它不计入公开 API 命令覆盖率，也不能作为缺失高层命令的发布兜底。AI Skills 默认禁止使用 `api request`，除非用户明确要求调试原始 API。

CI 必须生成路由、OpenAPI、命令和协议消费四张可关联清单。每个路由和 OpenAPI operation 只能处于以下状态之一：

- 已映射到唯一业务命令；
- 已被唯一协议适配器消费；
- 已登记为客户端入口，并注明消费方、Owner、鉴权和等价 CLI 能力；
- 已登记为服务端入口，并注明 Owner、鉴权和排除理由；
- 已登记为内部可观测性接口，并位于审核过的 allowlist。

不允许存在未分类路由、未记录的公开 operation、孤立 operation、命令冲突或协议步骤缺失。覆盖门禁为：

```text
路由分类率 = 已分类 Gin 路由 / 全部 Gin 路由 = 100%
HTTP API OpenAPI 覆盖率 = OpenAPI 中的非白名单路由 / Gin 非内部可观测白名单路由 = 100%
高层能力覆盖率 = 已映射命令或协议适配器的可命令化 operation / 全部 business-command 与 protocol-adapter operation = 100%
Bearer Scope 映射率 = 具有稳定非 system:unmapped Scope 的 Bearer 可调用路由 / 全部 Bearer 可调用路由 = 100%
关键旅程通过率 = 通过的关键旅程 / 全部关键旅程 = 100%
完整场景通过率 = 通过的操作场景 / 可执行操作场景 >= 95%
```

“不可执行”只能用于依赖当前测试环境不具备的外部系统，并必须提前标记环境条件；代码缺陷、未实现、鉴权失败或输出不稳定不能从分母中排除。

覆盖清单必须以机器可读文件落盘并由 CI 生成，至少包含：

```text
method
path
normalizedPath
operationId
routeClass
owner
authentication
requiredScopes
runtimeRequiredScope
canonicalCommand
protocolAdapter
consumedBy
testScenarioIds
exclusionReason
```

Gin 的 `:id` 与 OpenAPI 的 `{id}` 必须先归一化为同一个 `normalizedPath`，再按 `method + normalizedPath` 比较；不能用路径字符串直接相等或单纯比较总数计算覆盖率。

`exclusionReason` 只允许用于 `client-entry`、`server-entry` 与 `internal-observability`，不能把尚未实现的业务命令标成排除项。客户端入口必须验证回调 state、重定向白名单、Cookie/Session 和错误恢复；服务端入口必须验证签名、鉴权、重放防护、限流和错误响应。两类入口虽然不计入高层命令覆盖率，但都必须计入路由分类率和 OpenAPI 覆盖率。

`requiredScopes` 不能由文档维护者重复手写成另一套事实来源。生成阶段必须把 OpenAPI 声明与服务端 `RequiredAccessTokenScope(method, path)` 的结果逐项比较：允许 Bearer 的业务与协议路由不得为空或为 `system:unmapped`，值不一致时 CI 直接失败；明确不接受 Bearer 的服务端入口必须声明其真实认证机制，不能靠 `system:unmapped` 偶然阻止调用。

`auth login` 是 Luna OAuth 协议命令，不直接调用 Web 的本地账号登录或 OIDC start/callback。后者仍属于 `client-entry` 并进入低层契约与安全测试。这样既完整覆盖平台 API，又避免 CLI 收集用户站点密码、管理 Cookie Session，或把一次浏览器 302 误报为 CLI 登录成功。

### 6.8 传输与响应适配器

CLI 至少提供以下统一适配器：

| 适配器 | 适用接口 | 必须处理 |
| --- | --- | --- |
| `json` | 普通 REST API | 状态码、稳定错误、分页、Scope、请求 ID |
| `sse` | 构建日志、部署指标 | 增量事件、断线重连、服务端游标、SIGINT、空闲超时 |
| `websocket-terminal` | Pod 与发布终端 | 原始终端模式、stdin/stdout、resize、ping/pong、退出状态、信号恢复 |
| `binary-download` | 数据导出 | `Content-Disposition`、文件名净化、覆盖确认、临时文件原子重命名、stdout 模式 |
| `oauth-protocol` | authorize、token、revoke、Device Code | PKCE、state、轮询间隔、刷新旋转、吊销与错误码 |

所有适配器必须建立在统一 `HttpTransport` 上。Transport 至少负责：

- base URL、请求超时、取消、有限重定向和跨源时移除 `Authorization`；
- `HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY`、实例级代理与自定义 CA；
- Bearer 注入、401 单次刷新、请求 ID、`Retry-After` 和脱敏诊断；
- 在 Node.js npm 制品和 Bun 二进制中返回一致的状态码、Headers、流和错误类型。

CLI 不直接依赖浏览器全局 `EventSource` 或 `WebSocket`。SSE 使用 Fetch 流配合 `eventsource-parser` 解析，WebSocket 使用 `ws` 或经过等价兼容性验证的集中适配器；具体依赖版本必须进入 pnpm 锁文件和供应链扫描。任何业务命令不得自行选择网络库、代理 Agent 或 CA 加载方式。

OpenAPI 继续描述 HTTP 握手和普通响应；SSE event 名称/data、WebSocket client/server frame、关闭码和二进制下载 Headers 另外维护版本化协议 Schema，并通过 `x-luna-cli.transport.schemaRef` 引用。协议 Schema 与实现漂移时 CI 失败，避免把自由文本流误当作稳定接口。

SSE 重连只有在服务端提供可恢复游标时才能保证不丢不重；没有游标的流必须明确标记 `resume=false`，断线后提示用户重新读取普通日志接口。WebSocket 终端退出时必须恢复 TTY 状态，即使进程收到 SIGINT、SIGTERM 或网络异常。

二进制下载默认写入由服务端文件名或命令参数确定的文件，已存在时拒绝覆盖；`destination=@-` 才把原始字节写到 stdout，此时禁止 JSON 包装、进度和其他文本混入。

同一套 Transport 与协议 fixture 测试必须分别运行于 Node.js `>=22.14.0` 的 npm tarball 和锁定 Bun 版本生成的二进制。二者任一在代理、自定义 CA、SSE、WebSocket、取消或重定向安全行为上不一致，都视为发布阻断，而不是标记为某个分发渠道的已知限制。

### 6.9 异步任务与等待

构建、发布、部署、Hook、网关和系统组件安装等异步命令必须由元数据声明：

- 返回的任务或资源 ID；
- 终态集合；
- 建议轮询间隔或事件流；
- 是否支持 `wait=true`、`follow=true`、`timeout=<duration>` 和取消；
- 超时是否只停止本地等待，还是同时请求取消服务端任务。

默认行为以服务端现有语义为准，不得假装同步成功。人类模式必须打印创建出的 ID 和后续查看命令；机器模式返回稳定状态。收到 SIGINT 时默认只停止本地等待，只有用户明确设置 `cancelOnInterrupt=true` 且接口支持取消时才取消远端任务。

长任务使用版本化 JSONL 事件协议。第一行必须是协议声明，后续事件必须可关联，最后一行必须是终态摘要：

```jsonl
{"type":"version","streamVersion":"cli.luna.devops/events/v1","cliVersion":"0.1.0","serverVersion":"0.1.0","schemaDigest":"sha256:..."}
{"type":"progress","sequence":1,"eventId":"evt_1","correlationId":"req_1","operationId":"build.wait","resourceRef":{"kind":"BuildRun","id":"bldr_1"},"occurredAt":"2026-07-25T10:00:00Z","data":{"phase":"building","percent":20}}
{"type":"summary","sequence":2,"eventId":"evt_2","correlationId":"req_1","operationId":"build.wait","resourceRef":{"kind":"BuildRun","id":"bldr_1"},"occurredAt":"2026-07-25T10:01:00Z","data":{"status":"succeeded","exitCode":0}}
```

约束：

- `type=version` 必须先于业务事件；未知主版本直接失败，未知事件类型在同一主版本内允许跳过并保留告警。
- `sequence` 在单条流内单调递增；`eventId` 用于去重，`correlationId` 关联一次 CLI 调用或服务端任务。
- 每条业务事件都带 `operationId` 和 `resourceRef`，不能要求 Agent 从自由文本推断事件属于哪个资源。
- 流正常结束前必须出现唯一 `summary`；缺少摘要、摘要与 HTTP/任务终态冲突或连接静默断开均视为不确定结果，不能报告成功。
- 服务端支持恢复时返回 `resumeCursor`，CLI 重连时携带游标并按 `eventId` 去重；不支持恢复时明确返回 `resume=false`。
- Agent 模式必须为等待命令设置 `timeout`、`maxEvents` 和 `maxBytes`；达到边界后返回结构化 `resource_limit_exceeded`，不得无限等待或无限积累输出。

### 6.10 计划、执行与并发保护

每个写操作的 `x-luna-cli` 元数据必须声明：

```yaml
risk: none | low | medium | high
confirmation: none | local | server-plan
sideEffects:
  - resource.write
estimatedCost: supported | unsupported
dryRun:
  - client
  - server
concurrency: none | resource-version | etag
```

`yes=true` 只表示调用方跳过本地提示，不能代替服务端审批。涉及删除、权限、Secret、凭据、kubeconfig、运行时终端、数据导出、账单、用户管理和其他高风险操作时，必须使用“服务端计划 -> 精确批准 -> 单次执行”：

```bash
luna cluster update params=@change.json plan=true agent=true
luna cluster update planId=plan_123 yes=true agent=true
```

计划响应至少包含：

```json
{
  "planId": "plan_123",
  "intentHash": "sha256:...",
  "actor": {"userId": "usr_1", "authContextId": "grant_1"},
  "executionScope": {"server": "https://devops.example.com", "projectId": "prj_1"},
  "target": {"kind": "RuntimeCluster", "id": "cluster_1"},
  "normalizedParams": {},
  "resourceVersions": {"cluster_1": "42"},
  "diff": [],
  "warnings": [],
  "requiredScopes": [],
  "mfaPurpose": "kubeconfig_update",
  "rollback": {"supported": false},
  "expiresAt": "2026-07-25T10:05:00Z"
}
```

执行时服务端必须重新校验 actor、授权上下文、执行作用域、目标、规范化参数、资源版本、策略、Scope、MFA、过期时间和单次使用状态，并验证 `intentHash`。任一字段变化都使旧计划失效；计划不能跨账号、实例、项目空间或 Token Family 复用，也不能只依靠 CLI 本地保存的摘要保护。

中高风险更新不得盲写。资源支持 ETag 时使用 `If-Match`，否则使用稳定 `version` 或 Kubernetes `resourceVersion`；冲突返回退出码 `6`，并在结构化错误中给出 `expectedVersion`、`currentVersion` 和重新读取建议。Agent 必须重新读取、重新生成计划并再次征得批准，不能自动覆盖。

批量操作的计划必须逐项列出目标、影响和预期版本。执行结果逐项返回，计划中没有出现的新增目标必须被服务端拒绝，防止名称解析结果或查询集合在批准后发生变化。

## 7. Help 与可发现性

### 7.1 人类帮助

```bash
luna --help
luna project --help
luna project create --help
luna help command path=project.create
luna help catalog all=true
```

每个命令的帮助必须包含：

- 一句话用途；
- 使用语法；
- 参数与默认值；
- 所需权限 Scope；
- 是否可能触发 MFA；
- 输入与输出格式；
- 至少一个常用示例；
- 危险操作说明；
- 相关命令。

### 7.2 AI 帮助

```bash
luna help catalog query=project category=project risk=low limit=20 agent=true
luna help command path=project.create agent=true
```

机器可读帮助必须使用版本化 Schema，并包含：

- category、tool、canonical path 和别名；
- 参数类型、必填状态、枚举和默认值；
- `key=value` 输入名、value source、stdin/file 支持和输入限制；
- 字段约束、跨字段约束和本地校验错误码；
- 输出 Schema；
- 退出码；
- 是否交互；
- 是否幂等；
- Scope 和 MFA purpose。

AI 不应通过解析彩色帮助文本理解命令。JSON Help 是 AI 的正式命令契约。

Agent 不应在每轮对话中加载全部命令。`help catalog` 必须支持 `query`、`category`、`risk`、`scope`、`transport`、`limit` 和游标分页，只返回紧凑目录项；Agent 选定候选命令后，再通过 `help command` 获取完整输入、输出、错误和安全 Schema。

机器目录响应必须包含 `catalogVersion`、`openapiDigest` 和 `schemaDigest`。同一任务内摘要发生变化时，Agent 必须重新发现命令；禁止继续使用缓存参数猜测新契约。目录中的 `nextActions` 只能引用受信任命令注册表中的稳定 command path 和参数 Schema，不能把服务端 message、日志或第三方文本拼成待执行命令。

### 7.3 Shell Completion

由命令注册表生成 Bash、Zsh、Fish 和 PowerShell Completion：

```bash
luna completion bash
luna completion zsh
luna completion fish
luna completion powershell
```

资源名称动态补全必须设置短超时，失败时静默回退到静态补全，不能阻塞 Shell。

## 8. 活动登录与凭据

### 8.1 配置文件

默认文件：

```text
~/.luna/auth.json
```

可通过 `LUNA_CONFIG` 覆盖。CLI 只保存一个活动实例和账号凭据，不维护命名
context、实例列表或账号列表。切换实例或账号时重新执行 `luna login`，新登录会
原子覆盖旧登录和默认项目空间。

```json
{
  "version": 2,
  "server": "https://devops.liteyuki.org",
  "credential": {
    "type": "oauth",
    "accessToken": "<secret>",
    "refreshToken": "<secret>",
    "expiresAt": "2026-07-24T12:00:00Z",
    "scopes": ["project:read", "application:update"],
    "user": {
      "id": "usr_example",
      "name": "Platform Admin"
    }
  },
  "project": {
    "id": "prj_example",
    "identifier": "team-platform",
    "name": "Platform Team"
  },
  "language": "",
  "output": ""
}
```

`language` 和 `output` 为空表示继续跟随系统与 CLI 默认值。`credential` 或
`project` 为 `null` 或缺失时分别表示尚未登录或未设置默认项目空间。服务端地址
始终规范化为 origin。

### 8.2 登录与切换

```bash
luna login
luna login server=https://devops.example.com
luna auth status
luna logout
```

行为要求：

- 裸 `luna login` 固定登录官方实例 `https://devops.liteyuki.org`，不沿用上一次
  自定义实例。
- `server=<url>` 可显式登录其他实例；登录成功后覆盖原活动实例、凭据和默认项目。
- 切换账号同样重新执行登录，不提供 `context use` 或 `auth switch`。
- `auth status` 默认隐藏 Token，只显示实例、凭据类型、到期时间、用户和 Scope。
- 服务端 URL 规范化为 origin，不允许携带用户名、密码、fragment 或非根路径，除非未来明确支持子路径部署。
- 读取旧版 version 1 多 context 配置时不选择或迁移任一凭据，直接初始化为空的
  version 2 官方实例配置并要求重新登录，避免选错账号或跨源迁移 Token。

### 8.3 当前项目空间

项目空间是当前活动登录的可选默认值。重新登录会清除它，避免把旧实例项目 ID
带到新实例：

```bash
luna project current
luna project use project=prj_example
luna project use project=team-platform
luna project unset
```

该能力收益较高，原因是应用、构建、部署、发布、访问入口和项目配置等高频命令都以项目空间为边界。它能明显减少人工重复输入，也让命令更短、更适合交互探索；代价是引入隐式作用域，因此必须通过可见解析结果、严格优先级和 AI/CI 显式项目参数控制风险。

行为要求：

- `project use` 先通过当前实例查询并校验项目空间可见性，再把不可变项目 ID 和名称、标识快照写入活动登录配置。
- 项目名称或标识变化不影响已保存的不可变 ID；下一次成功解析时刷新本地展示快照。
- 项目被删除或当前凭据失去访问权时返回稳定错误 `project_context_invalid`，不得回退到其他项目空间；用户必须重新执行 `project use` 或 `project unset`。
- `project=<value>`、`--project <value>` 和 `LUNA_PROJECT` 只覆盖单次命令或当前进程，不修改活动登录配置。
- `project current` 输出当前实例、项目 ID、展示名称和来源 `argument|environment|config|none`。
- 人类模式执行写操作或危险操作时，预览和确认区域必须明确展示最终解析出的项目空间。
- CI 使用显式 `project=<id>` 或进程级 `LUNA_PROJECT`，不依赖持久默认项目。
- CLI 不实现额外常驻 REPL 或后台会话；临时“会话默认项目空间”由 Shell 作用域内的 `LUNA_PROJECT` 表达。

默认项目只减少重复参数，不授予额外权限；服务端继续执行最终权限判断。

### 8.4 本地文件安全

- `~/.luna` 权限为 `0700`，`auth.json` 权限为 `0600`。
- Windows 不能依赖 POSIX mode bit；首次创建目录和文件时必须设置仅当前用户、`SYSTEM` 和 `Administrators` 可访问的 DACL，并在读取已有配置前检查是否向 `Everyone`、`Users` 或其他主体授予读权限。无法收紧权限时拒绝落盘长期凭据并给出修复命令。
- 写入使用同目录临时文件、`fsync` 和原子重命名，防止进程中断损坏配置。
- 并发写入必须使用文件锁。
- 拒绝跟随指向其他用户可写位置的符号链接。
- 日志、错误、Shell Completion 和遥测不得输出 Token。
- 调试输出中的 `Authorization`、Cookie、Token、Secret 和敏感 URL 参数统一脱敏。
- Token 只发送到活动登录配置的同源地址。跨源重定向不得携带 `Authorization`。
- 临时 `server=<url>` 先规范化为 origin。只有其与活动登录的 origin 完全一致时才可复用当前凭据；不同 origin 必须显式提供 `LUNA_TOKEN` 或 `token=@-`，CLI 不得尝试发送当前 Token。
- 默认遵循 `HTTP_PROXY`、`HTTPS_PROXY` 和 `NO_PROXY`；实例级 `network.proxy` 只覆盖该实例，`network.noProxy` 可追加无需代理的地址。
- 企业自签 CA 使用实例级 `tls.caFile` 或运行时支持的系统 CA 配置，不得用 `insecureSkipTlsVerify` 代替长期 CA 配置。

第一版按约定保存到 `auth.json`。后续可以增加系统 Keychain 作为可选 Secret Backend，但不能重新引入命名 context 或隐式多账号切换。

### 8.5 服务端能力协商

CLI 面向多个可能版本不同的实例，不能只根据 CLI 版本猜测服务端能力。后端需新增稳定的公共元数据接口，例如：

```text
GET /api/v1/meta
```

响应至少包含：

```json
{
  "apiVersion": "v1",
  "serverVersion": "0.1.0",
  "openapiDigest": "sha256:...",
  "features": {
    "oauthDeviceCode": true,
    "bearerStepUpMfa": true,
    "terminal": true,
    "gitOauthAuthorizationTransaction": true
  },
  "minimumCliVersion": "0.0.7"
}
```

CLI 在登录和首次调用实例时缓存非敏感能力快照，但每次遇到 `unsupported_feature`、契约摘要变化或缓存过期时重新读取。命令不受当前实例支持时，必须在发请求前返回稳定错误和服务端升级建议，不能发送一个已知会失败的请求。`help catalog` 仍展示当前 CLI 的完整能力，并可通过 `serverSupported` 标记当前实例是否支持。

当前实现会在每个 OpenAPI 业务命令第一次访问实例时读取该接口，同时校验
`apiVersion`、`minimumCliVersion` 和 `openapiDigest`；校验成功后按实例 origin
缓存到当前 CLI 进程。缺少元数据接口、未知 API 代际、CLI 版本过低或契约摘要不一致
都会在发送业务请求前 fail closed。通用 `api request` 作为人类诊断逃生口不执行该
预检，也不能用于 Agent 业务编排。

`/api/v1/meta` 必须允许未登录客户端读取，并配置独立限流；响应不得包含内部地址、密钥、Provider 配置或其他敏感部署信息。第一版 CLI 连接到缺少该接口、返回未知 `apiVersion` 或不满足 `minCliVersion` 的实例时必须 fail closed，返回 `server_too_old` 或 `unsupported_api_version`，不得通过试探业务接口猜测能力。

## 9. 登录与授权

### 9.1 默认 OAuth 浏览器登录

```bash
luna auth login server=https://devops.example.com
```

流程：

1. CLI 生成 PKCE verifier、challenge 和 state。
2. CLI 在 `127.0.0.1` 随机可用端口启动一次性 loopback callback。
3. 打开系统浏览器进入 Luna DevOps 授权页。
4. 用户登录、完成必要 MFA 并确认 Scope。
5. CLI 校验 state，使用 authorization code + verifier 换取 Token。
6. CLI 获取当前用户信息并保存为活动登录。
7. callback server 立即关闭。

后端要求：

- 新增内置的第一方公共 OAuth Client，例如 `luna-cli`。
- 该 Client 由平台迁移或启动初始化固定创建，不开放动态客户端注册，也不要求管理员为每个实例手工生成 Client Secret。
- 公共客户端使用 `token_endpoint_auth_method=none`，不能在二进制内嵌 client secret。
- 必须强制 PKCE S256。
- Token Endpoint 必须接受该内置公共客户端不带 Client Secret 的换码与刷新请求；现有只支持 `client_secret_basic/post` 的实现必须先扩展。
- OAuth Scope 校验必须与个人访问令牌的可创建 Scope 策略分离：PAT 继续禁止要求 Step-up 的敏感 Scope，第一方 `luna-cli` OAuth Client 可以在用户明确同意、角色允许且后续强制 Step-up 的前提下申请这些 Scope。
- 第一方 CLI Client 不自动获得 `*`；请求的每个 Scope 都必须进入授权确认页、Access Token、审计和服务端最终权限判断。
- 只为该内置公共客户端允许 `http://127.0.0.1:{random-port}/callback` 或 `http://[::1]:{random-port}/callback`；主机必须是 loopback IP 字面量，path 固定，不允许任意主机、域名、userinfo 或额外路径通配。
- Authorization Code 必须单次使用、短时有效并绑定 Client ID、redirect URI 和 PKCE challenge；Token Endpoint 使用标准 form 编码处理授权码、刷新和 Device Code grant。

### 9.2 Device Code 登录

```bash
luna auth login deviceCode=true
```

按 RFC 8628 实现：

1. CLI 请求 `device_code`、`user_code`、`verification_uri`、`verification_uri_complete`、`expires_in` 和 `interval`。
2. CLI 显示用户码和验证地址，可用时打开浏览器。
3. 用户在另一设备登录并批准 Scope。
4. CLI 按服务端 interval 轮询 Token Endpoint。
5. 对 `authorization_pending` 继续等待，对 `slow_down` 增加轮询间隔。
6. 到期、拒绝或终止后清理临时状态。

新增协议端点：

```text
POST /api/v1/oauth/device/authorization
GET  /api/v1/oauth/device/verification?userCode=<user_code>
POST /api/v1/oauth/device/verification
POST /api/v1/oauth/token
```

其中：

- `device/authorization` 由未登录的 CLI 调用，只创建短时设备授权请求；
- `device/verification` 的 GET/POST 由已登录浏览器 Session 调用，GET 展示待确认的 Client、Scope 和设备信息，POST 带 CSRF 防护并明确批准或拒绝；
- Token Endpoint 仅由 CLI 轮询，不把浏览器 Session 与设备码混为同一认证上下文；
- 用户码查询必须恒定时间比较或基于哈希索引，响应不得泄露 `device_code`；
- 已批准请求只能兑换一次，拒绝、过期和成功兑换后均进入不可逆终态。

Token 请求使用：

```text
grant_type=urn:ietf:params:oauth:grant-type:device_code
device_code=<device_code>
client_id=luna-cli
```

OAuth Metadata 必须新增 `device_authorization_endpoint` 和对应 grant type。

上述 Device Code 链路已实现：后端提供用户确认页、设备码与用户码哈希存储、批准/拒绝、过期清理、轮询限流和一次性兑换；CLI 默认展示用户码、打开浏览器并遵循服务端轮询契约。

### 9.3 Access Token 登录

```bash
printf '%s' "$TOKEN" | luna auth login token=@-
LUNA_TOKEN="$TOKEN" luna project list output=json interactive=false
```

安全要求：

- 不允许 `token=<value>`，避免 Token 进入 Shell history 和进程参数。
- `token=@-` 只从 stdin 读取。
- `LUNA_TOKEN` 只覆盖当前进程，默认不写入 `auth.json`。
- 如需保存环境变量中的 Token，必须显式设置 `store=true` 并在 TTY 中确认。
- 登录时调用当前用户和 Scope 查询验证 Token，不接受无法验证的凭据。

### 9.4 状态与登出

```bash
luna auth status
luna auth refresh
luna auth logout
luna auth logout localOnly=true
```

`auth status` 只展示当前活动登录，不维护或枚举历史实例、账号和凭据。OAuth 登出时先调用 revoke，服务端不可达时询问是否仅删除本地凭据。个人访问令牌默认只从本地移除；吊销远端令牌必须使用明确的 `access-token revoke`。

### 9.5 Git Provider OAuth 授权

当前 Git Provider OAuth start 接口直接返回浏览器 302，回调完成后只把账号写入平台并重定向 Web 页面。CLI 无法仅凭这两个入口判断授权是否完成、失败，或最终创建了哪个 Git 账号，因此不能把现有 start 接口直接包装为一个“成功”的命令。

第一版使用可轮询的授权事务：

```text
POST /api/v1/git/providers/{providerId}/oauth/authorizations
GET  /api/v1/git/oauth/authorizations/{authorizationId}
```

创建响应至少包含：

```json
{
  "authorizationId": "gauth_example",
  "authorizationUrl": "https://devops.example.com/api/v1/git/providers/provider/oauth/start?state=...",
  "expiresAt": "2026-07-24T12:00:00Z",
  "pollInterval": 2
}
```

状态响应只允许 `pending`、`succeeded`、`failed`、`denied`、`expired`，成功时返回新建或更新后的 `accountId`，失败时返回稳定错误码。OAuth callback 校验 state 后更新同一授权事务，而不是要求 CLI 轮询整个账号列表猜测结果。

对应命令为：

```bash
luna git authorize provider=<provider-id>
```

CLI 打开 `authorizationUrl` 并按 `pollInterval` 等待终态；非交互模式返回授权地址和事务 ID，由调用方稍后再次查询。该事务短时有效、单次完成、绑定发起用户和 Provider，且不得把第三方 Access Token 返回给 CLI。

## 10. Step-up MFA

### 10.1 目标行为

CLI 调用受保护接口收到以下稳定错误时：

```json
{
  "code": "mfa_required",
  "purpose": "kubeconfig_update"
}
```

在交互终端中：

1. CLI 显示操作和 MFA purpose。
2. 使用分格 OTP 输入或恢复码输入。
3. 调用 MFA verify。
4. 服务端创建绑定当前 OAuth 授权会话和 purpose 的短时 assertion。
5. CLI 使用原请求的幂等键重试一次。

在非交互终端中：

- 不读取 `/dev/tty`，不悬挂等待。
- 输出结构化 `mfa_required` 错误并返回固定退出码。
- 用户可先执行显式验证，再重试原命令：

```bash
luna auth mfa-verify purpose=kubeconfig_update
```

### 10.2 后端改造要求

Step-up assertion 已统一绑定 Web Session 或内置 CLI OAuth Grant，终端和数据导出也使用同一认证上下文。以下要求作为已实现行为和后续回归边界保留：

- OAuth access token 可追溯到 OAuth grant 和授权会话；
- `VerifyMFA` 同时支持浏览器 Session 与 OAuth Bearer 授权上下文；
- MFA assertion 绑定 `user + authentication context + purpose`，其中 authentication context 是 Web Session ID 或 OAuth Grant/Token Family ID；
- `requireStepUp` 根据当前认证方式读取对应 assertion，不再因为请求使用 Bearer Token 而无条件拒绝；
- assertion 只保存在服务端，CLI 不保存 TOTP Secret；
- assertion 有短有效期和无操作过期策略；
- 验证成功、失败、恢复码使用和吊销写 AuditLog；
- 原操作按幂等键最多自动重试一次。
- 把终端授权和数据导出票据依赖的 `SessionID` 抽象成 `authentication context`；Web 继续绑定 Session ID，OAuth CLI 绑定 OAuth Grant 或 Token Family ID；
- WebSocket 握手允许携带 OAuth Bearer，并在升级前完成 Scope、项目角色、Step-up 和目标资源校验；
- 终端存活监控同时检查用户、OAuth Grant/Token Family、Scope、项目成员关系和 Step-up assertion；授权被吊销、Scope 被收回或用户失效时主动关闭连接；
- 数据导出授权与一次性下载票据绑定同一个 OAuth authentication context，刷新 Access Token 后仍可在 Token Family 未吊销时消费，且不能被其他 grant、PAT 或浏览器 Session 使用；
- `requireInteractiveSession` 不再作为“必须使用浏览器 Cookie”的通用能力判断；需要用户在场的接口改用明确的 `requireInteractiveAuthContext`，并分别声明允许的 Web Session、OAuth 与 PAT 模式。
- Scope catalog 和 OAuth consent API 必须返回同一套稳定 Scope 定义；要求 Step-up 的 Scope 需带机器可读标记，CLI Help 可以提前说明该操作只支持 OAuth 登录并可能要求 OTP。

个人访问令牌第一版不能执行要求 Step-up MFA 的操作。此时 CLI 应提示改用 OAuth 登录，不能通过放宽服务端策略绕过 MFA。

CLI 不依赖服务端尚不存在的 `challengeId`。如果未来增加一次性 challenge，必须先扩展稳定错误 Schema，并保持 `purpose` 仍可被旧 CLI 理解。

## 11. 输出与 AI 使用契约

### 11.1 stdout 与 stderr

- stdout：只输出成功结果；失败时保持为空。
- stderr：进度、交互提示、警告和诊断；命令失败时输出错误结果。
- `table` 是面向人的可读模式：列表使用本地化表格，单个对象使用本地化字段视图，长文本保持原始块。
- `json` 是稳定机器模式：成功时 stdout 只包含一个合法 JSON document；失败时 stderr 只包含一个合法 JSON error document，不添加本地化前缀、进度或装饰文本。
- `output=json interactive=false` 时，非结果诊断默认静默；仅在 `debug=true` 时将脱敏诊断写入 stderr，并且调用方不能把 debug 模式当作稳定机器协议。
- TTY 默认使用 `table`。
- 非 TTY 默认使用 JSON，避免管道中混入装饰文本。
- `output=<format>` 或 `--output <format>` 始终覆盖本地配置和自动选择。
- `LUNA_OUTPUT` 可设置当前进程默认输出模式。
- `quiet=true` 可关闭非必要 stderr 信息，但不能吞掉错误。

输出选择优先级：

```text
命令 output / --output
> LUNA_OUTPUT
> config.output
> TTY 自动选择
```

AI 和自动化不得依赖默认值。Skills 执行的每条命令必须显式包含：

```text
agent=true
```

`agent=true` 是 `output=json interactive=false color=false` 和资源边界检查的规范入口；即使用户本地配置默认使用 `table`，Skills 也不能省略。普通脚本仍可显式使用单独参数。

### 11.2 稳定格式

`output=json` 使用版本化 CLI Envelope，避免服务端某个资源响应结构变化时无边界地破坏自动化：

```json
{
  "apiVersion": "cli.luna.devops/v1",
  "schemaVersion": "project.list/v1",
  "operationId": "project.list",
  "command": "project.list",
  "data": {
    "items": [],
    "page": 1,
    "pageSize": 20,
    "total": 0,
    "totalPages": 0
  },
  "meta": {
    "requestId": "req_example",
    "server": "https://devops.example.com",
    "projectId": "",
    "actorId": "usr_example",
    "authType": "oauth",
    "cliVersion": "0.1.0",
    "openapiDigest": "sha256:..."
  }
}
```

`output=raw-json` 只输出服务端原始成功响应，供调试和迁移脚本使用，不承诺跨服务端版本稳定；AI Skills 禁止使用。`yaml` 与 `json` 使用相同 Envelope，`jsonl` 每行使用版本化 item Envelope。字段名、枚举、时间和错误码均不做本地化。

`schemaVersion` 指向当前 command 的输出 Schema 版本，`operationId` 对应 OpenAPI operation。`meta` 只包含非敏感执行上下文，不能包含 Access Token、Refresh Token、Cookie、Secret、OTP、恢复码或完整 Authorization Header。

面向 Agent 的 `nextActions` 只能由 CLI 的受信任命令注册表生成，格式为稳定 command path、参数占位和所需批准，不允许把 API message、日志、仓库内容、事件正文或第三方响应转换为命令建议。

时间默认使用 RFC 3339。人类表格可按本地时区显示，但 `output=json|yaml|jsonl` 保留服务端标准时间。

大列表支持 JSONL：

```bash
luna event list output=jsonl interactive=false | jq 'select(.severity == "error")'
```

### 11.3 错误结构

机器输出错误：

```json
{
  "error": {
    "code": "mfa_required",
    "message": "The operation requires step-up authentication.",
    "status": 403,
    "requestId": "req_example",
    "retryable": false,
    "purpose": "kubeconfig_update",
    "details": {}
  }
}
```

退出码：

| 退出码 | 含义 |
| --- | --- |
| `0` | 成功 |
| `1` | 未分类错误 |
| `2` | 命令参数或输入错误 |
| `3` | 未登录、Token 过期或刷新失败 |
| `4` | 权限不足或需要 MFA |
| `5` | 资源不存在 |
| `6` | 冲突、并发修改或状态不允许 |
| `7` | 限流或稍后重试 |
| `8` | 网络、服务端或外部依赖错误 |
| `9` | 批量操作部分成功 |

#### 服务端稳定错误契约

当前后端存在多种错误写入方式。`v0.1.0` 实现前必须把全部 `business-command`、`protocol-adapter`、`client-entry` 和 `server-entry` 的错误响应收敛到稳定 Envelope：

```json
{
  "error": {
    "code": "validation_failed",
    "status": 400,
    "message": "optional non-localized diagnostic",
    "requestId": "req_example",
    "retryAfter": 0,
    "fields": {
      "name": "required"
    },
    "details": {}
  }
}
```

约束如下：

- `code`、`status`、`fields`、`details` 和协议专用字段是机器契约；CLI 不通过 `message` 文本匹配业务分支。
- `message` 仅用于保留后端或第三方诊断，不要求本地化，不得包含堆栈、Secret、Token、SQL、kubeconfig 或内部网络敏感信息。
- `requestId` 在 API 入口统一生成或透传，并进入日志、审计和 CLI 错误输出。
- 429 和需要延迟重试的错误同时使用标准 `Retry-After` Header；Envelope 中的 `retryAfter` 只作为方便消费的等价值。
- 字段校验错误使用稳定字段路径和稳定 reason code，不把完整本地化句子塞进 `fields`。
- OAuth、Device Code、MFA、SSE 握手、WebSocket 升级和二进制下载失败也必须映射到该错误目录；协议规范要求的 OAuth `error` 字段可保留，但必须有确定的 CLI 映射。
- OpenAPI 为每个 operation 声明可能的稳定错误响应和共享错误 Schema；运行时返回未登记错误码时契约测试失败。
- 未分类内部异常统一映射为 `internal_error`，保留 `requestId` 并隐藏生产细节；CLI 可以展示请求 ID，但不能把原始响应体当作稳定错误结构。

HTTP API 可以采用 RFC 9457 `application/problem+json` 表达 `type`、`title`、`status`、`detail` 和 `instance`，但 Luna 的稳定 `code`、`requestId`、字段错误和协议扩展仍必须保留为扩展成员。CLI Envelope 负责把服务端 Problem Details 归一化为上面的机器错误结构，不依赖本地化 `title` 或 `detail` 做控制流。

唯一错误码目录使用语言无关文件 `openapi/errors.yaml` 维护。Go 后端、OpenAPI、Web、`packages/api-contract` 和 CLI 从该目录生成或校验映射。禁止把 TypeScript 包作为后端错误定义的事实来源，也禁止各处维护名称相似但语义不同的错误码。

### 11.4 交互原则

- 非 TTY 默认完全无交互。
- 危险操作在 TTY 中二次确认；自动化必须显式使用 `yes=true`，高风险操作还必须提供有效的服务端 `planId`。
- Secret 通过 stdin、环境变量或安全提示读取，不作为普通参数。
- 支持的写操作按 §6.4 提供 `dryRun=client|server`；客户端预览不宣称通过服务端权限、准入或冲突校验。
- 创建操作支持 `idempotencyKey=<key>`，自动重试不得重复创建资源。
- 流式日志收到 SIGINT 时正常关闭连接，并返回可区分的退出状态。
- 分页列表统一支持服务端原生的 `page`、`pageSize`、`sortBy` 和 `sortOrder`；`all=true` 由 CLI 连续读取全部分页，并受 `maxItems` 与 `timeout` 保护。
- 批量操作必须返回每项成功或失败结果；部分成功使用退出码 `9`，不能因为 HTTP 请求整体成功而吞掉单项错误。
- GET/HEAD 可对网络中断、429 和部分 5xx 执行有上限的指数退避，并遵循 `Retry-After`。写操作只有在 operation 声明幂等且携带幂等键时才允许自动重试。
- `yes=true` 只跳过 CLI 确认，不跳过服务端权限、MFA、状态校验或审计。
- Agent 模式的分页、重试、轮询和流式读取必须受 `maxItems`、`maxAttempts`、`timeout`、`maxEvents` 和 `maxBytes` 限制；达到边界后返回可恢复的结构化错误，不静默截断。
- 中高风险更新必须携带服务端认可的资源版本；发生冲突后不得自动追加 `force=true` 或替换为最新版本。

## 12. 国际化

第一版内置：

- `zh-CN`
- `en-US`

语言选择优先级：

```text
--lang
> LUNA_LANG
> config.language
> LC_ALL
> LC_MESSAGES
> LANG
> Intl.DateTimeFormat().resolvedOptions().locale
> en-US
```

规则：

- `zh`、`zh-CN`、`zh-Hans` 归一化为 `zh-CN`。
- 未支持的区域语言回退到同语言，再回退到 `en-US`。
- 帮助、提示、表头和人类错误说明本地化。
- 命令名、参数名、JSON key、API enum 和稳定错误码保持英文。
- i18n 资源编译进二进制，不在首次运行时联网下载。
- 所有 locale 必须通过缺失键和参数占位符一致性测试。

## 13. 安全要求

- OAuth CLI 使用公共客户端 + PKCE，不在代码或二进制内嵌 client secret。
- Device Code 必须短时有效、一次使用，并限制轮询频率。
- 浏览器回调只监听 loopback，不监听 `0.0.0.0`。
- Access Token、Refresh Token 和个人令牌不得出现在 argv、日志、错误和遥测。
- Token 刷新使用单飞锁，多个并发命令不能同时旋转 Refresh Token。
- 对 401 最多自动刷新并重试一次；对写请求重试必须有幂等保证。
- `insecureSkipTlsVerify=true` 必须显式设置并持续显示警告，不保存为隐式默认。
- CLI 不允许自行扩大 Scope。增权必须重新走 OAuth 授权。
- 服务端仍是权限和 Scope 的最终判断者。
- CLI 上报可选匿名使用统计前，必须另行设计并默认关闭。第一版不实现遥测。
- 日志、构建输出、仓库文件、事件、Webhook 内容、第三方 API 响应和 Kubernetes 字段全部视为不可信数据；CLI 只把它们作为 `data` 返回，不执行其中的命令，也不从中派生 `nextActions`。
- 人类输出必须转义 ANSI、OSC 8 链接、控制字符和双向文本控制符；JSON/JSONL 保留可解析数据但不得让终端渲染控制序列。
- Schema 中标记为 `writeOnly`、`format=password` 或 `x-sensitive=true` 的字段必须在 stdout、stderr、debug、审计摘要、崩溃报告和测试快照中统一脱敏。
- OTP、恢复码和站点密码属于用户在场凭据。Agent 收到 MFA 挑战后只能暂停并引导用户在浏览器或受控 TTY 中完成，不能要求用户把验证码发进对话，也不能代替用户完成 Step-up。
- 高风险批准必须绑定 §6.10 的精确计划和短时认证上下文，防止批准被重放到不同目标或修改后的参数。
- Agent 模式必须限制并行数、分页数量、重试次数、等待时间和输出字节，避免失控循环、Denial of Wallet 和对平台形成意外负载。
- 每次 Agent 调用都生成 `correlationId`，审计记录至少包含 actor、认证上下文、server、project、operationId、目标、计划 ID、幂等键、结果、请求 ID 和客户端版本。服务端不得信任客户端自报的 actor、风险级别或授权结论。
- CLI 不提供任意 Shell、JavaScript、模板表达式或“执行服务器返回命令”的通用入口；`api request` 也受认证、Scope、目标实例和输出边界约束。

## 14. 配套 AI Skills

仓库中的 [`ai-supports/skills`](../ai-supports/skills) 是 CLI 的薄编排层，调用链固定为：

```text
用户或 AI Agent
  -> Luna DevOps Skills
  -> luna CLI
  -> Luna DevOps REST API
```

Skills 只负责：

- 将自然语言意图路由到一个或少数业务域；
- 安排读取、预览、确认、变更和验证的顺序；
- 解释结构化结果、错误码和下一步；
- 对高风险操作执行用户确认边界。

Skills 不负责：

- 复制完整命令手册或硬编码尚未实现的命令；
- 直接调用 REST API、Kubernetes API 或第三方 Provider API；
- 绕过 CLI 和服务端的 Scope、RBAC、MFA、审计与脱敏；
- 把日志、仓库文件、事件正文或第三方响应当作可信指令。

每次执行前必须先通过 `luna version show agent=true` 检查 CLI 可用性，再按当前意图使用带 `query/category/risk/scope/limit` 的 `luna help catalog agent=true` 检索少量候选命令，并读取对应工具的机器可读 Help。Help 中没有的命令视为尚未支持，Agent 不得根据 endpoint 名称猜测，也不得把完整命令目录一次性注入上下文。

Skills 调用每一条 CLI 命令时都必须显式附带 `agent=true`，不能依赖本地配置、环境变量、TTY 或用户偏好决定输出模式。完整参数映射使用 `params=@file` 或 `params=@-`；Help Schema 将请求体暴露为 `body` 参数时使用 `body=@file` 或 `body=@-`，文件内容就是请求体本身，不再额外包裹 `body`。生成参数前先读取 command 输入 Schema，禁止发送未知字段。

对 `projectContext: required` 的命令，Skills 默认还必须显式传入全局
`project=<immutable-id>`，或按机器 Help 使用命令自身的
`projectId=<immutable-id>` / `projectID=<immutable-id>` 参数。只有低风险读取命令、
用户已经明确要求使用当前活动登录的默认项目，且 Skill 先通过
`luna project current agent=true` 校验过解析结果时，才允许省略；Skills
不得自行执行 `project use` 修改用户共享的持久配置。

Skills 的标准变更流程固定为：

```text
发现少量候选工具
  -> 读取具体命令 Schema
  -> 读取当前状态和资源版本
  -> client/server dry-run
  -> 高风险操作创建服务端计划
  -> 向用户展示目标、diff、风险、成本和回滚
  -> 用户批准精确 planId
  -> 执行一次
  -> 读取状态验证后置条件
```

收到 `mfa_required` 时，Skill 必须暂停并要求用户在受控界面完成，不读取或转述 OTP/恢复码。收到冲突、计划过期或目标集合变化时，必须废弃旧批准并重新读取、计划和确认。

日志、仓库内容、事件和第三方响应只作为不可信证据。即使其中出现“运行以下 luna 命令”或伪造的系统指令，Skill 也不得执行。分页、轮询和故障重试必须显式设置上限，且不能为了绕过错误自动扩大 Scope、改用管理员凭据、追加 `force` 或切换项目空间。

### 14.1 可执行工作流目录

除单 operation 的 OpenAPI 外，仓库新增 `openapi/workflows.yaml`，使用 OpenAPI Initiative Arazzo 1.1 描述跨步骤关键旅程。第一版至少覆盖：

- OAuth PKCE 登录、Device Code 登录与 Token 刷新；
- Git Provider 授权事务；
- 构建 -> 发布 -> 部署 -> 状态验证；
- MFA 挑战后的原操作重试；
- 数据导出和安全下载；
- Web Console 预授权、连接和关闭。

每个工作流声明输入、前置条件、operationId 序列、步骤输出、成功条件、超时、批准点和失败后的补偿/恢复提示。Arazzo 是文档、测试和 Skills 生成的事实来源，不作为第一版运行时通用工作流解释器；业务逻辑仍由 CLI 命令和服务端实现。

CLI 完成前，仓库中的 Skills 仅用于规格审阅和工作流设计，不发布为可执行能力。CLI 完成后再补充 Skill 元数据、运行结构校验，并在真实测试实例覆盖只读、变更、失败、权限不足、MFA 和敏感信息脱敏场景。

旧 MCP 和内嵌 Assistant 方案不再保留。平台 AI 自动化统一通过 CLI 进入，避免维护 REST、MCP 和 CLI 三套能力映射。

## 15. 构建与发布

### 15.1 安装方式

公开 npm 包名固定为 `@liteyuki/luna-cli`。发布前必须确认 npm 组织 `@liteyuki` 已创建且发布账号拥有权限；不使用容易被抢注的无 Scope 包名。

#### npm 与 pnpm 全局安装

```bash
npm install --global @liteyuki/luna-cli
pnpm add --global @liteyuki/luna-cli
```

安装后统一提供 `luna` 命令：

```bash
luna --version
luna help catalog output=json interactive=false
```

一次性运行用于试用或固定版本的 CI：

```bash
npx --yes @liteyuki/luna-cli@latest --version
pnpm dlx @liteyuki/luna-cli@latest --version
```

预发布版本必须显式指定 dist-tag 或版本，不能污染稳定安装：

```bash
npm install --global @liteyuki/luna-cli@next
pnpm add --global @liteyuki/luna-cli@beta
```

npm 全局安装通过 `package.json.bin` 将 `luna` 链接到全局可执行目录。Unix 入口必须有 `#!/usr/bin/env node`，Windows 由 npm/pnpm 生成命令包装器。遇到全局目录权限问题时，文档引导使用 Node.js 版本管理器或用户级 pnpm home，不推荐对安装命令使用 `sudo`。

#### 下载独立二进制

GitHub Release 是无 Node.js/Bun 环境的首选安装方式。用户必须同时下载对应平台制品和 `SHA256SUMS`，校验后再放入用户可写的 `PATH`：

```bash
version="cli-vX.Y.Z"
asset="luna-darwin-arm64"
curl -fL -o luna "https://github.com/LiteyukiStudio/luna-devops/releases/download/${version}/${asset}"
curl -fL -o SHA256SUMS "https://github.com/LiteyukiStudio/luna-devops/releases/download/${version}/SHA256SUMS"
grep " ${asset}$" SHA256SUMS | sed "s# ${asset}$# luna#" | shasum -a 256 -c -
chmod +x luna
mkdir -p "${HOME}/.local/bin"
mv luna "${HOME}/.local/bin/luna"
```

用户负责将 `${HOME}/.local/bin` 加入 `PATH`。Windows 与 Alpine/musl 第一阶段不提供独立二进制，统一使用 npm 或 pnpm 安装，并由 Node.js `22.14.0` 或更高版本运行。第一版不提供自动修改 shell profile 或系统级 `PATH` 的安装脚本，避免静默改写用户环境。

卸载时：

```bash
npm uninstall --global @liteyuki/luna-cli
pnpm remove --global @liteyuki/luna-cli
rm "${HOME}/.local/bin/luna"
```

卸载程序不删除 `~/.luna/auth.json`。凭据清理必须由用户显式执行 `luna logout` 或手动删除配置目录，避免包管理器卸载误删用户数据。

### 15.2 npm 包契约

`cli/package.json` 至少包含：

```json
{
  "name": "@liteyuki/luna-cli",
  "version": "0.0.0",
  "description": "Command-line client for Luna DevOps",
  "type": "module",
  "bin": {
    "luna": "bin/luna.js"
  },
  "files": [
    "bin",
    "dist",
    "README.md",
    "LICENSE"
  ],
  "engines": {
    "node": ">=22.14.0"
  },
  "repository": {
    "type": "git",
    "url": "https://github.com/LiteyukiStudio/luna-devops.git",
    "directory": "cli"
  },
  "publishConfig": {
    "access": "public",
    "registry": "https://registry.npmjs.org/"
  }
}
```

规则：

- `bin/luna.js` 只做稳定入口和运行时版本检查，实际实现位于 `dist/`，不得复制第二套命令逻辑。
- `files` 使用白名单；发布包不得包含源码映射中的本机路径、测试夹具、凭据、`.env`、缓存、仓库锁文件或 CI 文件。
- `repository.url` 必须与 npm Trusted Publisher 配置的 GitHub 仓库完全一致，`repository.directory` 指向 monorepo 中的 `cli`。
- 仓库中的 `version` 固定为 `0.0.0-development`，只表示源码开发态，不参与正式版本决策；源码清单使用 `private: true` 阻止绕过发布工作流直接发布。
- `cli-v*` Git tag 是发布版本的唯一来源。工作流去掉 `cli-v` 后校验 SemVer，在临时 npm 打包目录移除 `private` 标记并写入发布版本，同时在构建时把同一版本注入 npm JavaScript 制品和 Bun 二进制；不得修改或提交源工作树中的版本。
- npm tarball 与独立二进制必须注入相同的版本、Git SHA、构建时间和 OpenAPI 契约版本。
- npm 包不包含 `preinstall`、`install` 或 `postinstall` 脚本，不在安装阶段执行网络请求或本机修改。
- `npm pack --dry-run --json` 的文件清单要进入 CI 断言；真实发布使用经过测试的同一 `.tgz`，不在发布 Job 重新构建。

### 15.3 目标制品

| 平台 | 架构 | 产物 |
| --- | --- | --- |
| macOS | arm64 | `luna-darwin-arm64` |
| macOS | x64 | `luna-darwin-x64` |
| Linux glibc | x64 | `luna-linux-x64` |
| Linux glibc | arm64 | `luna-linux-arm64` |

所有 x64 二进制默认使用 Bun 的 `baseline` target，避免普通 x64 构建对 AVX2 的要求导致旧服务器启动失败；性能优先的现代 x64 制品只有在后续确有需求时再单独发布。macOS 二进制最低系统版本遵循所固定 Bun 版本的官方支持范围，第一版按 macOS 13 及以上验证并在 Release 中明确标注。

Windows 与 Alpine/musl 作为明确的 Node.js 降级目标，不进入第一阶段 Bun 二进制矩阵。npm/pnpm 是全平台主渠道；只有目标 runner 能真实执行且维护成本可控的 Bun 制品才进入 GitHub Release。

每个 Release 提供：

- npm tarball `liteyukistudio-luna-cli-<version>.tgz`，用于精确复现和安装 smoke test；
- SHA-256 checksum；
- 软件物料清单 SBOM；
- 可验证签名；
- 版本、Git SHA、构建时间和 OpenAPI 契约版本；
- 中英文变更记录。

CLI 使用独立 SemVer，不要求与平台项目版本同步。CLI 与服务端通过 API capability endpoint 协商能力，不只比较版本字符串。

“可验证签名”必须落实为具体平台流程：macOS 使用 Developer ID 签名并 notarize，Linux 和通用 Release 清单使用 GitHub OIDC provenance/签名。若第一版暂时拿不到 macOS 代码签名凭据，该平台独立二进制不得进入稳定下载矩阵，只能标记为预发布；npm 安装不受此限制。

### 15.4 版本与发布通道

项目与 CLI 使用独立的 tag 命名空间：

- `v<major>.<minor>.<patch>[-prerelease]` 继续触发 Luna DevOps 平台项目发版；
- `cli-v<major>.<minor>.<patch>[-prerelease]` 只触发 Luna CLI 发版。

CLI 发布映射如下：

| Git tag | npm dist-tag | GitHub Release |
| --- | --- | --- |
| `cli-v1.2.3` | `latest` | 正式版 |
| `cli-v1.2.3-rc.1` | `next` | Pre-release |
| `cli-v1.2.3-beta.1` | `beta` | Pre-release |

禁止将预发布版本发布到 `latest`。npm 已发布版本不可覆盖；重跑工作流时如果版本已存在，必须比对远端包的 `dist.integrity` 与本次 tarball，完全一致时跳过 npm 发布并继续补齐 GitHub Release，否则立即失败并要求发布新版本。

项目发布工作流必须忽略 `cli-v*`，CLI 发布工作流必须忽略普通 `v*`。手动重跑也只能选择已经存在且符合对应命名空间的 tag。

### 15.5 GitHub Actions 拆分

实现时新增两个独立工作流，不把 CLI 发布塞入现有容器发布工作流：

| 工作流 | 触发 | 职责 |
| --- | --- | --- |
| `.github/workflows/cli-ci.yml` | CLI 相关 PR、`main` push、手动 | 类型、Lint、测试、契约、npm tarball 和 Bun 编译 smoke |
| `.github/workflows/cli-release.yml` | `cli-v*` tag、受限手动重跑 | 版本校验、完整构建、签名、npm OIDC 发布、GitHub Release |

`cli-release.yml` 的文件名属于 npm Trusted Publisher 身份，改名会导致发布认证失败。实际工作流中的第三方 Action 必须固定到完整 commit SHA，下面蓝图为便于阅读使用主版本号。

### 15.6 `cli-ci.yml` 蓝图

```yaml
name: CLI CI

on:
  pull_request:
    paths:
      - "cli/**"
      - "packages/api-*/**"
      - "openapi/**"
      - "package.json"
      - "pnpm-workspace.yaml"
      - "pnpm-lock.yaml"
      - ".github/workflows/cli-ci.yml"
  push:
    branches: [main]
    paths:
      - "cli/**"
      - "packages/api-*/**"
      - "openapi/**"
      - "package.json"
      - "pnpm-workspace.yaml"
      - "pnpm-lock.yaml"
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: cli-ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: pnpm/action-setup@v4
        with:
          run_install: false
      - uses: actions/setup-node@v6
        with:
          node-version: "24"
          cache: pnpm
      - uses: oven-sh/setup-bun@v2
        with:
          bun-version-file: ".bun-version"
      - run: pnpm install --frozen-lockfile
      - run: pnpm --filter @liteyuki/luna-cli typecheck
      - run: pnpm --filter @liteyuki/luna-cli lint
      - run: pnpm --filter @liteyuki/luna-cli test
      - run: pnpm cli:openapi-check
      - run: pnpm cli:command-coverage
      - run: pnpm --filter @liteyuki/luna-cli build:npm
      - run: npm pack --workspace cli --dry-run --json
      - run: pnpm --filter @liteyuki/luna-cli pack:verify
      - run: pnpm --filter @liteyuki/luna-cli smoke:npm
      - run: pnpm --filter @liteyuki/luna-cli build:binary:native
      - run: pnpm --filter @liteyuki/luna-cli smoke:binary
```

`smoke:npm` 必须从实际生成的 `.tgz` 分别执行：

1. 在空临时目录通过 `npm install --global <tarball> --prefix <temp>` 安装并运行；
2. 在独立空目录通过 `pnpm add --global <tarball>` 安装并运行；
3. 验证 `--version`、JSON Help、非 TTY、语言回退和临时 `LUNA_HOME` 配置读写；
4. 测试完成后销毁临时目录，不读写开发机真实的 `~/.luna`。

### 15.7 `cli-release.yml` 蓝图

发布工作流只接受仓库中已经存在且符合 `cli-v<semver>` 的 tag。`workflow_dispatch` 仅用于重跑指定 CLI tag，不允许输入任意版本并在工作流内创建 tag。普通 `v*` 继续由项目发布工作流处理。正式实现应把第三方 Action 主版本替换为审核后的完整 commit SHA。

下面 YAML 只展示构建、测试、npm 发布和 Release 编排，不包含与具体证书供应商绑定的 Apple notarization 命令，不能原样作为 macOS 稳定发布工作流投入使用。实现时必须在 `binary` 与 `github-release` 之间增加独立签名阶段：

1. macOS 产物在 macOS runner 使用 Developer ID 签名，提交 notarization 并完成 stapling，再做签名验证和 smoke test；
2. Windows 与 Alpine/musl 不生成独立二进制，统一验证 npm/pnpm 安装后的 Node.js 制品；
3. Linux 产物、checksum、SBOM 和最终 Release manifest 使用 GitHub OIDC provenance/签名；
4. `github-release` 只能下载签名阶段输出的最终 artifact，不能回退使用未签名的构建 artifact；
5. 没有 Apple 签名凭据时，workflow 必须让稳定版本失败；只有预发布版本可以按明确命名上传未验证的 macOS 制品。

```yaml
name: CLI Release

on:
  push:
    tags:
      - "cli-v*"
  workflow_dispatch:
    inputs:
      tag:
        description: Existing cli-v-prefixed Git tag to rebuild
        required: true
        type: string

permissions:
  contents: read

concurrency:
  group: cli-release-${{ inputs.tag || github.ref_name }}
  cancel-in-progress: false

jobs:
  metadata:
    runs-on: ubuntu-latest
    outputs:
      tag: ${{ steps.meta.outputs.tag }}
      version: ${{ steps.meta.outputs.version }}
      npm_tag: ${{ steps.meta.outputs.npm_tag }}
      prerelease: ${{ steps.meta.outputs.prerelease }}
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0
          ref: ${{ inputs.tag || github.ref }}
      - id: meta
        run: node cli/scripts/release-metadata.mjs "${{ inputs.tag || github.ref_name }}"
      - run: git tag --points-at HEAD --list "${{ steps.meta.outputs.tag }}" | grep -Fx "${{ steps.meta.outputs.tag }}"

  quality:
    needs: metadata
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${{ needs.metadata.outputs.tag }}
      - uses: pnpm/action-setup@v4
        with:
          run_install: false
      - uses: actions/setup-node@v6
        with:
          node-version: "24"
          cache: pnpm
      - uses: oven-sh/setup-bun@v2
        with:
          bun-version-file: ".bun-version"
      - run: pnpm install --frozen-lockfile
      - run: pnpm --filter @liteyuki/luna-cli typecheck
      - run: pnpm --filter @liteyuki/luna-cli lint
      - run: pnpm --filter @liteyuki/luna-cli test
      - run: pnpm cli:openapi-check
      - run: pnpm cli:command-coverage
      - run: pnpm --filter @liteyuki/luna-cli test:security
      - run: pnpm --filter @liteyuki/luna-cli test:auth-integration

  binary:
    needs: [metadata, quality]
    strategy:
      fail-fast: false
      matrix:
        include:
          - runner: macos-14
            target: bun-darwin-arm64
            asset: luna-darwin-arm64
          - runner: macos-14
            target: bun-darwin-x64
            asset: luna-darwin-x64
          - runner: ubuntu-24.04
            target: bun-linux-x64
            asset: luna-linux-x64
          - runner: ubuntu-24.04-arm
            target: bun-linux-arm64
            asset: luna-linux-arm64
    runs-on: ${{ matrix.runner }}
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${{ needs.metadata.outputs.tag }}
      - uses: pnpm/action-setup@v4
        with:
          run_install: false
      - uses: actions/setup-node@v6
        with:
          node-version: "24"
          cache: pnpm
      - uses: oven-sh/setup-bun@v2
        with:
          bun-version-file: ".bun-version"
      - run: pnpm install --frozen-lockfile
      - run: >
          pnpm --filter @liteyuki/luna-cli build:binary
          --target=${{ matrix.target }}
          --outfile=${{ matrix.asset }}
      - run: pnpm --filter @liteyuki/luna-cli smoke:asset --asset=${{ matrix.asset }}
      - uses: actions/upload-artifact@v6
        with:
          name: unsigned-${{ matrix.asset }}
          path: cli/release/${{ matrix.asset }}
          if-no-files-found: error
          retention-days: 7

  npm-package:
    needs: [metadata, quality]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${{ needs.metadata.outputs.tag }}
      - uses: pnpm/action-setup@v4
        with:
          run_install: false
      - uses: actions/setup-node@v6
        with:
          node-version: "24"
          cache: pnpm
      - run: pnpm install --frozen-lockfile
      - run: pnpm --filter @liteyuki/luna-cli build:npm
      - run: pnpm --filter @liteyuki/luna-cli pack:release
      - run: pnpm --filter @liteyuki/luna-cli smoke:npm
      - uses: actions/upload-artifact@v6
        with:
          name: luna-cli-npm
          path: cli/release/*.tgz
          if-no-files-found: error
          retention-days: 7

  publish-npm:
    needs: [metadata, binary, npm-package]
    runs-on: ubuntu-latest
    environment: npm
    permissions:
      contents: read
      id-token: write
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${{ needs.metadata.outputs.tag }}
      - uses: actions/setup-node@v6
        with:
          node-version: "24"
          registry-url: "https://registry.npmjs.org"
          package-manager-cache: false
      - run: npm install --global "npm@>=11.5.1 <12"
      - uses: actions/download-artifact@v7
        with:
          name: luna-cli-npm
          path: cli/release
      - run: >
          node cli/scripts/publish-npm.mjs
          --tarball=cli/release/*.tgz
          --tag=${{ needs.metadata.outputs.npm_tag }}

  github-release:
    # 实现时还必须依赖 sign-macos、sign-windows 和 sign-manifest；
    # 此处的示意代码不能直接发布 binary Job 产生的 unsigned-* artifact。
    needs: [metadata, publish-npm]
    runs-on: ubuntu-latest
    permissions:
      contents: write
      id-token: write
      attestations: write
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${{ needs.metadata.outputs.tag }}
      - uses: actions/download-artifact@v7
        with:
          path: cli/release
          pattern: signed-luna-*
          merge-multiple: true
      - run: node cli/scripts/release-manifest.mjs cli/release
      - uses: actions/attest-build-provenance@v3
        with:
          subject-path: "cli/release/*"
      - env:
          GH_TOKEN: ${{ github.token }}
          TAG: ${{ needs.metadata.outputs.tag }}
          PRERELEASE: ${{ needs.metadata.outputs.prerelease }}
        run: |
          args=("$TAG" "cli/release/"* "--verify-tag" "--generate-notes")
          if [ "$PRERELEASE" = "true" ]; then
            args+=("--prerelease")
          fi
          gh release create "${args[@]}"
```

蓝图中的 `release-metadata.mjs`、`publish-npm.mjs`、`release-manifest.mjs` 是发布逻辑的唯一实现位置，不能把版本判断、远端幂等检查和 checksum 规则散落到 YAML shell 片段中：

- `release-metadata.mjs` 先校验 tag 符合 `cli-v<semver>`，再去掉 `cli-v` 前缀并生成发布版本、预发布标识和 npm dist-tag；
- `pack-npm.mjs` 只在临时目录把 tag 版本写入发布清单，校验 `npm pack` 返回版本和文件白名单，源 `cli/package.json` 始终保留开发占位版本；
- `publish-npm.mjs` 查询 npm 远端版本，未发布时执行 `npm publish <tarball> --access public --tag <tag>`，已发布时校验完整性；
- `release-manifest.mjs` 生成 `SHA256SUMS`、SBOM、签名输入和中英文 Release 清单。

Windows 与 Alpine/musl 不进入该二进制 Job，发布与 smoke 均复用已经过 npm/pnpm 全局安装验证的 Node.js 制品。交叉编译只能证明生成成功，不能替代目标运行时验证；原生 runner 不可用的平台不应被加入已支持的二进制矩阵。

### 15.8 npm Trusted Publishing 配置

npm 正式发布采用 Trusted Publishing，不在 GitHub Secret 中保存长期 `NPM_TOKEN`：

1. 确认 npm 组织 `@liteyuki` 已存在，发布维护者拥有创建 public package 的权限并已启用 2FA。
2. 在干净环境中使用仓库发布脚本生成并验证 tarball，再由维护者使用 2FA 手动执行 `npm publish <tarball> --access public --tag next`。这次发布会创建 `@liteyuki/luna-cli`，不需要提前在 npm 单独创建包，也不创建自动化 Token。
3. 首次发布成功后，在 npm 包设置中添加 GitHub Actions Trusted Publisher：
   - Organization or user：`LiteyukiStudio`
   - Repository：`devops`
   - Workflow filename：`cli-release.yml`
   - Environment：`npm`
   - Allowed actions：`npm publish`
4. GitHub `npm` Environment 启用保护规则，只允许受保护 tag，经指定维护者审批后发布。
5. 创建一个指向目标提交、且版本尚未发布的 `cli-v*` tag 验证 OIDC 发布。无需修改 `cli/package.json`；不能复用首次手动发布的 tag 版本做认证验收，因为幂等发布逻辑发现远端内容一致时会跳过 `npm publish`。
6. `publish-npm` Job 必须在 GitHub-hosted runner 上执行，权限只增加 `id-token: write`，不得设置 `NPM_TOKEN` 或 `NODE_AUTH_TOKEN`。
7. 发布环境使用 Node.js `>=22.14.0` 和 npm CLI `>=11.5.1`。完成 Trusted Publishing 验证后，在 npm 包设置中要求发布使用 2FA，并禁止传统写入 Token。
8. `package.json.repository.url` 与 GitHub 仓库精确匹配。公开 GitHub 仓库向公开 npm 包发布时由 npm 自动生成 provenance，不额外传入 `--provenance`。

Trusted Publisher 只允许绑定一个工作流。不要把 `npm publish` 抽进另一个 reusable workflow；若未来确需使用 `workflow_call`，npm 侧仍配置最外层调用工作流文件名，并保持 `id-token: write` 传递链清晰。

第一阶段采用“受保护 GitHub Environment 审批后直接 `npm publish`”。如果后续希望把发布审批移到 npm，可把 Trusted Publisher 的 Allowed actions 改为仅允许 `npm stage publish`，工作流同步改为 staged publishing，再由维护者使用 2FA 审批；两种模式不能在失败时自动互相回退。

### 15.9 发布顺序、幂等与恢复

发布顺序固定为：

```text
校验 tag 与版本
  -> 完整质量门
  -> 多平台二进制构建与 smoke
  -> npm tarball 构建与 npm/pnpm 安装 smoke
  -> npm Trusted Publishing
  -> checksum / SBOM / provenance
  -> GitHub Release
```

先发布 npm、后创建 GitHub Release，是为了避免 Release 页面宣称版本可用而 npm 安装仍失败。若 npm 成功但 GitHub Release 失败，重跑时必须识别 npm 远端同版本并校验 integrity，然后只补齐 Release；禁止覆盖已发布 npm 版本。

恢复规则：

- 同一 tag、同一源码和同一依赖锁必须生成可复现的 npm tarball；integrity 不一致直接失败。
- 已存在 GitHub Release 时，仅可补上传内容一致的缺失制品，不能静默覆盖同名附件。
- tag 版本非法、tag 不指向当前 checkout、工作树生成结果漂移时立即失败；源码 `package.json.version` 固定为开发态占位值，不参与发布版本判定。
- `latest`、`next`、`beta` 只能由版本解析脚本映射，手动重跑不能修改 dist-tag。
- npm 发布成功后不得通过删除版本重发；修复必须提升版本号。

### 15.10 发布门禁

- pnpm frozen lockfile 安装；
- TypeScript typecheck、lint 和 Vitest；
- OpenAPI 生成结果无漂移；
- Gin 路由分类 100%，除内部可观测白名单外全部 HTTP API 与 OpenAPI 覆盖 100%；
- `business-command`、`protocol-adapter`、`client-entry` 与 `server-entry` 的运行时错误均符合共享稳定错误 Envelope，未出现未登记错误码；
- 所有允许 Bearer 调用的业务与协议路由具有稳定 Scope，OpenAPI 声明与服务端运行时映射 100% 一致，不存在意外 `system:unmapped`；
- `business-command` 与 `protocol-adapter` operation 的高层命令或协议适配器覆盖 100%，`api request` 不计入；
- 每项公开业务或协议能力的成功主路径通过率 100%；
- 登录、项目、应用、构建、发布、部署、日志、终端、导出、权限、MFA 和 Secret 脱敏等关键旅程通过率 100%；
- 在干净测试实例执行完整 operation 场景矩阵，通过率不低于 95%，且失败项没有 P0/P1 缺陷；
- Node.js npm tarball 与 Bun 二进制通过同一套 HTTP、代理、自定义 CA、SSE、WebSocket、取消和重定向安全测试；
- Bun 编译矩阵成功且各平台产物完成目标环境 smoke test；
- npm tarball 文件白名单和 `npm pack --dry-run --json` 通过；
- npm 与 pnpm 从同一 tarball 全局安装后均可运行 `luna`；
- npm 包、二进制、Git tag、Git SHA 和 OpenAPI 契约版本一致；
- Token 脱敏和配置权限测试；
- OAuth PKCE、Device Code、Refresh、Revoke、MFA、终端授权存活与数据导出票据绑定集成测试；
- Agent 模式、`params=@file|@-`、Schema 拒绝未知字段、敏感字段脱敏和受限命令发现契约测试；
- 服务端计划的 actor/authentication-context/target/params/version 绑定、过期、单次使用、重放和集合漂移安全测试；
- JSONL 版本首帧、事件关联、恢复去重、资源上限与唯一终态摘要协议测试；
- 提示注入、终端控制字符、恶意日志、越权工具选择、无限分页/轮询和 MFA 用户在场安全评估；
- Arazzo 关键工作流的 operationId、输入输出映射、批准点和后置条件校验；
- checksum、SBOM、签名和 provenance 生成成功；
- 正式发布流程不存在长期 npm 写入 Token；
- 预发布版本未写入 npm `latest`。

### 15.11 95% 可用度的计算方法

本文中的“可用度”是 CLI 功能场景通过率，不是服务端 SLA、网络可用率或请求成功率。它必须由干净测试实例上的确定性测试报告计算，不能由人工抽样估计。

每个 `business-command` 或 `protocol-adapter` 至少生成一个成功场景，并按元数据补充适用场景：

| 场景 | 适用条件 | 通过要求 |
| --- | --- | --- |
| `happy-path` | 全部命令 | 请求成功，输出符合稳定 Schema，资源状态符合预期 |
| `local-validation` | 存在输入参数 | 无效类型、枚举、必填和跨字段组合在发请求前被拒绝 |
| `unauthenticated` | 非公开接口 | 未登录返回稳定认证错误，不泄露服务端细节 |
| `forbidden` | 存在 Scope 或角色要求 | 低权限凭据被服务端拒绝，CLI 映射稳定退出码 |
| `not-found` | 操作具体资源 | 不存在资源返回稳定错误，不误作用于同名资源 |
| `pagination-sort` | 列表接口 | 翻页、排序、`all=true` 和数量上限行为正确 |
| `secret-redaction` | 涉及 Secret、Token 或凭据 | stdout、stderr、debug、配置与错误均不回显敏感值 |
| `idempotency-retry` | 声明幂等或自动重试 | 重试不重复创建资源，不安全写操作不自动重试 |
| `step-up-mfa` | 受保护操作 | OAuth 可完成验证并只重试一次，PAT 不能绕过 |
| `protocol-lifecycle` | SSE、WebSocket、下载、OAuth | 连接、取消、恢复、终态与本地资源清理完整 |
| `agent-schema` | 可由 Agent 调用的命令 | 受限发现、复杂参数、未知字段拒绝和输出 Schema 一致 |
| `server-plan` | 高风险操作 | 计划精确绑定 actor/认证上下文/目标/参数/版本，过期和重放失败 |
| `optimistic-concurrency` | 中高风险更新 | 陈旧版本返回冲突，不发生盲覆盖或自动强制写入 |
| `untrusted-output` | 日志、事件、仓库和第三方内容 | 提示注入和控制字符不触发命令、越权或终端控制 |
| `resource-bounds` | 分页、重试、轮询和流式命令 | 达到数量、时间或字节上限后确定性停止 |
| `postcondition` | 变更和异步命令 | 执行后重新读取状态，未达目标时不报告成功 |

场景分母由 OpenAPI 和 `x-luna-cli` 元数据生成。一个 operation 缺少本应适用的场景时视为失败，不能通过不生成测试来缩小分母。测试报告必须同时给出：

```text
routeTotal
publicOperationTotal
commandTotal
protocolAdapterTotal
scenarioTotal
scenarioPassed
scenarioFailed
scenarioEnvironmentSkipped
criticalJourneyTotal
criticalJourneyPassed
```

环境跳过项必须在测试运行前由测试环境清单声明，例如没有可用的 GitHub Enterprise、SMTP 或真实 Kubernetes 集群；运行后才发现的异常不能改记为跳过。核心成功主路径不能因为环境缺失而跳过：第三方平台使用确定性 fixture、协议模拟器或测试租户覆盖；每个 provider 家族至少保留一个真实集成 smoke。每个正式 Release 至少在一个具备 PostgreSQL、Redis、Kubernetes、Git Provider、Registry 和 OIDC 的完整测试环境执行。

关键旅程和每项公开业务/协议能力的 `happy-path` 均不允许跳过，且必须 100% 通过。95% 只用于输入错误、权限、冲突、重试、分页、异常中断等附加场景，不能允许 5% 的 API 整体不可用：

```text
成功主路径通过率 = happyPathPassed / publicCapabilityTotal = 100%
关键旅程通过率 = criticalJourneyPassed / criticalJourneyTotal = 100%
完整场景通过率 = scenarioPassed / (scenarioPassed + scenarioFailed) >= 95%
```

环境跳过项不进入比例分母，但必须单独报告数量、原因、Owner 和补测期限；一个 operation 如果没有成功主路径，直接视为失败而不是跳过。即使比例达到 95%，只要存在认证绕过、越权、Secret 泄露、错误资源写入、不可恢复的数据破坏、成功主路径失败或关键旅程失败，仍然禁止发布。

## 16. 实施阶段

### Phase 0：契约准备

- 建立根 pnpm workspace 和统一锁文件。
- 除内部可观测白名单外，补齐全部 HTTP API 的 OpenAPI、`operationId`、Scope、错误码和 `x-luna-cli`。
- 从 Gin Router 自动生成路由清单，完成 `business-command`、`protocol-adapter`、`client-entry`、`server-entry` 和 `internal-observability` 分类。
- 建立稳定错误码与错误 Envelope 的单一机器可读目录，迁移散落的后端错误响应，并由 Go、OpenAPI、Web 和 CLI 校验同一份定义。
- 增加路由、OpenAPI、Scope、命令和协议消费覆盖门禁；允许 Bearer 的路由不得返回 `system:unmapped`。
- 抽取环境无关的 `api-contract` 和 `api-client`。
- 新增 `/api/v1/meta` 服务端能力协商接口。
- 为可由 Agent 调用的 operation 补齐输入/输出 JSON Schema、敏感字段、风险、dry-run、并发、资源上限和批准元数据。
- 新增 Arazzo `openapi/workflows.yaml`，覆盖登录、授权、构建发布部署、MFA、导出和终端关键旅程。
- 为高风险变更建立短时、单次、精确绑定的服务端计划协议；为中高风险更新补齐 ETag/version 并发保护。

验收：Web 继续使用共享 Client，CLI 能生成命令覆盖报告，全部路由均已分类，非内部可观测白名单接口均可在共享 Client 中调用，Bearer 可调用路由的 OpenAPI Scope 与服务端运行时映射完全一致，所有公开错误响应符合稳定 Envelope；高风险操作不存在只靠 `yes=true` 的执行路径。

### Phase 1：CLI 基础

- Commander 命令树、全局参数和帮助。
- 单活动登录 auth.json、默认项目空间和原子写入。
- i18n、输出渲染、稳定错误和退出码。
- 统一 `HttpTransport` 及 Node/Bun 网络适配，覆盖代理、自定义 CA、重定向、取消、SSE 和 WebSocket fixture。
- 两级命令解析、`key=value` 类型转换、多行/文件/stdin 输入和聚合校验错误。
- `agent=true`、`params=@file|@-`、受限命令发现、版本化输出 Schema 和资源边界。
- `api request`、`help catalog`、`help command` 和 Completion。
- Bun 多平台单二进制构建。

验收：CLI 基础、普通 JSON Client、本地配置和机器输出可用；这是内部里程碑，不作为覆盖全部 API 的可发布版本。

### Phase 2：OAuth

- 公共第一方 CLI Client。
- Authorization Code + PKCE + loopback callback。
- Device Authorization Grant。
- Device Code 浏览器确认页、批准/拒绝和轮询终态。
- Token refresh、revoke、status 和 logout。
- Token Endpoint 支持 `token_endpoint_auth_method=none` 的内置公共客户端。
- Git Provider OAuth 授权事务和 `git authorize` 闭环。

验收：有浏览器、无浏览器和跨设备三种场景均可登录。

### Phase 3：业务命令

- 按项目空间、应用、构建、发布、部署、访问入口、集群、事件等域逐步提供高层命令。
- 所有公开 API 映射到业务命令或有明确分类。
- JSON、SSE、WebSocket 终端、二进制下载、异步等待、批量操作、版本化 JSONL、幂等写操作和执行后状态验证。

验收：公开 API 的高层命令或协议适配覆盖达到 100%，`api request` 不参与覆盖统计，文档示例可在测试实例执行。

### Phase 4：MFA 与发布加固

- OAuth 授权会话绑定的 Step-up MFA。
- Bearer Token 认证上下文可完成 MFA verify，并被 `requireStepUp` 正确识别。
- 敏感操作自动挑战与单次安全重试。
- npm/pnpm 全局安装、npm Trusted Publishing 和 dist-tag 管理。
- GitHub Release 多平台二进制、checksum、签名、SBOM、provenance 和升级提示。
- 人类可用性与 AI 命令契约测试。
- 为 CLI 配套 Skills 生成发布元数据并执行真实实例评估。

验收：CLI 可执行 Web Console、数据导出、Secret、凭据、kubeconfig 等受保护操作，且不能绕过 MFA；全部公开能力成功主路径和关键旅程 100% 通过、其余完整操作场景矩阵通过率不低于 95%。完成此 Phase 后才允许发布 `v0.1.0`。

## 17. 验收场景

1. 用户不指定服务端执行 `luna login` 时连接官方实例；登录自定义实例或其他账号时，新活动登录原子覆盖旧凭据并清除默认项目。
2. 用户在无本地浏览器的服务器使用 `luna auth login deviceCode=true` 登录。
3. CI 通过 `LUNA_TOKEN` 调用命令，凭据不落盘。
4. OAuth Access Token 到期后自动刷新，原命令只重试一次。
5. 敏感命令触发 OTP，验证后继续执行；非 TTY 返回结构化 MFA 错误。
6. `luna help catalog query=project limit=20 agent=true` 能让 AI 获取少量候选命令，再由 `help command` 获取完整参数和输出约束。
7. `luna project list agent=true` 在中英文环境下输出相同字段结构。
8. 服务端新增公开 API 后，未更新 OpenAPI 或命令分类会使 CI 失败。
9. Token 不出现在 `ps`、Shell history、debug 日志、错误或 Completion 中。
10. macOS、Linux 和 Windows 的下载产物无需 Node.js 或 Bun 即可运行。
11. AI Skills 只使用机器可读 Help 中存在的 CLI 命令，CLI 缺失时明确停止而不绕过到 REST API。
12. 字符串、布尔值、数字、枚举、数组、对象、空值、文件和 stdin 输入均有成功与失败契约测试。
13. 多行输入不会丢失换行，超过内联限制时返回稳定错误并提示使用 `@file` 或 `@-`。
14. 用户可通过命令参数、环境变量或本地配置选择默认可读/JSON 输出；Skills 仍对每条命令显式指定 `agent=true`。
15. npm 与 pnpm 从发布 tarball 全局安装后都提供相同的 `luna` 命令、版本和 Help。
16. GitHub Release 独立二进制在没有 Node.js 和 Bun 的目标环境中可以运行，并通过 `SHA256SUMS` 校验。
17. `latest` 只指向正式版本，`next` 和 `beta` 只指向对应预发布版本。
18. npm 发布使用 GitHub OIDC Trusted Publishing，仓库和 Environment 中不存在长期 npm 写入 Token。
19. 用户设置活动登录默认项目后命令解析到该项目；重新登录其他实例或账号会清除默认项目，单次 `project=` 和 `LUNA_PROJECT` 覆盖均不修改持久配置。
20. 默认项目空间被删除或失去权限时返回 `project_context_invalid`，不会自动选择其他项目空间。
21. AI Skills 对项目级变更默认显式传入不可变项目 ID，不会修改用户活动登录的默认项目。
22. 普通 `v*` 只触发平台项目发版，`cli-v*` 只触发 CLI 发版；两套工作流都拒绝对方的 tag 命名空间。
23. Gin 新增路由但没有路由分类时，CI 明确失败并列出路由；新增非内部可观测白名单路由但没有 OpenAPI、operationId 或对应命令、协议适配、服务端入口登记时同样失败。
24. 构建日志 SSE 能正常跟随和被 SIGINT 终止；断线时根据服务端是否支持游标决定恢复或提示重新读取。
25. Pod 与发布 WebSocket 终端支持 TTY resize、stdin、退出状态和异常后的本地终端恢复。
26. 数据导出默认拒绝覆盖已有文件，成功后原子落盘；`destination=@-` 只输出原始字节。
27. 创建构建或发布后，`wait=true` 能等待到终态，超时默认只停止本地等待，不误取消远端任务。
28. 使用代理、自定义 CA 和 `NO_PROXY` 的企业网络环境可以登录并调用 API，跨源重定向不会携带 Authorization。
29. CLI 连接不支持某项能力的旧实例时，根据 `/api/v1/meta` 返回 `unsupported_feature`，不会发送已知不兼容请求。
30. `output=json` 始终使用版本化 CLI Envelope；`output=raw-json` 只用于人工调试，AI Help 明确标记为禁止。
31. 全量测试报告同时展示总 operation 数、可执行 operation 数、通过数、失败数和环境跳过数；通过率不得通过删除失败用例或滥用跳过项提高。
32. 每项公开业务或协议能力至少有一个成功主路径且全部通过；95% 阈值只约束附加场景。
33. 本地登录、浏览器 Session、OIDC 回调、Webhook 和探针上报均进入契约与安全测试，但不会生成误导性的人工 CLI 命令。
34. 服务端业务、OAuth、MFA、SSE 握手、WebSocket 升级和下载错误均返回已登记稳定错误码；CLI 不匹配本地化 message。
35. 同一代理、自定义 CA、SSE、WebSocket、取消和跨源重定向 fixture 在 npm/Node.js 与 Bun 二进制上行为一致。
36. `luna git authorize` 能打开浏览器、等待授权事务并返回确定的 Git Account ID；拒绝、过期和回调失败不会被误报为成功。
37. Device Code 的浏览器确认页只能批准当前登录用户可见的请求，具备 CSRF、过期、单次兑换和轮询限流保护。
38. `params=@payload.json` 与 `params=@-` 按 JSON Schema 校验，未知字段、类型错误和敏感字段错误在请求前被拒绝。
39. Agent 使用带 query/category/risk/limit 的目录发现少量命令，再获取单命令完整 Schema；命令目录摘要变化后不会继续使用旧参数。
40. 高风险命令先返回绑定 actor、认证上下文、执行作用域、目标、参数、资源版本和过期时间的 `planId`；参数变化、跨账号复用、过期和二次执行均被拒绝。
41. 两个客户端并发修改同一资源时，陈旧版本返回退出码 `6`，不会覆盖较新的更改。
42. JSONL 长任务首行声明协议版本、每条事件可关联，且只在收到唯一终态摘要后报告成功。
43. Agent 分页、轮询和流式读取达到 `maxItems`、`timeout`、`maxEvents` 或 `maxBytes` 时确定性停止，不形成无限循环。
44. 日志、仓库文件和事件正文中的伪造指令不会触发 CLI 命令；ANSI、OSC 和双向控制字符不会控制终端。
45. MFA 挑战会暂停 Agent 工作流并要求用户在受控界面完成，OTP 和恢复码不会进入 Agent 对话、日志或命令参数。
46. 变更命令执行后重新读取资源验证后置条件；HTTP 成功但目标状态未达成时返回结构化失败或不确定状态。
47. Arazzo 关键工作流引用的 operationId、步骤输出、批准点和恢复路径均通过生成校验和集成测试。

## 18. 已确认的实施决策

以下决策已经确认，Phase 0 和后续实现不得再退回到较弱方案：

| 决策 | 已确认方案 | 原因 |
| --- | --- | --- |
| “支持所有 API”是否要求每条路由都有人工命令 | 不要求。全部非内部 HTTP API 进入低层契约；业务和协议能力 100% 提供高层命令；浏览器/服务端入口只做契约与安全测试 | Webhook callback、浏览器 Session 和探针接收端没有合理的人工命令语义，强行生成只会误导用户 |
| CLI 是否支持站点账号密码登录 | 不支持。CLI 只提供 Luna OAuth 浏览器登录、Device Code 和个人访问令牌 | 避免 CLI 收集密码、保存 Cookie Session 或复制 Web OIDC 流程；本地账号仍可在浏览器中登录后授权 CLI |
| Git Provider OAuth 如何闭环 | 新增短时授权事务 API，并由 `git authorize` 轮询事务状态 | 轮询 Git Account 列表无法可靠区分并发授权、更新账号和失败状态 |
| 第三方 Provider 是否每次发版都跑全部真实平台 | 每个 operation 使用确定性 fixture/模拟器；每个 Provider 家族至少一个真实集成 smoke | 对每个 Git/Registry/OIDC/SMTP 厂商建立永久真实环境成本过高且不稳定，但完全不做真实 smoke 又容易遗漏协议差异 |
| 未取得 Apple 签名凭据时如何发布 | macOS 独立二进制只作为预发布或标记未验证，不进入稳定下载矩阵；npm 与 Linux 制品可正常发布 | 避免规格宣称“可验证签名”但实际分发未签名稳定制品 |

签名资源、真实 Provider smoke 的具体账号和凭据仍在建立 CI 时配置，但不改变表中的发布边界。

## 19. 参考依据

- [Bun Single-file executable](https://bun.sh/docs/bundler/executables)
- [Commander.js](https://github.com/tj/commander.js/)
- [oclif Introduction](https://oclif.io/docs/introduction/)
- [openapi-typescript 与 openapi-fetch](https://openapi-ts.dev/openapi-fetch/)
- [OAuth 2.0 Device Authorization Grant, RFC 8628](https://www.rfc-editor.org/rfc/rfc8628)
- [Proof Key for Code Exchange, RFC 7636](https://www.rfc-editor.org/rfc/rfc7636)
- [OAuth 2.0 for Native Apps, RFC 8252](https://www.rfc-editor.org/rfc/rfc8252)
- [GitHub CLI `gh auth login`](https://cli.github.com/manual/gh_auth_login)
- [i18next Getting Started](https://www.i18next.com/overview/getting-started)
- [npm：全局安装包](https://docs.npmjs.com/downloading-and-installing-packages-globally/)
- [npm：`package.json` 与 `bin`](https://docs.npmjs.com/files/package.json/)
- [npm：发布公开 scoped package](https://docs.npmjs.com/creating-and-publishing-scoped-public-packages/)
- [npm：Trusted Publishing](https://docs.npmjs.com/trusted-publishers/)
- [npm：dist-tag](https://docs.npmjs.com/cli/dist-tag/)
- [GitHub Actions：发布 Node.js 包](https://docs.github.com/en/actions/tutorials/publish-packages)
- [GitHub CLI：JSON 字段、jq 与模板输出](https://cli.github.com/manual/gh_help_formatting)
- [GitHub CLI：API 输入、文件、stdin 与分页](https://cli.github.com/manual/gh_api)
- [Terraform：机器可读 UI 与 JSONL 事件协议](https://developer.hashicorp.com/terraform/internals/machine-readable-ui)
- [Terraform：Plan 命令](https://developer.hashicorp.com/terraform/cli/commands/plan)
- [Terraform：自动化中保存 Plan 再执行](https://developer.hashicorp.com/terraform/tutorials/automation/automate-terraform)
- [Claude Code CLI：结构化输入输出与工具权限](https://docs.anthropic.com/en/docs/claude-code/cli-usage)
- [OpenAI：Structured Outputs 与严格 JSON Schema](https://openai.com/index/introducing-structured-outputs-in-the-api/)
- [OpenAI：为 Agent 暴露少量相关工具并明确批准边界](https://developers.openai.com/api/docs/guides/latest-model)
- [OpenAPI Arazzo 1.1](https://spec.openapis.org/arazzo/latest.html)
- [JSON Schema Draft 2020-12](https://json-schema.org/draft/2020-12)
- [RFC 9457：Problem Details for HTTP APIs](https://www.rfc-editor.org/rfc/rfc9457)
- [kubectl run：client/server dry-run](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_run/)
- [OWASP AI Agent Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/AI_Agent_Security_Cheat_Sheet.html)
