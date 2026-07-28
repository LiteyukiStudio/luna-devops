# 安装与使用

> Luna CLI 的官方 npm 包是 `@liteyuki/luna-cli`。预发布版与正式版使用不同
> dist-tag；请只从 npm 官方仓库或项目 GitHub Releases 安装，不要下载非官方同名包。

## 使用 npm 或 pnpm

稳定版：

```bash
npm install --global @liteyuki/luna-cli
pnpm add --global @liteyuki/luna-cli
```

预发布版必须显式选择通道：

```bash
npm install --global @liteyuki/luna-cli@beta
pnpm add --global @liteyuki/luna-cli@beta
```

安装后验证：

```bash
luna --version
luna --help
luna help catalog output=json interactive=false
```

npm 包要求 Node.js `22.14.0` 或更高版本。建议使用 Node.js 版本管理器或用户级 pnpm home，不要用 `sudo` 解决全局目录权限问题。

npm/pnpm 是 Windows、macOS、常规 Linux 发行版以及 Alpine/musl 的统一安装方式。它直接使用本机 Node.js 运行，不依赖 Bun 独立二进制。

## 使用独立二进制

首次稳定版只发布经过目标 runner 验证的 Linux glibc x64 和 arm64 制品。版本发布后，下载与系统匹配的文件和 `SHA256SUMS`：

```bash
version="vX.Y.Z"
asset="luna-linux-x64"
base="https://github.com/LiteyukiStudio/luna-cli/releases/download/${version}"

curl -fL -o luna "${base}/${asset}"
curl -fL -o SHA256SUMS "${base}/SHA256SUMS"
grep " ${asset}$" SHA256SUMS | sed "s# ${asset}$# luna#" | sha256sum -c -
chmod +x luna
install -m 0755 luna "${HOME}/.local/bin/luna"
```

macOS 可把最后一步的 `sha256sum` 换成 `shasum -a 256`。不过在 Apple Developer ID 签名和公证接入前，macOS 只会在预发布版本提供带 `-unsigned` 后缀的测试制品，不建议用于生产环境。

Windows 与 Alpine/musl 暂不发布独立二进制。请使用 npm 或 pnpm 安装；这避免把 Bun 目标运行时下载、Windows 签名和 musl 动态库差异转嫁给用户。

## 登录实例

CLI 在 `~/.luna/auth.json` 中只保存一个活动实例和账号凭据，行为接近
`docker login`。重新登录其他实例或账号时，原有活动登录和默认项目空间会被覆盖，
不需要也不提供 context 切换。

未指定地址时登录官方实例：

```bash
luna login
```

这等价于登录 `https://devops.liteyuki.org`。登录其他实例必须显式提供地址：

```bash
luna login server=https://devops.example.com
printf '%s' "$LUNA_TOKEN" | luna login mode=access-token token=@-
```

`luna whoami` 查看当前登录，`luna logout` 撤销并清理它。自动化脚本应明确提供
凭据，并使用：

```text
output=json interactive=false
```

不要依赖彩色文本、列宽或本地化的人类输出。

## 帮助与语言

面向人类使用时，从以下三级帮助逐步查找命令：

```bash
luna
luna --help
luna project --help
luna project get-projects --help
```

直接运行 `luna` 且不传子命令时，会展示本地化的根帮助，不会执行任何远程操作。第三级帮助会列出业务参数是否必填、类型、输入来源、风险、Scope、接口和示例。业务参数使用 `key=value`；文件、JSON 和多行内容使用 `key=@file` 或 `key=@-`。

临时切换中文：

```bash
LUNA_LANG=zh-CN luna --help
luna --lang zh-CN project get-projects --help
```

优先级为 `--lang` > `LUNA_LANG` > 本地配置的 `language` > 系统语言 > 英文。
修改环境变量后应重新启动当前命令；已经安装的旧预发布版本需要升级到最新
`beta` 才能获得完整语言检测。

`latest` 与 `beta` 是两个独立的 npm 更新通道。`pnpm update --global
@liteyuki/luna-cli` 只会跟随当前稳定通道，不会自动切换到预发布版。需要测试
beta 时请显式执行：

```bash
pnpm add --global @liteyuki/luna-cli@beta
rehash
luna --version
which -a luna
pnpm list --global @liteyuki/luna-cli
```

如果 `luna --help` 仍显示英文，先确认版本和实际命令路径，避免 PATH 中残留的旧
`luna` 覆盖 pnpm 安装版本。

## Shell Completion

命令注册器支持 Bash、Zsh、Fish 和 PowerShell Completion。使用 `luna completion bash`、`luna completion zsh`、`luna completion fish` 或 `luna completion powershell` 生成脚本；具体输出契约可通过 `luna help command path=completion.zsh output=json interactive=false` 查看。

## 卸载

```bash
npm uninstall --global @liteyuki/luna-cli
pnpm remove --global @liteyuki/luna-cli
rm "${HOME}/.local/bin/luna"
```

卸载不会自动删除 `~/.luna/`。凭据应通过 `luna logout` 显式撤销并清理；
只有确认不再需要本地登录信息时，才手动删除该目录。
