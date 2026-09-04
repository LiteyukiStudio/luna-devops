# 使用 Luna CLI

Luna CLI 用于在终端或自动化脚本中管理 Luna DevOps。尚未安装时先阅读[安装](./installation)。

## 命令格式

```text
luna <分类> <操作> key=value
```

例如：

```bash
luna project get-projects
luna project use project=prj_example
```

列表命令默认使用 `visibility=related`。平台管理员只有在明确需要全局结果时才传入 `visibility=all`，例如 `luna project get-projects visibility=all`；已知项目空间的资源查询应继续传入项目 ID 缩小结果。

不需要记住全部命令，使用分层帮助查找参数和示例：

```bash
luna --help
luna project --help
luna project get-projects --help
```

遇到连接、认证或版本问题时先运行 `luna doctor`。

## 进入 Release 交互终端

需要在运行容器中交互排障时，使用 OAuth 登录并执行：

```bash
luna release exec projectId=prj_example releaseId=rel_example
luna release exec projectId=prj_example releaseId=rel_example container=api
```

连接建立后，本地终端会直接进入 Release 当前工作负载容器的 Shell。执行 `exit`
或按 `Ctrl-D` 会结束远端会话并恢复本地终端。`release terminal` 是同一人工命令的
别名。

该命令需要交互式 TTY、当前账号具备相应项目权限，并要求项目空间和部署配置允许
运行终端访问。它不会下发集群凭据，也不能在脚本或 Agent 模式中运行。

## 权限与会话

Luna CLI 登录后拥有与当前账号相同的权限，不需要选择或维护额外 Scope。平台会在每次请求时按账号的平台角色、项目空间成员关系和资源策略重新判断权限；CLI 不会扩大或缓存账号权限。个人令牌和第三方 OAuth 应用仍使用各自独立的授权范围。

同一 OAuth 应用在不同设备或终端上的登录会形成独立会话。退出当前登录或撤销当前 Token 只影响该会话；在账号授权管理中撤销整个应用授权时，该应用的全部会话都会失效。

## 脚本与 Agent

自动化应使用 JSON 输出，并关闭交互：

```bash
luna project get-projects output=json interactive=false
```

高风险操作仍需明确确认，CLI 不会绕过平台权限。Token 应从环境变量或密钥服务传入，不要写进命令历史。具体命令和参数以 `luna help` 为准。
