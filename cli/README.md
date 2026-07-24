# Luna CLI

Luna CLI 是 Luna DevOps 的命令行客户端，面向终端用户和自动化 Agent。命令采用固定的两级结构：

```text
luna <工具分类> <具体工具> key=value
```

English documentation follows the Chinese section.

## 当前状态

CLI 目前处于 `0.1.0` 开发阶段。仓库中已经实现：

- 多实例和项目空间上下文的数据模型、解析与本地存储；
- Access Token 登录、校验和本地凭据存储基础能力；
- `key=value`、JSON、文件和标准输入参数解析；
- 人类可读输出与稳定 JSON Envelope；
- 本地命令注册、帮助目录、Shell Completion 和 OpenAPI 命令注册器；
- 从 OpenAPI 生成并注册全部 110 个已登记操作；
- npm 包、Bun 独立二进制的 CI、打包、安装 smoke 与发布门禁。

`cli/src/entry.ts` 已作为 npm 与 Bun 二进制的统一入口，共享契约和客户端会被安全打包进发布产物。本地已经通过 npm/pnpm 全局安装和 Bun 独立二进制 smoke。项目尚未完成首次公开发布，因此 npm 安装命令要等 `cli-v*` 版本正式发布后才可使用。

当前明确未完成的能力包括：OpenAPI 尚未覆盖的后端公开路由、服务端能力协商客户端、Authorization Code + PKCE、Device Code、Bearer Step-up MFA、SSE/WebSocket/下载协议适配和中高风险服务端执行计划。高风险 API 在计划协议完成前会 fail closed，不会被 `yes=true` 绕过。

## 计划中的安装方式

首次正式发布后，可以通过 npm 或 pnpm 安装：

```bash
npm install --global @liteyuki/luna-cli
pnpm add --global @liteyuki/luna-cli
```

也可以从 GitHub Release 下载独立二进制。稳定版当前只计划发布经过目标环境 smoke test 的 Linux 制品；macOS 和 Windows 在接入代码签名与公证之前，只会在预发布版本提供名称带 `-unsigned` 的测试制品。

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

`cli-v*` tag 只用于 Luna CLI 发布，不会触发平台的 `v*` 发布流程。

---

## English

Luna CLI is the command-line client for Luna DevOps, designed for both people and automation agents:

```text
luna <category> <tool> key=value
```

### Current status

The CLI is under active `0.1.0` development. It includes contexts, Access Token authentication, structured input and output, command discovery, all 110 operations currently documented by OpenAPI, and release validation.

`cli/src/entry.ts` is the shared npm and Bun entry point, and workspace packages are bundled safely into the distribution. Local npm/pnpm global-install and standalone-binary smoke tests pass. The first public package has not been published yet.

Undocumented server routes, capability negotiation in the client, Authorization Code + PKCE, Device Code, Bearer step-up MFA, streaming transports, downloads, and server-issued plans remain release work. High-risk API operations fail closed until the server-plan protocol exists.

### Planned installation

After the first public release:

```bash
npm install --global @liteyuki/luna-cli
pnpm add --global @liteyuki/luna-cli
```

Standalone binaries will also be attached to GitHub Releases. Stable releases currently include only Linux binaries that pass target-environment smoke tests. Until Apple and Windows signing are configured, desktop binaries are available only on prereleases and are explicitly suffixed with `-unsigned`.

See the documentation links above for installation, release channels, checksums, SBOMs, provenance, and current limitations.
