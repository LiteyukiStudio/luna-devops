# 使用 Luna CLI

Luna CLI 用于在终端中管理 Luna DevOps。还没有安装时，请先阅读[安装 Luna CLI](./installation)。

## 命令格式

业务命令使用统一格式：

```text
luna <分类> <操作> key=value
```

例如：

```bash
luna project get-projects
luna project use project=prj_example
```

设置默认项目空间后，后续项目级命令可以省略项目 ID。

## 查找命令

不需要记住所有命令，按层级查看帮助即可：

```bash
luna --help
luna project --help
luna project get-projects --help
```

帮助会列出参数、权限要求和示例。

## 常用操作

```bash
luna whoami
luna doctor
luna project get-projects
luna logout
```

`luna doctor` 会检查当前登录、服务连接和版本兼容性，遇到连接或认证问题时优先运行它。

## 脚本与自动化

脚本应使用机器可读输出：

```bash
luna project get-projects output=json interactive=false
```

自动化脚本只应解析 JSON。高风险操作仍需明确确认，且不会绕过平台权限或安全验证。

需要通过个人令牌登录时，从环境变量或密钥服务传入，不要把令牌直接写进命令历史：

```bash
printf '%s' "$LUNA_TOKEN" | luna login mode=access-token token=@-
```

Agent 可使用配套的 [`luna-devops` Skill](https://github.com/LiteyukiStudio/luna-cli/tree/main/skills/luna-devops)。具体命令和参数以 `luna help` 为准。
