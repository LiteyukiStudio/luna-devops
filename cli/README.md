# Luna CLI

Luna CLI 是 Luna DevOps 的命令行客户端，面向终端用户和自动化 Agent。命令采用固定的两级结构：

```text
luna <工具分类> <具体工具> key=value
```

English documentation follows the Chinese section.

## 当前状态

CLI 目前处于预发布阶段。源码清单使用 `0.0.0-development` 占位版本和 `private: true` 防误发布，实际版本由 `cli-v*` tag 在发布时注入。仓库中已经实现：

- 多实例和项目空间上下文的数据模型、解析与本地存储；
- Access Token 登录、校验和本地凭据存储基础能力；
- `key=value`、JSON、文件和标准输入参数解析；
- 人类可读输出与稳定 JSON Envelope；
- 本地命令注册、帮助目录、Shell Completion 和 OpenAPI 命令注册器；
- 从 OpenAPI 生成并注册全部 110 个已登记操作，连同本地命令共 131 条；
- npm 包、Linux/macOS Bun 独立二进制的 CI、打包、安装 smoke 与发布门禁。

`cli/src/entry.ts` 已作为 npm 与 Bun 二进制的统一入口，共享契约和客户端会被安全打包进发布产物。预发布版本已经发布到 npm，且发布产物会经过 npm/pnpm 全局安装、中文帮助、机器 Help 和受支持独立二进制的 smoke。

当前明确未完成的能力包括：OpenAPI 尚未覆盖的后端公开路由、服务端能力协商客户端、Authorization Code + PKCE、Device Code、Bearer Step-up MFA、SSE/WebSocket/下载协议适配和中高风险服务端执行计划。高风险 API 在计划协议完成前会 fail closed，不会被 `yes=true` 绕过。

## 安装

可以通过 npm 或 pnpm 安装预发布通道：

```bash
npm install --global @liteyuki/luna-cli@beta
pnpm add --global @liteyuki/luna-cli@beta
```

也可以从 GitHub Release 下载独立二进制。稳定版当前只计划发布经过目标环境 smoke test 的 Linux glibc 制品；macOS 在接入代码签名与公证之前，只会在预发布版本提供名称带 `-unsigned` 的测试制品。Windows 与 Alpine/musl 请使用 npm 或 pnpm 安装，并通过 Node.js `22.14.0` 或更高版本运行。

## 不依赖 Skills 使用

CLI 自带面向人类的分层帮助，不需要先安装 AI Skills：

```bash
luna
luna --help
luna project --help
luna project get-projects --help
```

直接运行 `luna` 且不传子命令时，会显示同一份本地化根帮助，不会执行远程操作。第一级列出分类和快速开始，第二级列出分类内工具，第三级显示接口、权限、风险、参数来源、必填项和示例。业务参数统一使用 `key=value`；JSON、文件或多行文本使用 `key=@file.json` 或 `key=@-`。

语言解析顺序为：`--lang`、`LUNA_LANG`、当前 context 的 `language`、系统 `LC_ALL` / `LC_MESSAGES` / `LANG`、运行时语言，最后回退英文。例如：

```bash
LUNA_LANG=zh-CN luna --help
luna --lang zh-CN project get-projects --help
luna context set name=production server=https://devops.example.com language=zh-CN
```

npm 的 `latest` 与 `beta` 是独立更新通道。测试预发布版本时必须显式安装
`@beta`，普通的全局更新不会从稳定版自动切换到预发布版。

AI Skills 会在此基础上使用 `luna help catalog ... agent=true` 和
`luna help command ... agent=true` 获取稳定 JSON 契约，让执行更准确；CLI 本身不依赖 Skills。
Skills 与 CLI 使用相同版本并由同一个 `cli-v*` GitHub Release 发布，安装时
必须选择与本地 CLI 完全相同版本的
`luna-devops-<version>.skill`。该文件内部按领域拆分 `references/`，由 Agent
根据任务按需加载，不需要分别安装多个 Skill。

详细说明：

- [中文 CLI 文档](https://luna-devops.liteyuki.org/guide/cli/)
- [English CLI documentation](https://luna-devops.liteyuki.org/en/guide/cli/)
- [完整设计规格](../notes/cli-spec.md)

## 开发验证

从仓库根目录执行：

```bash
pnpm install --frozen-lockfile
pnpm --filter @liteyuki/luna-cli typecheck
pnpm --filter @liteyuki/luna-cli lint
pnpm --filter @liteyuki/luna-cli test
pnpm --filter @liteyuki/luna-cli build
node --test scripts/cli/tests/*.test.mjs
node scripts/cli/verify-skills-sync.mjs
```

`cli-v*` tag 用于 Luna CLI 与同版本 Skills 的配套发布，不会触发平台的 `v*`
发布流程。

---

## English

Luna CLI is the command-line client for Luna DevOps, designed for both people and automation agents:

```text
luna <category> <tool> key=value
```

### Current status

The CLI is in prerelease. The source manifest uses the `0.0.0-development` placeholder and `private: true` to prevent accidental publication; release versions are injected from `cli-v*` tags. It includes contexts, Access Token authentication, structured input and output, command discovery, 110 OpenAPI operations plus local commands, and release validation.

`cli/src/entry.ts` is the shared npm and Bun entry point, and workspace packages are bundled safely into the distribution. Prereleases are available on npm and pass npm/pnpm global-install, localized Help, machine Help, and supported standalone-binary smoke tests.

Undocumented server routes, capability negotiation in the client, Authorization Code + PKCE, Device Code, Bearer step-up MFA, streaming transports, downloads, and server-issued plans remain release work. High-risk API operations fail closed until the server-plan protocol exists.

### Installation

```bash
npm install --global @liteyuki/luna-cli@beta
pnpm add --global @liteyuki/luna-cli@beta
```

Standalone binaries will also be attached to GitHub Releases. Stable releases currently include only Linux glibc binaries that pass target-environment smoke tests. Until Apple signing is configured, macOS binaries are available only on prereleases and are explicitly suffixed with `-unsigned`. Windows and Alpine/musl use the npm or pnpm distribution on Node.js `22.14.0` or later.

See the documentation links above for installation, release channels, checksums, SBOMs, provenance, and current limitations.

The CLI includes layered human Help without requiring Skills:

```bash
luna
luna --help
luna project --help
luna project get-projects --help
```

Running `luna` without a subcommand displays the same localized root Help and
does not perform a remote operation. Locale precedence is `--lang`,
`LUNA_LANG`, context `language`, system locale, then English. Use
`LUNA_LANG=zh-CN luna --help` for Chinese. npm `latest` and `beta` are separate
update channels, so prerelease testing must explicitly install `@beta`. Skills
build on the CLI's machine-readable Help for more precise agent operation; the
CLI does not depend on Skills. Skills use the exact same version and ship in the
same `cli-v*` GitHub Release as the CLI.
