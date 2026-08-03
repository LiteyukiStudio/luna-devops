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
