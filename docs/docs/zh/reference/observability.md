# 可观测

Luna DevOps 通过 OpenTelemetry 导出 Trace、Metrics 和结构化日志。你需要准备支持 OTLP HTTP 的 OpenTelemetry Collector，以及按需使用的 Prometheus、Loki 和 Tempo。

## 导出遥测

为 API、Worker 和 Agent 配置同一个 Collector 地址：

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production,k8s.cluster.name=main
```

地址留空会关闭导出。Collector 需要认证时，通过 Secret 注入 `OTEL_EXPORTER_OTLP_HEADERS`，不要把凭据写入公开配置文件。

重启后确认 Collector 能收到以下服务：

- `luna-devops-api`
- `luna-worker`
- `luna-agent`（启用 AI 助手时）

## 阅读与检索日志

日志记录始终结构化，但终端渲染与 OTel 导出彼此独立。本地使用默认的 `LOG_FORMAT=auto`：交互式终端显示每条记录占一行的 console 格式，重定向后自动输出 JSON；Docker Compose 保留 `auto` 并在非交互容器中自动输出 JSON，Helm 的生产配置显式使用 `LOG_FORMAT=json`。`LOG_COLOR=auto|always|never` 只影响 console，`NO_COLOR` 会强制禁用颜色，JSON 和 OTel 记录不会包含 ANSI。

失败日志使用稳定的 `event.name`、`operation`、`outcome` 和 `error.code`，并在 `error.message` 保留经凭据遮罩的完整错误链；存在上下文时还包含 `trace_id`、`span_id`、`request_id` 和资源 ID。排障时先用响应中的 `requestId` 或 `traceId` 检索日志，再查看完整依赖错误。内部地址、文件路径、SQLSTATE 和资源 ID 属于诊断信息，不会被删除；Token、Authorization、Cookie、密码、API Key、私钥等真实凭据值会被遮罩。

生产 API 错误响应只包含稳定 `code`、通用 `message` 本地化键和可用的 `requestId` / `traceId`，不会返回数据库错误、内部地址或堆栈。`APP_ENV=development` 时会额外返回经凭据遮罩的 `developerDetail`。

## 配置 Agent 观测页面

平台管理员进入“全局设置 → AI 助手 → AI 高级设置”，分别填写：

- Prometheus 查询地址，例如 `http://prometheus:9090`
- Loki 查询地址，例如 `http://loki:3100`
- Tempo 查询地址，例如 `http://tempo:3200`
- 可选的 Tenant ID 和 Bearer Token

三个地址都填写后开启“Agent 可观测”，逐个执行“测试连接”。查询地址和 Token 只由 Luna API 使用，不会发送到浏览器。配置完成后在“运营面板 → Agent 观测”查看模型用量、工具调用、对话轮和 Trace。

打开某一轮对话详情后，头部会展示该轮完整 Trace ID；点击该信息块即可复制，用于在 Tempo、日志平台或诊断工单中检索同一条调用链。

### 理解模型 Token 用量

会话总览、轮次列表和轮次详情都使用 Provider 官方 `usage` 作为事实源：

- 输入 Token 是完整输入量，已经包含缓存读取和缓存写入；输出 Token 已经包含 reasoning。缓存与 reasoning 数值只用于拆分说明，不能再次加到输入或输出。
- 缓存读取、缓存写入或 reasoning 显示为 `—`，表示 Provider 没有报告该明细；显示 `0` 才表示 Provider 明确报告为零。
- Agent 模型 Span 使用 `gen_ai.usage.input_tokens`、`gen_ai.usage.output_tokens`、`gen_ai.usage.cache_read.input_tokens`、`gen_ai.usage.cache_write.input_tokens` 和 `gen_ai.usage.reasoning.output_tokens`。用量不可用时，可查看 `luna.gen_ai.usage.status` 与 `luna.gen_ai.usage.unavailable_reason`。
- 输入与输出吞吐使用标准 Histogram `gen_ai.client.token.usage`，并由 `gen_ai.token.type` 区分 `input` 和 `output`；缓存与 reasoning 明细仍从 Span 或轮次接口读取。

缓存明细是否存在取决于模型 Provider 的官方响应；未报告缓存写入不能据此推断发生了缓存写入或写入量为零。

### 分析 Prompt 缓存命中率

1. 在 Agent 观测总览选择要分析的时段。总览命中率汇总该时段内纳入统计的官方模型用量。
2. 打开 Trace 详情时，将范围收窄到当前 Trace 中标记为 `assistant` 的模型调用；标题生成和上下文摘要分别标记为 `title` 与 `summary`，不会混入助手回复的 Token 与缓存命中率。
3. 两个层级都先分别汇总 Token，再计算官方加权命中率：`Σ缓存读取输入 Token / Σ输入 Token`。不要对每次请求的命中率做算术平均。

缓存写入已经包含在输入 Token 中，因此属于分母，但不属于缓存命中分子。单独的缓存写入明细缺失不会改变总输入分母。若任一纳入统计的官方用量缺少缓存读取明细，或汇总输入 Token 为零，命中率显示为 `—`（API 中为 `null`）；只有汇总输入 Token 大于零且缓存读取明确汇总为零时，才显示 `0%`。

## Prometheus 抓取

需要直接抓取 API 指标时配置：

```bash
METRICS_ENABLED=true
METRICS_ADDR=:9090
METRICS_PATH=/metrics
```

该监听器应只暴露给受控监控网络。Worker 和 Agent 指标仍通过 OTLP 导出。

## 高敏内容观测

`AI_OBSERVABILITY_CAPTURE_CONTENT=true` 可能把脱敏后的模型输入输出、工具参数和结果写入受控 Trace；关联日志只记录事件元数据，不记录 Prompt 或工具参数。生产环境应保持关闭；只在受控排障窗口临时开启，并先限制 Tempo 权限和保留时间。结束后恢复为 `false` 并重启 Agent。

## 验证

1. 发起一次正常 API 请求和一次构建或发布任务。
2. 在观测后端确认同一操作的 Trace、日志和指标可以关联。
3. 暂时停止 Collector，确认业务请求不被阻断且恢复后继续导出。
4. 检查遥测中没有 Token、Cookie、Secret、请求正文或模型 Prompt。

生产环境应为 Collector 和后端启用 TLS、认证、最小权限、采样和保留策略。
