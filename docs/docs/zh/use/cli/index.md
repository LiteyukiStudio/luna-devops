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

不需要记住全部命令，使用分层帮助查找参数、权限和示例：

```bash
luna --help
luna project --help
luna project get-projects --help
```

遇到连接、认证或版本问题时先运行 `luna doctor`。

## Scope 与项目角色

CLI 帮助中的“所需 Scope”来自平台发布的 OpenAPI 契约。OAuth 登录签发的 Scope 是凭证能力上限，不会提升账号的平台角色或项目空间角色；普通项目成员访问项目资源时，凭证 Scope 和项目角色必须同时允许，平台管理员也不能绕过凭证 Scope。

同一 OAuth 应用在不同设备或终端上的登录会形成独立会话。退出当前登录或撤销当前 Token 只影响该会话；在账号授权管理中撤销整个应用授权时，该应用的全部会话都会失效。

## kubectl kubeconfig

使用 `luna kubeconfig write` 可以创建 Kube Credential 并以 `0600` 权限原子写入新文件；使用 `luna kubeconfig merge` 可以在检查同名冲突后合并到现有 kubeconfig。这两个专用命令不会把一次性 kubeconfig 或 Token 输出到普通 stdout，且需要当前 OAuth 会话具有 `token:manage`。

命令参数、安全合并方式、Context 规则和 kubectl 权限边界见[使用 kubectl 管理项目资源](/use/kubectl)。

## 脚本与 Agent

自动化应使用 JSON 输出，并关闭交互：

```bash
luna project get-projects output=json interactive=false
```

高风险操作仍需明确确认，CLI 不会绕过平台权限。Token 应从环境变量或密钥服务传入，不要写进命令历史。具体命令和参数以 `luna help` 为准。
