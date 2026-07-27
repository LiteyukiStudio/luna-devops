# Luna CLI

Luna CLI 是 Luna DevOps 的命令行客户端，面向终端用户和自动化 Agent。命令采用固定的两级结构：

```text
luna <工具分类> <具体工具> key=value
```

English documentation follows the Chinese section.

## 当前状态

CLI 目前处于预发布阶段。源码清单使用 `0.0.0-development` 占位版本和 `private: true` 防误发布，实际版本由 `cli-v*` tag 在发布时注入。仓库中已经实现：

- 单一活动实例、账号凭据和默认项目空间的配置模型与本地存储；
- 默认使用 OAuth Device Code 登录、自动刷新与尽力吊销，并支持显式的个人访问令牌备用登录；
- `key=value`、JSON、文件和标准输入参数解析；
- 人类可读输出与稳定 JSON Envelope；
- 本地命令注册、帮助目录、Shell Completion 和 OpenAPI 命令注册器；
- 从 OpenAPI 生成并注册普通业务 HTTP 命令，并为特殊传输提供显式协议命令；
- 面向人类的 `login`、`logout`、`whoami`、`doctor` 顶层短命令；
- 检查当前登录、认证、服务端版本、OpenAPI 契约和能力开关的 `health doctor` 诊断；
- 在每个 OpenAPI 业务命令前自动协商 API 代际、最低 CLI 版本和契约摘要，并按实例缓存成功结果；
- npm 包、Linux/macOS Bun 独立二进制的 CI、打包、安装 smoke 与发布门禁。

`cli/src/entry.ts` 已作为 npm 与 Bun 二进制的统一入口，共享契约和客户端会被安全打包进发布产物。预发布版本已经发布到 npm，且发布产物会经过 npm/pnpm 全局安装、中文帮助、机器 Help 和受支持独立二进制的 smoke。

普通 OpenAPI 业务命令会自动读取 `/api/v1/meta` 并在不兼容时 fail
closed；`luna doctor` 用于主动查看详细诊断。标记为 hidden 的浏览器回调、
Webhook、内部接收器和底层协议操作不会注册为 canonical raw command，SSE、
下载和终端等能力只通过对应的专用协议命令提供。`high` 和 `critical` 风险操作
在交互终端中必须逐次明确确认；非交互或 Agent 模式必须显式传入 `--yes`，
否则以稳定的 `confirmation_required` 错误拒绝。CLI 的确认只表示调用意图，
后端权限、Scope 与 Step-up MFA 仍是最终安全裁决。通用 `api request` 仅保留为
人类诊断逃生口，不参与业务能力伪装。终端和数据导出要求 CLI OAuth 登录与
对应 purpose 的有效 Step-up；个人访问令牌不能满足或绕过这一协议授权。
覆盖数量与比例不在本文维护，以 `pnpm check:platform-cli-coverage` 的实时输出为准。

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
luna login
luna login server=https://devops.example.com
printf '%s' "$LUNA_TOKEN" | luna login mode=access-token token=@-
luna whoami
luna doctor
luna logout
luna project --help
luna project get-projects --help
```

这些顶层短命令只面向人类交互，分别复用 `auth login`、`auth status`、
`health doctor` 和 `auth logout` 的同一处理器。脚本与 AI 应使用 canonical
两级命令；严格 `agent=true` 模式会拒绝顶层别名，避免审计和机器契约出现两套路径。

直接运行 `luna` 且不传子命令时，会显示同一份本地化根帮助，不会执行远程操作。第一级列出分类和快速开始，第二级列出分类内工具，第三级显示接口、权限、风险、参数来源、必填项和示例。业务参数统一使用 `key=value`；JSON、文件或多行文本使用 `key=@file.json` 或 `key=@-`。

未指定 `server` 时，`luna login` 固定登录官方实例
`https://devops.liteyuki.org`。登录其他实例时必须显式传入
`server=https://...`；再次登录会覆盖本地现有的实例、凭据和默认项目空间。
CLI 不提供 context 切换机制，一个本地配置始终只表示一个活动登录。

语言解析顺序为：`--lang`、`LUNA_LANG`、本地配置的 `language`、系统
`LC_ALL` / `LC_MESSAGES` / `LANG`、运行时语言，最后回退英文。例如：

```bash
LUNA_LANG=zh-CN luna --help
luna --lang zh-CN project get-projects --help
```

npm 的 `latest` 与 `beta` 是独立更新通道。测试预发布版本时必须显式安装
`@beta`，普通的全局更新不会从稳定版自动切换到预发布版。

AI Skills 会在此基础上使用
`luna help catalog ... output=json interactive=false agent=true` 和
`luna help command ... output=json interactive=false agent=true`
获取稳定 JSON 契约。Skill 发起的每条命令都固定使用这三个参数，不依赖本地
默认输出或交互状态；CLI 本身不依赖 Skills。
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

The CLI is in prerelease. The source manifest uses the `0.0.0-development` placeholder and `private: true` to prevent accidental publication; release versions are injected from `cli-v*` tags. It includes one active server/account login, OAuth Device Code authentication with refresh and revocation, an explicit personal-access-token fallback, a default project, structured input and output, OpenAPI-generated business commands, dedicated protocol commands, command discovery, and release validation. Live coverage totals and ratios come only from `pnpm check:platform-cli-coverage`.

`cli/src/entry.ts` is the shared npm and Bun entry point, and workspace packages are bundled safely into the distribution. Prereleases are available on npm and pass npm/pnpm global-install, localized Help, machine Help, and supported standalone-binary smoke tests.

Canonical OpenAPI commands automatically negotiate the API generation, minimum
CLI version, and OpenAPI digest through `/api/v1/meta`; `luna doctor` exposes
the detailed diagnostics. Hidden browser callbacks, webhooks, internal receivers,
and low-level protocol operations are not registered as canonical raw commands;
SSE, downloads, and terminals are exposed only through their dedicated protocol
commands. High- and critical-risk operations require an explicit interactive
confirmation, or `--yes` in non-interactive and agent mode. CLI confirmation
records caller intent only: server permissions, scopes, and step-up MFA remain
authoritative. Terminal and data-export protocols require a CLI OAuth login and
a valid step-up assertion for the matching purpose; personal access tokens cannot
satisfy or bypass that authorization. Generic `api request` remains a human-only
diagnostic escape hatch.

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
luna login
luna login server=https://devops.example.com
printf '%s' "$LUNA_TOKEN" | luna login mode=access-token token=@-
luna whoami
luna doctor
luna logout
luna project --help
luna project get-projects --help
```

The four root shortcuts reuse the canonical `auth login`, `auth status`,
`health doctor`, and `auth logout` handlers. Scripts and agents must use the
canonical two-level paths; strict Agent mode rejects root aliases.

Running `luna` without a subcommand displays the same localized root Help and
does not perform a remote operation. A bare `luna login` always targets the
official `https://devops.liteyuki.org` instance. Pass `server=https://...` to
log in elsewhere; a new login replaces the locally active server, credential,
and default project. There is no context-switching layer. Locale precedence is
`--lang`, `LUNA_LANG`, configured `language`, system locale, then English. Use
`LUNA_LANG=zh-CN luna --help` for Chinese. npm `latest` and `beta` are separate
update channels, so prerelease testing must explicitly install `@beta`. Skills
build on the CLI's machine-readable Help for more precise agent operation; the
CLI does not depend on Skills. Skills use the exact same version and ship in the
same `cli-v*` GitHub Release as the CLI.
