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

## 配置 Agent 观测页面

平台管理员进入“全局设置 → AI 助手 → AI 高级设置”，分别填写：

- Prometheus 查询地址，例如 `http://prometheus:9090`
- Loki 查询地址，例如 `http://loki:3100`
- Tempo 查询地址，例如 `http://tempo:3200`
- 可选的 Tenant ID 和 Bearer Token

三个地址都填写后开启“Agent 可观测”，逐个执行“测试连接”。查询地址和 Token 只由 Luna API 使用，不会发送到浏览器。配置完成后在“运营面板 → Agent 观测”查看模型用量、工具调用、对话轮和 Trace。

## Prometheus 抓取

需要直接抓取 API 指标时配置：

```bash
METRICS_ENABLED=true
METRICS_ADDR=:9090
METRICS_PATH=/metrics
```

该监听器应只暴露给受控监控网络。Worker 和 Agent 指标仍通过 OTLP 导出。

## 高敏内容观测

`AI_OBSERVABILITY_CAPTURE_CONTENT=true` 可能把脱敏后的模型输入输出、工具参数和结果写入 Trace 与日志。生产环境应保持关闭；只在受控排障窗口临时开启，并先限制 Tempo/Loki 权限和保留时间。结束后恢复为 `false` 并重启 Agent。

## 验证

1. 发起一次正常 API 请求和一次构建或发布任务。
2. 在观测后端确认同一操作的 Trace、日志和指标可以关联。
3. 暂时停止 Collector，确认业务请求不被阻断且恢复后继续导出。
4. 检查遥测中没有 Token、Cookie、Secret、请求正文或模型 Prompt。

生产环境应为 Collector 和后端启用 TLS、认证、最小权限、采样和保留策略。
