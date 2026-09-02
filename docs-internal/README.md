# Luna DevOps 内部开发文档

本目录只保存会持续指导开发的内部边界、仍在推进的跨模块方案和短期操作手册。面向用户与部署
管理员的内容放在 `docs/docs/{zh,en}`；实现状态放在 `TODO.md`；接口和字段以代码、OpenAPI 与测试
为事实源。

已完成方案、旧原型和一次性验收记录不在本目录长期归档，可从 Git 历史追溯。不要重新建立
`legacy/`，也不要复制工作流、脚本、OpenAPI 或 CLI 帮助中已经可以自动核对的事实。

## 核心文档

| 文档 | 用途 |
| --- | --- |
| [产品概要.md](产品概要.md) | 产品定位、平台边界、运行观察与计费口径 |
| [代码检查流程.md](代码检查流程.md) | 健康检查、重构触发条件、CI 与发布门禁原则 |
| [11-AI助手与Agent规格.md](11-AI助手与Agent规格.md) | Agent 架构、工具目录、审批、计费与安全边界 |
| [12-AI声明式交互卡片Schema.md](12-AI声明式交互卡片Schema.md) | Agent 交互卡片协议契约 |
| [可观测和插桩规范.md](可观测和插桩规范.md) | Trace、日志、Metric、Context 与遥测安全规范 |

## 其他入口

- AI 编码代理约束：[`AGENTS.md`](../AGENTS.md)
- 人类贡献者入口：[`CONTRIBUTING.md`](../CONTRIBUTING.md)
- 计划与验收状态：[`TODO.md`](../TODO.md)
- Luna CLI 独立仓库：[LiteyukiStudio/luna-cli](https://github.com/LiteyukiStudio/luna-cli)
