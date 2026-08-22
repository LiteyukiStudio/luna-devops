# 安装 Luna CLI

Luna CLI 的官方 npm 包是 `@liteyuki/luna-cli`，需要 Node.js `22.14.0` 或更高版本。

```bash
npm install --global @liteyuki/luna-cli
luna --version
```

也可以使用 `pnpm add --global @liteyuki/luna-cli`。建议使用 Node.js 版本管理器或用户级 pnpm home，不要用 `sudo` 规避全局目录权限问题。

## 登录

```bash
luna login
luna whoami
```

自托管实例使用 `luna login server=https://devops.example.com`。登录后继续阅读[使用 Luna CLI](./index)。

## 更新与卸载

```bash
npm update --global @liteyuki/luna-cli
npm uninstall --global @liteyuki/luna-cli
```

使用 pnpm 时将 `npm` 命令替换为 `pnpm update --global` 或 `pnpm remove --global`。卸载前建议运行 `luna logout` 撤销登录；卸载不会自动删除 `~/.luna/` 配置。

配套 Agent Skill 与 CLI 必须使用同一版本，可从 [Luna CLI Releases](https://github.com/LiteyukiStudio/luna-cli/releases) 下载对应的 `luna-devops-<version>.skill`。
