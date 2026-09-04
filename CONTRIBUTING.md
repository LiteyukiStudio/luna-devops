# 参与贡献

感谢关注 Luna DevOps。人类贡献者从本页进入；AI 编码代理直接阅读 [工程约束](AGENTS.md)。

## 开始

1. 阅读 [仓库 README](README.md) 完成本地环境准备。
2. 阅读 [内部文档索引](docs-internal/README.md) 和任务涉及的现行规范。
3. 检查工作树，沿真实调用链修改，并保留已有未提交改动。

工程、安全、文档和验证规则以 [`AGENTS.md`](AGENTS.md) 为准；前端特有规则见
[`web/AGENTS.md`](web/AGENTS.md)，代码检查方法见
[`docs-internal/代码检查流程.md`](docs-internal/代码检查流程.md)。不要在本页复制这些规则。

## 提交前

- 为行为变化补充相称的测试，并同步受影响的 OpenAPI、迁移、前端类型、Agent/Worker 契约与中英文公开文档。
- 运行相关技术栈门禁；具体命令以项目脚本和 CI 为准。
- 不提交环境文件、构建产物、日志或敏感信息。
- 提交信息使用 `<type> <gitmoji>: <summary>`，例如 `fix 🐛: 修复构建状态回读`。

## 文档归属

| 内容 | 位置 |
| --- | --- |
| 用户与部署文档 | `docs/docs/{zh,en}` |
| 长期内部规范与现行设计 | `docs-internal/` |
| 未完成计划 | `TODO.md` |
| API、CLI、版本事实 | OpenAPI、CLI help、GitHub Release |

完成历史由测试、提交、Issue 和 PR 保存，不在规范或 TODO 中重复归档。

## 许可证

贡献即表示同意以 [MIT License](LICENSE) 发布改动；第三方依赖与品牌资源遵循各自许可。
