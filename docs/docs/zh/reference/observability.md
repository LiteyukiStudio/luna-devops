# 接入可观测平台

Luna DevOps 可以通过 OpenTelemetry 同时发送链路、指标和结构化日志。平台不内置或绑定某一种可观测后端；先准备一个支持 OTLP HTTP 的 OpenTelemetry Collector，再把同一个地址配置给 API、Worker 和 Agent。

## 最小配置

把下面的环境变量注入三个服务：

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
```

重启后，服务会分别以 `luna-devops-api`、`luna-worker` 和 `luna-agent` 上报遥测。该变量留空时不会启动导出器，Collector 不可用也不会阻止业务请求。

AI 请求进入排队状态时会保留 W3C Trace Context，因此同一次操作中的 API 请求、Agent Run、模型请求、工具调用、平台回调和数据库访问可以在 Tempo 中沿同一个 Trace 查看。等待审批或用户输入后恢复执行时仍使用同一上游 Trace，并通过 Run ID 区分执行阶段。

## 查询 Trace

Luna DevOps 使用请求级 Trace：一次 AI 会话可以包含多轮对话，因此会话不是一条无限增长的 Trace。通常每轮用户消息对应一条 Trace；等待审批或用户输入后恢复的同一个 Run 仍接续原 Trace。会话、轮次和执行实例分别使用以下稳定属性关联：

| 范围 | 属性 | 用途 |
| --- | --- | --- |
| AI 会话 | `gen_ai.conversation.id` | 查询一个会话的全部轮次 |
| 对话轮次 | `luna.turn.id` | 查询一轮用户消息及其完整处理过程 |
| Agent 执行 | `luna.run.id` | 查询一次排队、恢复或重试中的执行实例 |
| 工具调用 | `luna.tool_call.id` | 定位一次具体工具调用 |
| 工具名称 | `gen_ai.tool.name` | 按工具类型汇总或筛选 |

在 Grafana Tempo 的 TraceQL 查询编辑器中，可以直接使用：

```text title="查询一次 AI 会话的全部轮次"
{ resource.service.name = "luna-agent" && span.gen_ai.conversation.id = "aicnv_xxx" }
```

```text title="查询一轮对话"
{ span.luna.turn.id = "aitrn_xxx" }
```

```text title="查询一次 Run 或工具调用"
{ span.luna.run.id = "airun_xxx" }
{ span.luna.tool_call.id = "aitool_xxx" }
```

```text title="查询某个会话中的失败链路"
{ span.gen_ai.conversation.id = "aicnv_xxx" } && { span:status = error }
```

```text title="查询耗时超过 5 秒的模型阶段"
{ resource.service.name = "luna-agent" && span:name = "agent.model.stream" && span:duration > 5s }
```

TraceQL 的 Span 名内建字段写作 `span:name`；打开一条 Trace 后，详情页筛选栏使用 `span.name`。例如隐藏 PostgreSQL 细粒度 Span 时，在详情筛选栏填写 `span.name !~ ^pg[.-].*`，再关闭“显示所有 Span”。

### 主要 Span 名称

| 类别 | Span 名称或模式 | 含义 |
| --- | --- | --- |
| HTTP 入口 | 通常为 `METHOD /route` | API 或 Agent 的 HTTP 请求入口；优先通过 HTTP 方法、路由和状态属性筛选 |
| HTTP 出站 | 自动生成的 HTTP Client Span | 调用 Agent、模型服务、外部平台或回调地址 |
| Agent Run | `agent.run.execute` | 一次 Agent Run 的执行边界 |
| 模型流式生成 | `agent.model.stream` | 主对话模型的流式输出阶段 |
| 模型非流式生成 | `gen_ai.chat.complete` | 标题、建议或其他非流式模型调用 |
| 模型健康检查 | `gen_ai.chat.health` | Provider 连通性检查 |
| 平台工具 | `agent.tool.execute` | 需要调用 Luna API 的工具执行 |
| Agent 内部工具 | `agent.tool.internal` | 卡片、选项、路由等 Agent 内部工具 |
| 工具 API 请求 | `luna_api.tool.execute` | Agent 到 Luna API 的委托交换和工具请求 |
| Provider 配置 | `luna_api.provider_config.get` | Agent 获取当前模型与运行配置 |
| 异步任务入队 | `task.enqueue.<task_type>` | API 将任务发送到 Asynq；任务类型中的 `:` 会转换为 `.` |
| 异步任务执行 | `task.process.<task_type>` | Worker 消费任务，例如 `task.process.deploy.run` |
| 构建阶段 | `worker.build.*` | 容量检查、命名空间、任务解析、BuildKit Job、结果跟踪和结算 |
| 部署阶段 | `worker.deploy.*` | 依赖解析、运行配置、资源应用、Hook 和 Rollout 验收 |
| 网关阶段 | `worker.gateway.*` | 命名空间、路由资源、DNS 和证书观测 |
| 运行时与账单 | `worker.runtime.*`、`worker.billing.*` | Release 状态同步及运行时、存储用量结算 |
| 通知与清理 | `worker.notification.*`、`worker.cleanup.*`、`worker.retention.*` | 通知发送、资源清理和数据保留任务 |
| 数据库迁移 | `database.migrate` | 数据库结构迁移；启动连接检查成功时不生成 Trace |
| PostgreSQL 自动 Span | `pg.query:*`、`pg-pool.connect` 或 SQL 操作名 | SQL 与连接池细粒度耗时，可在 Trace 详情中按需隐藏 |
| DNS 与网关探测 | `dns.lookup_cname`、`gateway_probe.*` | DNS 查询及网关探测采集 |

Worker 的具体阶段会继续携带 `task.type`、`task.id`、`task.retry_count` 等属性。按一次异步任务查询时，优先使用：

```text
{ span.task.id = "task_xxx" }
```

按任务类型查看部署链路时，可以使用：

```text
{ resource.service.name = "luna-worker" && span.task.type = "deploy:run" }
```

需要标记环境或集群时，可以增加资源属性：

```bash
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production,k8s.cluster.name=main
```

Collector 需要鉴权时，再增加 Header；不要把密钥写入 Compose 或 Helm values，生产环境应从 Secret 注入：

```bash
OTEL_EXPORTER_OTLP_HEADERS=api-key=replace-me
```

Helm 部署可以直接设置 endpoint 和资源属性：

```yaml
observability:
  otlpEndpoint: http://otel-collector.observability.svc.cluster.local:4318
  resourceAttributes: deployment.environment.name=production,k8s.cluster.name=main
```

## 本地验证

本地临时验证可以使用 Grafana 的 OpenTelemetry LGTM 一体化镜像。它不属于 Luna DevOps，不需要写入平台 Compose：

```bash
docker run --rm --name otel-lgtm \
  -p 3000:3000 \
  -p 4317:4317 \
  -p 4318:4318 \
  grafana/otel-lgtm:latest
```

源码运行时设置：

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=development
```

容器中的 Luna DevOps 访问宿主机时，把 endpoint 改为 `http://host.docker.internal:4318`。打开 `http://localhost:3000` 后，可以按 `service.name` 查看 Trace、Metrics 和 Logs。

至少验证一次数据库列表查询、一次 Worker 异步任务和一次 Agent 工具调用。正常链路应能看到入口请求、数据库或外部依赖子调用及业务阶段；失败链路应能使用同一个 Trace ID 关联到结构化错误日志。

## 生产建议

- `/healthz`、`/internal/health/live`、`/internal/health/ready` 等机器探针的成功请求只保留健康指标，不生成 Trace 或访问日志；Agent 就绪检查失败时仍记录结构化告警。用户主动发起的 Provider、集群和镜像站连通性测试不属于探针，仍保留完整观测数据。
- 在 Collector 使用 batch 和 memory limiter processor，避免后端短时不可达拖垮应用。
- 错误和慢链路可以完整保留，正常链路按容量采样。采样应在 Collector 统一完成，不要让不同服务各自使用冲突策略。
- Collector 与可观测后端放在受控网络中；跨网络上报时使用 TLS 和认证 Header。
- 日志和链路默认不会记录 Cookie、Token、Secret、密码、请求正文、模型 Prompt 或工具敏感参数。下游系统仍应设置访问控制和保留周期。
- API 的独立 Prometheus `/metrics` 入口只作为兼容抓取面；Worker 和 Agent 不开放独立指标端口。完整平台指标统一通过 OTLP 进入 Collector 和指标后端，避免跨进程抓取、重复采集和多副本 Counter 抖动。
- 导入 `grafana/dashboards/luna-devops-overview.json` 后，先看服务上报数、错误率和成功率，再依次查看 API 延迟、Worker 队列与交付结果、Agent 首 Token/工具调用和数据库容量。Stat 用于当前健康，Time series 用于趋势与分位数，Bar gauge 只用于队列积压等同一时刻的分类比较，Table 只用于慢路由与依赖明细。

OTel Collector 的接收器、处理器和后端导出器请按所选后端配置，参考 [OpenTelemetry Collector 部署文档](https://opentelemetry.io/docs/collector/deploy/)。
