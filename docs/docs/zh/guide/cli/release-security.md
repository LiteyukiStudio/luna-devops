# 发布与制品验证

Luna DevOps、Luna CLI 与 Luna CLI Skills 使用独立的版本和 tag 命名空间：

| 产品 | Git tag | 发布渠道 |
| --- | --- | --- |
| Luna DevOps | `v1.2.3` | 容器镜像与 GitHub Release |
| Luna CLI | `cli-v1.2.3` | npm `latest`、二进制与 GitHub Release |
| Luna CLI | `cli-v1.2.3-rc.1` | npm `next` 与 GitHub Prerelease |
| Luna CLI | `cli-v1.2.3-beta.1` | npm `beta` 与 GitHub Prerelease |
| Luna CLI Skills | `cli-skills-v1.2.3` | `.skill`、整套 ZIP 与 GitHub Release |
| Luna CLI Skills | `cli-skills-v1.2.3-beta.1` | `.skill`、整套 ZIP 与 GitHub Prerelease |

三个前缀由不同工作流消费，互不触发。版本关系在
`release-compatibility.json` 中维护：Skills 必须声明可用的 CLI SemVer
范围；CLI 只给出建议配套的 Skills 版本，不会因为缺少 Skills 而拒绝运行。

仓库中的 `cli/package.json.version` 固定为 `0.0.0-development`，只表示源码开发态，并通过 `private: true` 阻止从源码目录误发布。发布版本只来自 `cli-v*` tag：工作流校验 tag 中的 SemVer，在临时 npm 打包目录移除 `private` 标记、写入版本，并把同一版本注入 npm JavaScript 制品和 Bun 二进制，因此发版前不需要手动修改或提交 `package.json`。发布阶段直接读取待发布 tarball 内的 `package/package.json` 校验包名、版本和 `private` 状态，不使用源码占位版本判断制品版本。

## 首次创建 npm 包

npm 不需要提前创建一个空包。首次执行下面的发布命令时，会创建 public scoped package `@liteyuki/luna-cli`：

```bash
npm publish <已验证的-tarball.tgz> --access public --tag next
```

首次发布前需要确认：

1. npm 组织 `@liteyuki` 已存在，当前维护者拥有创建 public package 的权限；
2. 维护者已启用 2FA，并在干净环境中使用仓库脚本完成构建、打包和 smoke test；
3. 首次发布使用预发布版本和 `next` 标签，不直接占用稳定版；
4. 首次发布成功后，再在包设置中配置 GitHub Actions Trusted Publisher；
5. 配置完成后创建一个尚未发布版本的 `cli-v*` tag 验证 OIDC 发布。复用首次发布的 tag 版本只会触发幂等校验，不会真正执行 `npm publish`。

## CI 门禁

CLI 相关变更会执行：

1. 使用锁文件安装 pnpm 工作区依赖。
2. 重新生成 API 契约并检查 drift。
3. 读取机器 Help，校验配套 Skills 引用的命令、Agent 参数和能力边界。
4. TypeScript typecheck、ESLint、单元测试和构建。
5. 生成真实 npm tarball，并检查文件白名单。
6. 在干净临时目录中分别使用 npm 与 pnpm 全局安装同一个 tarball。
7. 构建当前 Linux host 的 Bun baseline 二进制并运行 smoke test。

发布门禁还会断言 tarball 中的 `package.json.version`、npm 安装后的 `luna --version` 和独立二进制的 `luna --version` 都等于 tag 版本。

## Luna CLI Skills 发版

`cli-skills-v*` tag 触发 `cli-skills-release.yml`。该工作流读取 tag 版本和
`release-compatibility.json` 中的 `cliSkills.requiresCli`，执行 Skill
结构校验与 CLI 命令同步检查，然后发布：

- 每个 Skill 一个 `<skill-name>-<version>.skill`；
- 一个 `luna-cli-skills-<version>.zip` 整套压缩包；
- `LUNA-CLI-SKILLS-MANIFEST.json`，包含必需 CLI 版本范围和各制品 SHA-256；
- `SHA256SUMS` 与 GitHub OIDC build provenance。

`.skill` 是 ZIP 格式，每个文件只包含一个与 Skill `name` 相同的根目录和其中的
`SKILL.md`、脚本与引用资料。打包脚本固定文件顺序和时间戳，同一 tag
重跑会得到相同哈希。所有制品都从
[GitHub Releases](https://github.com/LiteyukiStudio/luna-devops/releases)
下载，不从文档站复制一份。

## 更新日志同步

三条 Release 工作流成功后会触发 `changelog-sync.yml`。它从不可变 tag
重新生成中英文 Luna DevOps、Luna CLI 和 Luna CLI Skills 更新日志，并创建
文档 PR。这样既能保持主分支保护，也避免发布任务直接改写开发者分支。
仓库需要在 Actions 设置中允许工作流创建 Pull Request；如果组织策略禁止该
权限，发布制品仍会完成，但需要维护者手动运行生成脚本并提交更新日志。

发布工作流在这些门禁之外，还会构建明确的平台矩阵：

- Linux x64 baseline；
- Linux arm64；
- macOS arm64 和 x64 预发布测试制品。

Windows 与 Alpine/musl 不进入独立二进制矩阵，统一通过 npm 或 pnpm 安装，在 Node.js `22.14.0` 或更高版本上运行。这样发布门禁只承诺能够在目标 runner 真实执行的制品，不依赖构建阶段临时下载 Bun 目标运行时，也不要求用户补装 musl 平台的额外动态库。

## 签名边界

当前仓库没有 Apple Developer ID 和公证凭据。工作流不会假装 macOS 制品已经签名：

- 正式版只包含经过目标环境 smoke 的 Linux 独立二进制；
- 预发布版可以包含名称带 `-unsigned` 的 macOS 测试制品；
- 未签名 macOS 制品不应进入生产环境；
- 接入对应签名和验证阶段后，才会扩展稳定制品矩阵。

## npm Trusted Publishing

npm 发布使用 GitHub OIDC Trusted Publishing，不保存长期写入 Token。维护者需要在 npm 包设置中绑定：

- Organization or user：`LiteyukiStudio`
- Repository：`luna-devops`
- Workflow：`cli-release.yml`
- Environment：`npm`

GitHub `npm` Environment 应配置保护规则和维护者审批。发布 Job 只授予 `id-token: write`，不设置 `NPM_TOKEN` 或 `NODE_AUTH_TOKEN`。

如果同一版本已经存在，工作流会比较 npm `dist.integrity`：

- 内容一致：跳过 npm 发布，继续补齐 GitHub Release；
- 内容不同：立即失败，必须发布新版本。

## 校验下载

每个 Release 生成：

- `SHA256SUMS`；
- `RELEASE-MANIFEST.json`；
- SPDX JSON SBOM；
- GitHub OIDC build provenance；
- SBOM attestation bundle。

下载后至少校验 SHA-256：

```bash
grep " luna-linux-x64$" SHA256SUMS | sha256sum -c -
```

还应在 GitHub Release 的 Attestations 页面确认制品来自
`LiteyukiStudio/luna-devops` 的对应发布工作流，并检查 tag、commit
和制品名称是否与预期一致。
