# 发布与制品验证

Luna DevOps 与 Luna CLI 现在由两个独立仓库发布：

| 产品 | 仓库 | Git tag | 发布渠道 |
| --- | --- | --- | --- |
| Luna DevOps | `LiteyukiStudio/luna-devops` | `v1.2.3` | 容器镜像与平台 GitHub Release |
| Luna CLI + Skill | `LiteyukiStudio/luna-cli` | `v1.2.3` | npm、二进制、单一 `.skill` 与 CLI GitHub Release |

两个仓库都使用标准 `v*` tag，但工作流和 Release 位于各自仓库，不会互相触发。
CLI 与 Skill 必须使用同一版本、tag、commit 和 GitHub Release；CLI 可以单独运行，
Skill 则精确依赖配套 CLI 版本。

## 版本来源

CLI 源码中的 `package.json.version` 固定为 `0.0.0-development`，并使用
`private: true` 防止从工作区误发布。发布版本只来自 CLI 仓库的 `v*` tag。
工作流会在临时 npm 打包目录中写入版本、移除 `private`，并把相同版本注入
JavaScript 制品和 Bun 二进制，不需要为发布版本提交一次清单修改。

预发布后缀决定 npm dist-tag：

| tag 示例 | npm dist-tag | GitHub Release |
| --- | --- | --- |
| `v1.2.3` | `latest` | Release |
| `v1.2.3-rc.1` | `next` | Prerelease |
| `v1.2.3-beta.1` | `beta` | Prerelease |

## CI 门禁

CLI 仓库的常规 CI 会：

1. 安装锁定依赖并执行 TypeScript、ESLint、单元测试和构建。
2. 校验 OpenAPI 契约副本和配套 Skill。
3. 只读检出 Luna DevOps 平台源码。
4. 对比 Gin Router、平台 OpenAPI、CLI 机器目录和精确协议分类。
5. 要求普通业务命令覆盖率为 100%。
6. 生成真实 npm tarball，并用 npm 与 pnpm 做全局安装 smoke。

CLI 仓库只读取平台源码，不推送平台仓库，也不要求两个仓库共享提交历史。

## CLI 与 Skill 配套发版

CLI 仓库的 `v*` tag 触发 `cli-release.yml`。同一工作流构建 CLI，同时执行
Skill 结构校验、命令同步检查和确定性打包，并在同一个 GitHub Release 发布：

- `luna-devops-<version>.skill`；
- `LUNA-CLI-SKILLS-MANIFEST.json`；
- npm tarball 与受支持平台的独立二进制；
- `SHA256SUMS`、Release manifest、SBOM 和 provenance。

`.skill` 本质上是 ZIP 文件，内部只有一个 `luna-devops` 根目录。Agent 先读取根
`SKILL.md` 路由任务，再按需加载 `references/`，不会一次注入所有领域资料。

所有新 CLI 与 Skill 制品都从
[Luna CLI Releases](https://github.com/LiteyukiStudio/luna-cli/releases)
下载。`0.0.12` 及更早版本仍保留在平台旧仓库中，历史链接不迁移。

## npm Trusted Publishing

npm 发布使用 GitHub OIDC Trusted Publishing，不保存长期写入 Token。npm 包
设置应绑定：

- Organization or user：`LiteyukiStudio`
- Repository：`luna-cli`
- Workflow：`cli-release.yml`
- Environment：`npm`

发布 Job 只授予必要的 `id-token: write`。如果同一版本已经存在，工作流比较
npm integrity：内容一致时跳过，内容不一致时失败并要求发布新版本。

## 平台契约来源

平台 OpenAPI 仍由 `LiteyukiStudio/luna-devops` 维护。CLI 发布前的覆盖门禁通过
只读 checkout 获取指定平台 revision，并将 revision 与 OpenAPI digest 写入制品
元数据。这能让两个产品独立发布，同时保留可追溯的兼容性依据。

## 制品边界

- npm/pnpm + Node.js 是所有受支持系统的通用安装方式。
- Linux glibc x64/arm64 提供经过 smoke 的独立二进制。
- macOS 未完成 Developer ID 签名和公证前，只提供明确标记的测试制品。
- Windows 与 Alpine/musl 使用 npm/pnpm + Node.js 降级渠道。

下载独立二进制后至少校验 `SHA256SUMS`，并在 GitHub Attestations 中确认制品来自
`LiteyukiStudio/luna-cli` 对应 tag 和发布工作流。
