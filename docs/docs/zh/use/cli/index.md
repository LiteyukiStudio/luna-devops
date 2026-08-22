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

不需要记住全部命令，使用分层帮助查找参数、权限和示例：

```bash
luna --help
luna project --help
luna project get-projects --help
```

遇到连接、认证或版本问题时先运行 `luna doctor`。

## 脚本与 Agent

自动化应使用 JSON 输出，并关闭交互：

```bash
luna project get-projects output=json interactive=false
```

高风险操作仍需明确确认，CLI 不会绕过平台权限。Token 应从环境变量或密钥服务传入，不要写进命令历史。具体命令和参数以 `luna help` 为准。
