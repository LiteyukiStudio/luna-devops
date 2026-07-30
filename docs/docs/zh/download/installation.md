# 安装 Luna CLI

Luna CLI 的官方 npm 包是 `@liteyuki/luna-cli`，需要 Node.js `22.14.0` 或更高版本。

## 安装

选择一种包管理器即可：

```bash
npm install --global @liteyuki/luna-cli
```

```bash
pnpm add --global @liteyuki/luna-cli
```

安装后确认命令可用：

```bash
luna --version
luna --help
```

建议使用 Node.js 版本管理器或用户级 pnpm home，不要用 `sudo` 解决全局目录权限问题。

## 登录

登录官方实例：

```bash
luna login
luna whoami
```

登录其他 Luna DevOps 实例：

```bash
luna login server=https://devops.example.com
```

完成后即可继续阅读[使用 Luna CLI](./cli)。

## 更新

```bash
npm update --global @liteyuki/luna-cli
```

或：

```bash
pnpm update --global @liteyuki/luna-cli
```

需要测试预发布版本时，显式安装 `beta` 通道：

```bash
pnpm add --global @liteyuki/luna-cli@beta
```

其他发行制品可在 [Luna CLI Releases](https://github.com/LiteyukiStudio/luna-cli/releases) 查看。

## 卸载

```bash
npm uninstall --global @liteyuki/luna-cli
# 或
pnpm remove --global @liteyuki/luna-cli
```

卸载前建议先运行 `luna logout` 撤销登录。卸载命令不会自动删除 `~/.luna/` 中的本地配置。
