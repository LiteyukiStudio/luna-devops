# 安装与使用

> Luna CLI 尚未完成首次公开发布。下面的包管理器命令会在对应版本发布后可用；如果 npm 返回 404，请使用仓库开发流程，不要从非官方来源下载同名包。

## 使用 npm 或 pnpm

稳定版：

```bash
npm install --global @liteyuki/luna-cli
pnpm add --global @liteyuki/luna-cli
```

预发布版必须显式选择通道：

```bash
npm install --global @liteyuki/luna-cli@next
pnpm add --global @liteyuki/luna-cli@beta
```

安装后验证：

```bash
luna --version
luna help catalog output=json interactive=false
```

npm 包要求 Node.js `22.14.0` 或更高版本。建议使用 Node.js 版本管理器或用户级 pnpm home，不要用 `sudo` 解决全局目录权限问题。

npm/pnpm 是 Windows、macOS、常规 Linux 发行版以及 Alpine/musl 的统一安装方式。它直接使用本机 Node.js 运行，不依赖 Bun 独立二进制。

## 使用独立二进制

首次稳定版只发布经过目标 runner 验证的 Linux glibc x64 和 arm64 制品。版本发布后，下载与系统匹配的文件和 `SHA256SUMS`：

```bash
version="cli-vX.Y.Z"
asset="luna-linux-x64"
base="https://github.com/LiteyukiStudio/luna-devops/releases/download/${version}"

curl -fL -o luna "${base}/${asset}"
curl -fL -o SHA256SUMS "${base}/SHA256SUMS"
grep " ${asset}$" SHA256SUMS | sed "s# ${asset}$# luna#" | sha256sum -c -
chmod +x luna
install -m 0755 luna "${HOME}/.local/bin/luna"
```

macOS 可把最后一步的 `sha256sum` 换成 `shasum -a 256`。不过在 Apple Developer ID 签名和公证接入前，macOS 只会在预发布版本提供带 `-unsigned` 后缀的测试制品，不建议用于生产环境。

Windows 与 Alpine/musl 暂不发布独立二进制。请使用 npm 或 pnpm 安装；这避免把 Bun 目标运行时下载、Windows 签名和 musl 动态库差异转嫁给用户。

## 多实例上下文

CLI 设计为同时管理多个 Luna DevOps 实例。上下文保存实例地址、凭据引用、默认项目空间、语言和输出偏好，配置默认位于 `~/.luna/`。

可以使用以下命令管理上下文：

```bash
luna context set name=production server=https://devops.example.com
luna context list output=json interactive=false
luna context use name=production
luna context current
```

自动化脚本应显式指定上下文或实例，并使用：

```text
output=json interactive=false
```

不要依赖彩色文本、列宽或本地化的人类输出。

## Shell Completion

命令注册器支持 Bash、Zsh、Fish 和 PowerShell Completion。使用 `luna completion bash`、`luna completion zsh`、`luna completion fish` 或 `luna completion powershell` 生成脚本；具体输出契约可通过 `luna help command path=completion.zsh output=json interactive=false` 查看。

## 卸载

```bash
npm uninstall --global @liteyuki/luna-cli
pnpm remove --global @liteyuki/luna-cli
rm "${HOME}/.local/bin/luna"
```

卸载不会自动删除 `~/.luna/`。凭据应通过 CLI 登出命令显式撤销并清理；只有确认不再需要任何上下文时，才手动删除该目录。
