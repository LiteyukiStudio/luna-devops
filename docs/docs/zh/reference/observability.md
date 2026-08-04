# 接入可观测平台

Luna DevOps 通过 OpenTelemetry 发送链路、指标和结构化日志。平台不绑定特定后端；使用前准备支持 OTLP HTTP 的 OpenTelemetry Collector。

## 最小配置

为 API、Worker 和 Agent 配置同一个地址：

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
```

重启后，三个服务分别以 `luna-devops-api`、`luna-worker` 和 `luna-agent` 上报。变量留空时关闭导出；Collector 暂时不可用不会阻止业务请求。

需要标记环境或集群时可以增加：

```bash
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production,k8s.cluster.name=main
```

Collector 需要鉴权时，通过 Secret 注入 `OTEL_EXPORTER_OTLP_HEADERS`，不要把凭据写入公开配置文件。

## Agent 全内容观测（高敏）

默认遥测不会记录用户提示词、模型回复或工具正文。仅在排查模型行为且已限制 Tempo/Loki 访问权限和保留周期时，临时为 Agent 开启：

```bash
AI_OBSERVABILITY_CAPTURE_CONTENT=true
```

重启 Agent 后，Trace 和日志会包含经过脱敏的模型输入输出及工具调用内容。平台会过滤常见 Token、Cookie、密码和 API Key，但提示词与业务返回仍可能包含个人或平台数据，因此该功能不能替代数据隔离。

排障结束后，将开关恢复为 `false` 并重启 Agent。不要把高敏内容观测作为常驻审计日志。

## 配置 Luna 内嵌 Agent 观测

如需在 Luna DevOps 运营面板中查看 Agent 观测数据，平台管理员进入“全局设置 → AI 助手 → AI 高级设置”，填写 Prometheus、Loki 和 Tempo 的查询根地址，然后开启“启用 Agent 可观测”。这些查询地址与用于上报的 `OTEL_EXPORTER_OTLP_ENDPOINT` 不同。

- Prometheus：Agent 指标查询地址，例如 `http://prometheus:9090`。
- Loki：结构化日志查询地址，例如 `http://loki:3100`。
- Tempo：Trace 查询地址，例如 `http://tempo:3200`。
- 使用多租户或 Bearer Token 时，填写对应 Tenant ID 和令牌。

三个查询地址均配置后才能启用。令牌会加密保存且不回显。数据源应只允许 Luna API 所在网络访问，不要直接暴露给浏览器。

每个数据源旁都有独立的“测试连接”按钮。测试使用当前表单中的地址、Tenant ID 和新输入的令牌；令牌留空时复用已保存值。结果会区分“连接正常且有数据”和“连接正常但最近一小时无数据”。测试失败不会阻止保存，便于先保存尚未开放网络的部署配置；启用后的 Agent 观测页会如实将不可达数据源标记为“不可用”。

配置完成后进入“运营面板 → Agent 观测”，可以查看 Run 成功率、活跃 Run、首 Token P95、模型与依赖错误率、Token 吞吐、工具失败趋势、最近 Agent Run 和 Trace 关联失败日志。调用链工作区支持“按会话”和“按轮次”两种视角：平台管理员可以跨用户检索会话，在会话详情中查看标题、所属用户、用户消息、Agent 回复、各轮状态和关联 Trace；按轮次视角直接展示最近 Trace。每轮用户消息通常产生一条 Trace，同一会话的多轮 Trace 通过 Conversation ID 关联。

打开任一轮的“查看调用链”，可以使用默认完整展开的内嵌瀑布视图检查 Span 父子关系、并行阶段、耗时占比、错误节点，以及模型、工具、HTTP 和数据库调用；Span 树支持逐节点或整体折叠与展开。会话消息从 Luna 的权威会话存储读取，不写入 Tempo、Loki 或 Prometheus；Tempo 属性侧栏仍只展示经过白名单筛选的模型、Token、工具和错误属性，不展示 Prompt 或工具敏感参数。跨用户会话与消息仅平台管理员可见，详情读取会写入审计日志。

浏览器只调用 Luna API 提供的固定查询，不能提交任意 PromQL、LogQL 或 TraceQL，也不会收到数据源地址和凭据。Trace 详情由 Luna API 通过 Tempo 2.x 查询接口读取并归一化，同时兼容旧代理返回的 OTLP JSON 结构；Tempo 数据保留期结束后，对应详情将无法继续打开。

Prometheus 暂无 Agent 指标或查询返回非有限计算结果时，对应指标会显示为零或空趋势，不会阻止 Loki 日志与 Tempo 链路继续展示。

## 导入 Grafana 仪表盘

仓库提供两套可直接导入的仪表盘：

- `grafana/dashboards/luna-devops-overview.json`：平台、API、Worker、交付链路、Agent 和数据库概览。
- `grafana/dashboards/luna-agent-llm-observability.json`：Agent Run、模型延迟、Token、工具、日志和 Trace。

导入时分别选择已有的 Prometheus、Tempo 和 Loki 数据源。建议先查看成功率、错误率和延迟，再按会话、轮次或 Run ID 定位 Trace；只有确实需要原始模型行为时才临时开启高敏内容观测。

## 查询 Trace

一次用户消息通常对应一条 Trace。常用关联字段如下：

| 范围 | 属性 |
| --- | --- |
| AI 会话 | `gen_ai.conversation.id` |
| 对话轮次 | `luna.turn.id` |
| Agent 执行 | `luna.run.id` |
| 工具调用 | `luna.tool_call.id` |

例如在 Tempo 中查询一次 Run：

```text
{ span.luna.run.id = "airun_xxx" }
```

具体查询语法以所用 Tempo/Grafana 版本为准。

## 生产建议

- Collector 和观测后端应位于受控网络，跨网络上报时启用 TLS 和认证。
- 在 Collector 统一配置批处理、内存限制和采样；优先保留错误和慢链路。
- 为日志、Trace 和高敏 Agent 内容设置最小访问权限与合理保留周期。
- API 可以提供 Prometheus 兼容抓取入口；完整平台指标建议统一通过 OTLP 汇入后端。
- 健康检查成功请求可能不会生成 Trace。用户主动发起的 Provider、集群和镜像站测试仍会保留观测数据。

Collector 的部署与导出配置参见 [OpenTelemetry Collector 文档](https://opentelemetry.io/docs/collector/deploy/)。
