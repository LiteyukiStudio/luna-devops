# Luna DevOps 内部开发文档

本目录只保存会持续指导开发的内部边界、仍在推进的跨模块方案和短期操作手册。面向用户与部署
管理员的内容放在 `docs/docs/{zh,en}`；实现状态放在 `TODO.md`；接口和字段以代码、OpenAPI 与测试
为事实源。

已完成方案、旧原型和一次性验收记录不在本目录长期归档，可从 Git 历史追溯。不要重新建立
`legacy/`，也不要复制工作流、脚本、OpenAPI 或 CLI 帮助中已经可以自动核对的事实。

## 核心文档

| 文档 | 用途 |
| --- | --- |
| [01-产品与一体化方案.md](01-产品与一体化方案.md) | 产品定位、平台边界、运行观察与计费口径 |
| [07-代码健康检查SOP.md](07-代码健康检查SOP.md) | 健康检查、重构触发条件、CI 与发布门禁原则 |
| [11-AI助手与Agent规格.md](11-AI助手与Agent规格.md) | Agent 架构、工具目录、审批、计费与安全边界 |
| [12-AI声明式交互卡片Schema.md](12-AI声明式交互卡片Schema.md) | Agent 交互卡片协议契约 |
| [14-可观测插桩与验收标准.md](14-可观测插桩与验收标准.md) | Trace、日志、Metric、Context 与遥测安全规范 |
| [15-站内消息盒子协议.md](15-站内消息盒子协议.md) | 消息投影、用户状态与操作请求的稳定不变量 |
| [场景测试.md](场景测试.md) | 端到端用户场景、Agent 工具链和权威回读验收 |

## 进行中方案

方案只在存在未完成的跨文件事项时保留；完成后提取仍有效的规则并删除正文。

- [16-application-deployments-panel重构方案.md](16-application-deployments-panel重构方案.md) — 部署面板的行为保持型拆分。
- [17-项目空间数据卷中心与数据迁移方案.md](17-项目空间数据卷中心与数据迁移方案.md) — 数据卷主链完成后的 legacy Contract 清理。

## 临时操作手册

- [20-运行时环境明文审计.md](20-运行时环境明文审计.md) — 结构化环境变量契约启用前的只读审计与人工处置；对应事项完成后删除。

## 其他入口

- AI 编码代理约束：[`AGENTS.md`](../AGENTS.md)
- 人类贡献者入口：[`CONTRIBUTING.md`](../CONTRIBUTING.md)
- 计划与验收状态：[`TODO.md`](../TODO.md)
- Luna CLI 独立仓库：[LiteyukiStudio/luna-cli](https://github.com/LiteyukiStudio/luna-cli)
