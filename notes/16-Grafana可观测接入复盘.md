# Grafana 可观测接入复盘

本文记录 Luna DevOps 接入 OpenTelemetry、Prometheus、Tempo、Loki 和 Grafana 时实际遇到的问题。它不是部署教程，而是研发与运维排障清单；正式插桩约束仍以 `notes/14-可观测插桩与验收标准.md` 为准。

## 1. 最重要的结论

“Grafana 无数据”不能直接等同于“服务没有上报”。这条链路至少包含四个可独立失败的阶段：

```text
应用插桩
  -> OTLP Collector 接收与处理
  -> Tempo / Loki / Prometheus 存储或抓取
  -> Grafana 数据源、变量和面板查询
```

必须按阶段验证，不能只盯着最终仪表盘反复修改应用。

本次最典型的问题是：Agent 已经向 Loki 上报 `gen_ai.content.*` 和 `gen_ai.tool.content.*`，在 Explore 中可以查到完整事件，但仪表盘仍显示“无数据”。最终根因在 Grafana 查询表达式和模板变量，而不是 Agent 采集链路。

## 2. Collector 与 Prometheus 出口容易混淆

OTel Collector 中常见的两个端口含义不同：

- `service.telemetry.metrics` 暴露的是 Collector 自身运行指标，常见端口为 `8888`；
- `exporters.prometheus.endpoint` 暴露的是应用通过 OTLP 上报后转换出的业务指标，本次使用 `9464`。

如果 Prometheus 只抓取 `otel-collector:8888`，可以看到 Collector 存活，却看不到 API、Worker 和 Agent 的业务指标。需要抓取 Prometheus exporter 的实际端口：

```yaml
scrape_configs:
  - job_name: otel-collector-exported-metrics
    static_configs:
      - targets:
          - otel-collector:9464
```

诊断顺序：

1. 在 Collector 容器内确认 `4318`/`4317`、`9464` 和健康检查端口实际监听；
2. 请求 `http://otel-collector:9464/metrics`，确认存在 `luna_devops_` 指标；
3. 在 Prometheus 的 Targets 页面确认对应 Target 为 `UP`；
4. 在 Prometheus 查询页直接查询指标；
5. 最后再检查 Grafana 面板。

Prometheus 使用 `/-/reload` 前必须以 `--web.enable-lifecycle` 启动。容器内没有 curl 时可以使用：

```bash
wget --post-data='' -O- http://localhost:9090/-/reload
```

重载只会重新读取配置，不会修复 YAML 错误。应先使用 Prometheus 自带的配置检查工具或查看启动日志。

## 3. OTLP 协议、地址和鉴权必须成套匹配

项目统一使用 OTLP HTTP/protobuf，应用只配置 Collector 根地址：

```env
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer%20replace-me
```

常见错误：

- 应用使用 HTTP exporter，却指向 gRPC 的 `4317`；
- Collector 开启 Bearer Token 鉴权，应用未传 Header 或 Header 格式错误；
- 把 Collector 的外部域名写入容器，但网络、TLS 或 DNS 并不可达；
- Collector 的 traces、logs、metrics pipeline 只配置了其中一条；
- 修改环境变量后没有重建或重启 Agent，旧进程仍使用旧配置。

不要只验证 Collector 健康检查。健康只能证明 Collector 进程存活，不能证明三条 pipeline 都完成接收和导出。

## 4. Trace、Logs、Metrics 必须分别验收

三种信号彼此独立：

| 信号 | 首选验证入口 | 最小验证内容 |
| --- | --- | --- |
| Trace | Tempo Explore | 按 `resource.service.name` 查询入口或业务根 Span |
| Logs | Loki Explore | 按 `service_name` 查询稳定 `event_name` |
| Metrics | Prometheus 查询页 | 查询原始 counter/histogram，不先依赖 Grafana 聚合式 |

一种信号正常，不代表另外两种也正常。例如 Tempo 有 Agent Trace，但 Loki 内容面板仍可能因为日志 pipeline、字段查询或高敏开关而为空。

## 5. Loki 查询的实际坑

### 5.1 OTel 日志字段不一定需要 `| json`

OTLP 导入 Loki 后，`event_name`、`trace_id`、`gen_ai_conversation_id` 等字段可能已经是 Loki 的结构化元数据，而不是日志正文中的 JSON 字段。本次在查询中额外使用 `| json`，反而导致后续字段筛选无法命中。

应先在 Explore 逐级确认：

```logql
{service_name="luna-agent"}
```

```logql
{service_name="luna-agent"} | event_name=~`gen_ai\.content\.(input|output|error)`
```

确认字段存在后再追加会话和 Run 条件。当前内容查询示例：

```logql
{service_name="luna-agent"}
| event_name=~`gen_ai\.content\.(input|output|error)`
| gen_ai_conversation_id=~`.*`
| luna_turn_id=~`.*`
| luna_run_id=~`.*`
| trace_id=~`.*`
| line_format `{{.event_name}} · {{.gen_ai_input_messages}}{{.gen_ai_output_messages}}{{.gen_ai_response_error_body}}`
```

工具内容查询示例：

```logql
{service_name="luna-agent"}
| event_name=~`gen_ai\.tool\.content\.(input|output)`
| gen_ai_conversation_id=~`.*`
| luna_turn_id=~`.*`
| luna_run_id=~`.*`
| trace_id=~`.*`
| gen_ai_tool_name=~`.*`
| line_format `{{.event_name}} · {{.gen_ai_tool_name}} · {{.luna_tool_call_id}} · {{.gen_ai_tool_call_arguments}}{{.gen_ai_tool_call_result}}`
```

### 5.2 Grafana 模板变量会改变查询文本

在 Explore 中写死 `.*` 能查到，不代表仪表盘变量替换后仍是同一个表达式。

本次踩坑：ID 输入框使用 `${variable:regex}` 后，Grafana 会对自由输入值再次转义或按多选变量规则展开，最终查询与 Explore 中验证过的表达式不同。ID 类自由文本过滤器应使用：

```text
${conversation_id:raw}
${turn_id:raw}
${run_id:raw}
${trace_id:raw}
```

工具名是多选查询变量，可以继续使用 `${tool:regex}`。不要把所有变量机械地统一为同一种格式。

诊断时必须在 Grafana Query Inspector 中检查最终发送给数据源的表达式，而不是只看 Dashboard JSON 中的模板。

### 5.3 大正文查询不是立即完成

模型输入、输出和工具结果即使经过 32 KiB 单字段限制，单条日志仍明显大于普通业务日志。六小时范围内同时加载 Prompt、回复和工具结果时，面板可能需要十余秒才完成。

因此：

- 刷新后先观察面板是否仍处于 loading，不要立刻把空白判断为无数据；
- 排障时先缩短到最近 15 分钟或 1 小时；
- 先按 Conversation ID 或 Run ID 缩小范围；
- 不要把高敏内容面板放在平台总览首页自动加载。

## 6. Tempo TraceQL 的实际坑

### 6.1 属性作用域和内置字段要写对

当前 Agent 根运行查询使用：

```traceql
{
  resource.service.name = "luna-agent"
  && span:name = "agent.run.execute"
  && span.gen_ai.conversation.id =~ ".*"
  && span.luna.turn.id =~ ".*"
  && span.luna.run.id =~ ".*"
}
```

这里存在三类不同语义：

- `resource.service.name`：Resource attribute；
- `span:name`、`span:status`：TraceQL intrinsic；
- `span.gen_ai.conversation.id`：Span attribute。

不要凭印象写成 `span.name`、`service.name` 或混用其他查询语言语法。安装的 Tempo/Grafana 版本对表达式支持也可能不同，必须先在 Explore 验证。

### 6.2 不要用负向过滤解决 PostgreSQL 噪声

Agent Trace 中出现大量 `pg.query:*` 和 `pg-pool.connect` 是完整数据库插桩的正常结果。直接尝试在 TraceQL 中对 Span 名称做负向正则，本次环境返回了 400；而且即使查询只由某个 Span 命中，Tempo 返回的仍是整条 Trace，打开 Trace 后依然会包含子 Span。

LLM 仪表盘默认采用正向选择：只在 Trace 结果表中检索 `agent.run.execute` 根业务 Span，失败面板也只检索失败的 `agent.run.execute`：

```traceql
{
  resource.service.name = "luna-agent"
  && span:name = "agent.run.execute"
  && span.gen_ai.conversation.id =~ ".*"
  && span:status = error
}
```

这能避免把数据库子 Span 当成 Agent Run 搜索结果。打开一条完整 Trace 后仍可按需查看数据库细节，这是诊断能力，不应从采集端删除。

### 6.3 `<root span not yet received>` 不等于完全没有 Trace

Grafana 出现 `<root span not yet received>` 时，通常表示 Tempo 已收到部分子 Span，但根 Span 尚未完成、尚未送达或该批次还未合并。先等待一个 batch 周期并用 Trace ID 重查，再检查：

- 根 Span 是否在进程异常退出前结束；
- BatchSpanProcessor 是否有机会 flush；
- Collector 与 Tempo 是否丢弃或超时；
- W3C 父上下文是否被错误创建成远端但没有对应上游根 Span。

不要仅凭这一行判断 service name 没上报。

## 7. 高敏 LLM 内容采集的边界

Prompt、回复、Reasoning 摘要、工具参数和工具结果默认不采集。必须在 Agent 进程中显式设置并重启：

```env
AI_OBSERVABILITY_CAPTURE_CONTENT=true
```

注意：

- 该开关只影响启用后的新调用，不会补采历史对话；
- 在容器终端输入变量名会被 Shell 当作命令，检查值应使用 `echo "$AI_OBSERVABILITY_CAPTURE_CONTENT"`；
- 开关为 `true` 只能证明配置进入进程，仍需制造一次新的模型和工具调用验证上报；
- 内容会统一脱敏并限制为 32 KiB，但仍可能包含业务隐私；
- 只在排障窗口开启，使用较短 Loki/Tempo 保留期，完成后关闭并重启 Agent。

至少应看到这些事件：

```text
gen_ai.content.input
gen_ai.content.output
gen_ai.content.error
gen_ai.tool.content.input
gen_ai.tool.content.output
```

## 8. Grafana Dashboard JSON 的维护方式

### 8.1 数据源必须在导入时正确映射

LLM 仪表盘同时依赖 Prometheus、Tempo 和 Loki。导入时三者必须映射到实际数据源；一个映射错误只会让部分面板无数据，容易被误判为应用插桩不完整。

### 8.2 修改后提高 Dashboard version 并覆盖导入

同一 UID 的仪表盘修改后应提高 `version`，导入时选择覆盖现有 Dashboard。否则浏览器可能仍展示旧查询。

覆盖后还要：

1. 刷新页面；
2. 检查时间范围和顶部变量；
3. 等待查询结束；
4. 在 Query Inspector 中确认线上表达式确实是新版；
5. 不要只依据本地 JSON 正确就宣布完成。

### 8.3 JSON 与线上界面必须双向验证

推荐流程：

1. 在 Explore 使用常量表达式找到真实数据；
2. 把已验证表达式写入仓库 JSON；
3. `jq empty` 检查 JSON；
4. 在 Grafana 覆盖导入；
5. 通过真实业务操作制造新数据；
6. 在线确认 Metrics、Logs、Trace 三部分；
7. 将线上最终表达式与仓库 JSON 对比，防止只在 UI 临时修改而没有进入版本控制。

## 9. 本次最终验收记录

本次使用生产实例发起一轮只读 Agent 对话，要求 Agent 查询项目空间详情和应用列表。验收结果：

- Loki 能查询到本轮 `gen_ai.content.input`、`gen_ai.content.output`；
- 内容面板能展示模型输入、回复和工具调用信息；
- 工具面板能展示 `getProject`、`listApplications`、`create_options` 的参数与结果；
- Correlated failures 能展示 `agent.tool.failed` 和 `gen_ai.content.error`；
- Tempo 能按 `agent.run.execute` 查询 Agent Run；
- 仪表盘 Trace 结果表不再把 PostgreSQL 子 Span 当作 Agent Run；
- 查询不再返回 TraceQL 400；
- Dashboard JSON、文档构建和线上覆盖导入均已验证。

## 10. 后续接入检查清单

### 应用与 Collector

- [ ] 服务进程实际读取了 OTLP endpoint 和鉴权 Header；
- [ ] endpoint 协议和 Collector receiver 端口一致；
- [ ] traces、logs、metrics 三条 pipeline 都配置并启动；
- [ ] Collector exporter 目标可达，错误日志中没有持续重试或丢弃；
- [ ] `service.name` 使用项目约定的固定值。

### 存储与抓取

- [ ] Tempo 能按 service name 查到业务根 Span；
- [ ] Loki 能按 service name 和稳定 event name 查到日志；
- [ ] Prometheus 抓取的是应用指标或 Collector Prometheus exporter，而不是只抓 Collector 自身指标；
- [ ] Prometheus Target 为 `UP`，原始指标查询有数据。

### Grafana

- [ ] 三个数据源映射正确；
- [ ] 时间范围覆盖刚制造的请求；
- [ ] Dashboard 变量展开后的最终查询与 Explore 验证式一致；
- [ ] ID 自由输入变量使用 `:raw`，多选工具变量按需要使用 `:regex`；
- [ ] Loki 字段属于结构化元数据时没有误加 `| json`；
- [ ] Trace 结果优先查询稳定业务根 Span，不用负向过滤掩盖子 Span；
- [ ] Dashboard version 已更新且线上已覆盖导入；
- [ ] 大内容面板等待查询完成后再判定结果。

### 安全

- [ ] 默认未采集 Prompt、回复、工具参数和结果；
- [ ] 高敏开关只在明确排障窗口开启；
- [ ] 日志和 Span 中没有 Token、Cookie、密码、API Key、URL 凭据；
- [ ] 高敏数据保留期、访问权限和关闭流程已确认。
