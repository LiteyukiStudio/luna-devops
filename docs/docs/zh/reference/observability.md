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

内容观测默认关闭。仅在受控的开发、测试或安全审计环境中确有需要时，临时为 Agent 开启：

```bash
AI_OBSERVABILITY_CAPTURE_CONTENT=true
```

重启 Agent 后，Trace 和日志可能记录模型输入消息、模型输出消息、工具调用参数、工具执行结果，以及模型或工具返回的错误。这些内容即使经过脱敏，仍可能包含用户输入、资源名称、配置内容或诊断信息，因此该功能不能替代数据隔离。

生产环境应保持关闭。只有管理员已评估数据访问权限、保留周期和敏感信息风险时才可临时开启；任务结束后，将开关恢复为 `false` 并重启 Agent。不要把高敏内容观测作为常驻审计日志。

## 配置 Luna 内嵌 Agent 观测

如需在 Luna DevOps 运营面板中查看 Agent 观测数据，平台管理员进入“全局设置 → AI 助手 → AI 高级设置”，填写 Prometheus、Loki 和 Tempo 的查询根地址，然后开启“启用 Agent 可观测”。这些查询地址与用于上报的 `OTEL_EXPORTER_OTLP_ENDPOINT` 不同。

- Prometheus：Agent 指标查询地址，例如 `http://prometheus:9090`。
- Loki：结构化日志查询地址，例如 `http://loki:3100`。
- Tempo：Trace 查询地址，例如 `http://tempo:3200`。
- 使用多租户或 Bearer Token 时，填写对应 Tenant ID 和令牌。

三个查询地址均配置后才能启用。令牌会加密保存且不回显。数据源应只允许 Luna API 所在网络访问，不要直接暴露给浏览器。

每个数据源旁都有独立的“测试连接”按钮。测试使用当前表单中的地址、Tenant ID 和新输入的令牌；令牌留空时复用已保存值。结果会区分“连接正常且有数据”和“连接正常但最近一小时无数据”。测试失败不会阻止保存，便于先保存尚未开放网络的部署配置；页面在实际查询某个数据源失败时会显示对应指标或详情暂不可用。

配置完成后进入“运营面板 → Agent 观测”。页面顶部可选择 1 小时、6 小时、24 小时、7 日、30 日或 1 年周期，并汇总输入 Token、输出 Token、工具调用、对话轮数量、终态轮次成功率和执行耗时 P95；浏览器会按当前用户记住上一次选择的周期，重新进入或刷新后继续使用。下方按“从一次用户输入到 Agent 输出结束”为一轮，提供跨用户搜索和服务端分页。点击任意轮次会从右侧打开详情，不再提供会话级观测视图。

轮次详情按开始时间把真实 Span 呈现为纵向步骤时间轴，并支持筛选全部、模型、工具或异常步骤。默认只保留用户消息、Agent、模型和工具步骤；一次工具调用以 `luna-agent` 服务的 `agent.tool.execute`（内部工具为 `agent.tool.internal`）作为权威业务步骤，其 `luna_api.tool.execute` 网络传输子 Span 不重复展示。开启“展示外部服务”后才显示外部 HTTP、数据库等基础设施 Span。模型输入输出按消息角色和 Markdown 呈现，工具入参与返回使用格式化、语法高亮的 JSON，常用 Span 属性显示为本地化业务标签。展开“原始 Span JSON”可查看 Tempo 返回的完整原始数据。使用标题栏的“复制诊断 JSON”或“下载诊断 JSON”，可导出本轮元数据以及按开始时间排序的全部 Span；导出不受当前时间轴筛选或“展示外部服务”开关影响，并同时保留标准化字段、内容事件和 Tempo 原始 Span，便于离线诊断 Agent Harness。

开启 `AI_OBSERVABILITY_CAPTURE_CONTENT` 后，新产生的模型步骤会展示 System Prompt、用户消息及模型输出，工具步骤会展示执行状态、调用入参和返回结果；这些内容来自对应 Span Event。关闭内容观测时，面板仍展示步骤元数据与原始 JSON，但不会补录此前未采集的模型或工具内容。用户消息与最终 Agent 回复同时从 Luna 的权威会话存储读取。跨用户轮次与内容仅平台管理员可见，Trace 详情读取会写入审计日志。

浏览器只调用 Luna API 提供的固定查询，不能提交任意 PromQL、LogQL 或 TraceQL，也不会收到数据源地址和凭据。Trace 详情由 Luna API 通过 Tempo 2.x 查询接口读取并归一化，同时兼容旧代理返回的 OTLP JSON 结构；Tempo 数据保留期结束后，对应详情将无法继续打开。

Prometheus 暂无 Agent 指标或查询返回非有限计算结果时，对应周期指标显示为零。轮次列表仍从 Luna 数据库分页读取；Tempo 不可用时只影响右侧步骤时间轴。

从源码开发 Luna DevOps 时，仓库还提供一套只绑定本机端口的可选 Compose 观测环境：

```bash
docker compose -f docker-compose-dev-observability.yaml up -d
```

宿主机进程使用 `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`，Agent 观测的三个查询根地址分别为 `http://localhost:9090`、`http://localhost:3100` 和 `http://localhost:3200`。这套环境默认无生产级鉴权，只应用于本机开发；详细容器内地址和清理方式见仓库 `observability/README.md`。

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
