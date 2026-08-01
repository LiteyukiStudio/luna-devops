# 接入可观测平台

Luna DevOps 可以通过 OpenTelemetry 同时发送链路、指标和结构化日志。平台不内置或绑定某一种可观测后端；先准备一个支持 OTLP HTTP 的 OpenTelemetry Collector，再把同一个地址配置给 API、Worker 和 Agent。

## 最小配置

把下面的环境变量注入三个服务：

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
```

重启后，服务会分别以 `luna-devops-api`、`luna-worker` 和 `luna-agent` 上报遥测。该变量留空时不会启动导出器，Collector 不可用也不会阻止业务请求。

AI 请求进入排队状态时会保留 W3C Trace Context，因此同一次操作中的 API 请求、Agent Run、模型请求、工具调用、平台回调和数据库访问可以在 Tempo 中沿同一个 Trace 查看。等待审批或用户输入后恢复执行时仍使用同一上游 Trace，并通过 Run ID 区分执行阶段。

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

- 在 Collector 使用 batch 和 memory limiter processor，避免后端短时不可达拖垮应用。
- 错误和慢链路可以完整保留，正常链路按容量采样。采样应在 Collector 统一完成，不要让不同服务各自使用冲突策略。
- Collector 与可观测后端放在受控网络中；跨网络上报时使用 TLS 和认证 Header。
- 日志和链路默认不会记录 Cookie、Token、Secret、密码、请求正文、模型 Prompt 或工具敏感参数。下游系统仍应设置访问控制和保留周期。
- 原有 Prometheus `/metrics` 入口可以继续用于已有抓取系统；OTLP Metrics 适合统一接入 Collector，两者不要分别维护不同告警口径。

OTel Collector 的接收器、处理器和后端导出器请按所选后端配置，参考 [OpenTelemetry Collector 部署文档](https://opentelemetry.io/docs/collector/deploy/)。
