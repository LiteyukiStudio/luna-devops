# 安装 Luna CLI

Luna CLI 的官方 npm 包是 `@liteyuki/luna-cli`，需要 Node.js `22.14.0` 或更高版本。

## 安装

使用 npm 安装：

```bash
npm install --global @liteyuki/luna-cli
```

也可以使用 `pnpm add --global @liteyuki/luna-cli`。

安装后确认命令可用：

```bash
luna --version
```

建议使用 Node.js 版本管理器或用户级 pnpm home，不要用 `sudo` 解决全局目录权限问题。

## 为 AI 安装配套 Skill

可以把下面的提示词直接复制给支持 Skill 的 AI 编码助手。配套 Skill 发布在 [Luna CLI Releases](https://github.com/LiteyukiStudio/luna-cli/releases)，必须与已安装的 Luna CLI 版本完全一致，安装包文件名为 `luna-devops-<version>.skill`。

```text
请为当前 AI 编码环境安装 Luna CLI 配套的 `luna-devops` Skill。

1. 先运行 `luna version show output=json interactive=false agent=true`，读取当前 Luna CLI 版本。
2. 打开 https://github.com/LiteyukiStudio/luna-cli/releases/latest。如果最新 Release 与当前 CLI 版本不一致，请在 Releases 中选择与 CLI 完全相同的版本。
3. 下载该 Release 附带的 `luna-devops-<version>.skill`，并使用当前 AI 环境支持的 Skill 安装方式完成安装。
4. 不要从仓库源码目录复制 Skill，也不要混用不同版本。
5. 安装完成后，确认 `luna-devops` Skill 可用，并告诉我 CLI 版本、Skill 版本和安装结果。如果需要重新加载 AI 环境或在下一轮对话中生效，也请明确说明。
```

## 登录

登录 Luna DevOps：

```bash
luna login
luna whoami
```

自托管实例可通过 `luna login server=https://devops.example.com` 指定地址。完成后即可继续阅读[使用 Luna CLI](./cli)。

## 更新

```bash
npm update --global @liteyuki/luna-cli
```

使用 pnpm 时运行 `pnpm update --global @liteyuki/luna-cli`。预发布版本和其他发行制品见 [Luna CLI Releases](https://github.com/LiteyukiStudio/luna-cli/releases)。

## 卸载

```bash
npm uninstall --global @liteyuki/luna-cli
# 或
pnpm remove --global @liteyuki/luna-cli
```

卸载前建议先运行 `luna logout` 撤销登录。卸载命令不会自动删除 `~/.luna/` 中的本地配置。
