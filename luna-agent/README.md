# luna-agent

Luna DevOps 的 AI 助手运行时。它从 Luna API 获取模型与工具目录配置，执行模型调用、工具审批和交互卡片流程，并将 Run、Turn、事件及 ToolCall 持久化到 PostgreSQL。

## 运行边界

- PostgreSQL 是会话与执行状态的唯一事实源；生产部署只运行一个 Agent 副本。
- Agent 使用固定服务身份调用 Luna API，最终权限仍由 API 按当前用户和项目空间判断。
- 平台工具来自 OpenAPI 目录；模型先用 `search_tools` 检索，再用 `get_tool_details` 加载完整契约。
- 高危操作必须逐次审批，敏感表单内容不进入模型上下文。
- AI 费用只计入发起用户的个人钱包。

详细设计以 [`docs-internal/11-AI助手与Agent规格.md`](../docs-internal/11-AI助手与Agent规格.md) 和 [`docs-internal/12-AI声明式交互卡片Schema.md`](../docs-internal/12-AI声明式交互卡片Schema.md) 为准。

## 本地启动

先按仓库根目录 `.env.example` 配置 PostgreSQL、`AI_INTERNAL_SECRET` 与 API 连接，再执行：

```bash
pnpm --dir luna-agent install
pnpm --dir luna-agent dev
```

进程默认读取仓库根目录 `.env`，并允许 `luna-agent/.env.local` 覆盖本地值。

## 验证

```bash
pnpm --dir luna-agent lint
pnpm --dir luna-agent typecheck
pnpm --dir luna-agent test
pnpm --dir luna-agent build
```

开发约束继承仓库根 [`AGENTS.md`](../AGENTS.md)。Prompt、工具描述和模型任务说明使用中文；稳定协议字段、错误码及工具名保持原值。
