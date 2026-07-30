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

帮助会列出参数、权限要求、风险级别和示例。

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

只解析标准输出中的 JSON，不要依赖表格宽度、颜色或本地化文字。高风险操作在交互终端中会要求确认；非交互环境必须显式传入 `--yes`，但它不会绕过平台权限或安全验证。

需要通过个人令牌登录时，从环境变量或密钥服务传入，不要把令牌直接写进命令历史：

```bash
printf '%s' "$LUNA_TOKEN" | luna login mode=access-token token=@-
```

## Agent 使用

Agent 应使用：

```text
output=json interactive=false agent=true
```

配套的 `luna-devops` Skill 可从 [Luna CLI 仓库](https://github.com/LiteyukiStudio/luna-cli/tree/main/skills/luna-devops) 获取。具体命令和参数始终以 `luna help` 为准。
