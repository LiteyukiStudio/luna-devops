# Luna CLI 规格迁移说明

Luna CLI 已从 Luna DevOps monorepo 分离到独立仓库：

- 源码：<https://github.com/LiteyukiStudio/luna-cli>
- 规范：<https://github.com/LiteyukiStudio/luna-cli/blob/main/docs/cli-spec.md>
- Release：<https://github.com/LiteyukiStudio/luna-cli/releases>
- 配套 Skill：<https://github.com/LiteyukiStudio/luna-cli/tree/main/skills/luna-devops>

独立仓库是 CLI 源码、API Client、OpenAPI 契约副本、配套 Skill、测试和发布流程的唯一事实来源。本文件只保留迁移入口，避免平台仓库与 CLI 仓库维护两份会漂移的规格。

平台仓库继续维护后端 OpenAPI，并由 CLI 仓库的只读 CI 拉取平台源码执行契约漂移和命令覆盖检查。两个仓库没有 submodule、subtree 或 subrepo 关联。

## 本地联调

平台仓库根目录的 `/cli/` 已加入 `.gitignore`，可按需克隆独立仓库：

```bash
git clone git@github.com:LiteyukiStudio/luna-cli.git cli
pnpm --dir cli install
pnpm --dir cli sync:openapi
LUNA_PLATFORM_ROOT=.. pnpm --dir cli check:platform-coverage
```

该目录仅用于本地联调，不属于平台 pnpm workspace，也不会进入平台提交或发布制品。
