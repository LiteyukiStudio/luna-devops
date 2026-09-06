# 使用 Luna CLI

Luna CLI 用于在终端或自动化脚本中管理 Luna DevOps。尚未安装时先阅读[安装](./installation)。

## 命令格式

```text
luna <分类> <操作> key=value
```

例如：

```bash
luna project list
luna project use project=xnn-api
```

列表命令默认使用 `visibility=related`。平台管理员只有在明确需要全局结果时才传入 `visibility=all`，例如 `luna project list visibility=all`；已知项目空间的资源查询应继续传入项目引用缩小结果。

不需要记住全部命令，使用分层帮助查找参数和示例：

```bash
luna --help
luna project --help
luna project list --help
```

遇到连接、认证或版本问题时先运行 `luna doctor`。

## ID 与可读标识

命令参数沿用 API 的 `projectId`、`applicationId` 和 `targetId` 名称，但人工使用时既可传稳定 ID，也可传作用域内不可变的精确标识：

- `projectId`：随机生成、不可变且非自增的 `prj_...` 主键，或项目空间标识符，例如 `xnn-api`；
- `applicationId`：`app_...`，或已选项目空间内的应用标识符；
- `targetId`：`dplt_...`，或已选应用内的部署阶段，例如 `prod`。

CLI 会逐层精确解析这些引用，并在确认高风险操作和发送业务请求前转换为稳定 ID。它不会用显示名称做模糊猜测；引用不存在或不唯一时会返回稳定错误码，并提示对应的列表命令。

`releaseId` 是随机生成、不可变的 `rel_...` 单次发布记录 ID，不是应用 ID、部署目标 ID 或 Kubernetes 工作负载名称。例如 `luna-postgres-w8kt4h-prod` 这类名称表示部署目标生成的 Kubernetes 资源名称，不能作为 `releaseId`。需要进入当前工作负载时，应使用下面的部署目标命令。

## 进入部署目标交互终端

需要在运行容器中交互排障时，使用 OAuth 登录并执行：

```bash
luna deployment exec projectId=xnn-api applicationId=postgres-api targetId=prod
luna deployment exec projectId=prj_111111111111111111111111 applicationId=app_222222222222222222222222 targetId=dplt_333333333333333333333333 container=api
```

连接建立后，本地终端会直接进入部署目标当前工作负载容器的 Shell。执行 `exit`
或按 `Ctrl-D` 会结束远端会话并恢复本地终端。`deployment terminal` 是同一人工命令的别名。旧的 `release exec` 入口仅为兼容已有用法保留；它仍需要真实的 `rel_...`，且连接的也是该发布所属部署目标的当前工作负载，而不是历史发布快照。

该命令需要交互式 TTY、当前账号具备相应项目权限，并要求项目空间和部署配置允许
运行终端访问。它不会下发集群凭据，也不能在脚本或 Agent 模式中运行。

## 从 Web 复制命令

应用和集群的 Web Console 会在命令栏显示对应的 Luna CLI 命令。点击复制按钮可将命令复制到本机终端，问号按钮会打开这份 CLI 文档。

平台生成的命令会携带当前 Luna DevOps 实例地址和页面已知的稳定资源 ID，不包含 Token、一次性终端票据、`--yes` 或 TLS 绕过选项。CLI 使用本机当前登录账号执行，服务端仍会重新检查权限和运行终端策略；浏览器中能看到复制入口不代表命令一定获准执行。

## 权限与会话

Luna CLI 登录后拥有与当前账号相同的权限，不需要选择或维护额外 Scope。平台会在每次请求时按账号的平台角色、项目空间成员关系和资源策略重新判断权限；CLI 不会扩大或缓存账号权限。个人令牌和第三方 OAuth 应用仍使用各自独立的授权范围。

同一 OAuth 应用在不同设备或终端上的登录会形成独立会话。退出当前登录或撤销当前 Token 只影响该会话；在账号授权管理中撤销整个应用授权时，该应用的全部会话都会失效。

## 脚本与 Agent

自动化应使用 JSON 输出，并关闭交互：

```bash
luna project list output=json interactive=false
```

严格的 `agent=true` 模式不解析可读引用，资源参数必须传入稳定 ID。

高风险操作仍需明确确认，CLI 不会绕过平台权限。Token 应从环境变量或密钥服务传入，不要写进命令历史。具体命令和参数以 `luna help` 为准。
