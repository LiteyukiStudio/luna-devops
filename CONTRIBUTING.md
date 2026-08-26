# 参与贡献

感谢关注 Luna DevOps。本文件是面向人类贡献者的协作入口；AI 编码代理请直接阅读 [`AGENTS.md`](AGENTS.md)。两者共同遵循 [`docs-internal/`](docs-internal/README.md) 中的内部开发文档。

## 开工前

1. 阅读 [`README.md`](README.md) 了解项目结构、技术栈与本地运行方式。
2. 阅读 [`docs-internal/README.md`](docs-internal/README.md)，了解当前核心内部文档及其归属。
3. 涉及的功能若有对应内部规范或方案，先在 [`docs-internal/`](docs-internal/README.md) 中检索，避免与既定决策冲突。

## 开发约束（摘要）

以下约束的完整权威版本见 `AGENTS.md` 与 `docs-internal/`，此处只列最常遇到的几条：

- **端到端一致性**：改动涉及前端、API、Worker、Agent 多层时，各层 Schema、错误码、权限、可观测字段必须在同一事项内同步，不允许只改一层。
- **可观测**：新功能按 [`docs-internal/可观测和插桩规范.md`](docs-internal/可观测和插桩规范.md) 补齐 Trace、结构化日志与低基数 Metrics。
- **i18n**：所有用户可见前端文案走 i18n，不硬编码。
- **工具与依赖**：前端统一 `pnpm`，Python 统一 `uv`；`web/`、`docs/`、`luna-agent/`、`tests/` 各自维护依赖清单，不用根 workspace。React、CodeMirror 等依赖对象身份判断的运行时库必须保持单一兼容版本，依赖变更后执行 `pnpm --dir web check:singletons`。
- **Secret**：不落明文、不回显、不进日志与遥测。

## 提交

- 提交信息使用 gitmoji + Conventional Commits：`<type> <gitmoji>: <summary>`，例如 `feat ✨: 新增项目空间管理页面`。完整规则见 `AGENTS.md` 的「Git 提交消息」一节。
- 功能或行为变化时，同步更新公开文档站（`docs/docs/{zh,en}`）；影响计划、验收或状态时更新 [`TODO.md`](TODO.md)。
- 提交前至少通过相关范围的检查：

```bash
go test ./...
pnpm --dir web lint && pnpm --dir web build
pnpm --dir luna-agent lint && pnpm --dir luna-agent test   # 涉及 Agent 时
pnpm --dir docs build                                       # 涉及文档站时
```

## 文档归属速查

| 内容类型 | 位置 |
| --- | --- |
| 用户/部署管理员公开文档 | `docs/docs/{zh,en}` |
| 长期开发规范（可观测、SOP、协议契约） | `docs-internal/`（见 [长期规范](docs-internal/README.md)） |
| 稳定产品/仓库边界与仍在进行的跨文件方案 | `docs-internal/`（见 [内部索引](docs-internal/README.md)） |
| 已完成方案、复盘、审计和问题记录 | `TODO.md`、Issue、PR 与 Git 历史，不在仓库文档重复归档 |
| AI 代理硬约束 | `AGENTS.md` |
| 计划与状态 | `TODO.md` |

## 许可证

贡献即表示你同意以 [MIT License](LICENSE) 发布你的改动。第三方依赖与品牌资源仍受其自身条款约束。
