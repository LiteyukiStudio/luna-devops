# 本地可观测开发栈

`docker-compose-dev-observability.yaml` 提供仅供本地源码开发使用的可观测环境：

- OpenTelemetry Collector：接收 API、Worker、Agent 和 Web 转发的 OTLP/HTTP 或 OTLP/gRPC；
- Prometheus：抓取 Collector 转换后的业务指标；
- Loki：接收并查询 OTel 结构化日志；
- Tempo：接收并查询 Trace；
- Grafana：自动配置三个数据源，并加载仓库 `grafana/dashboards/` 中的 Dashboard。

所有宿主机端口只绑定 `127.0.0.1`。组件未配置生产级认证、高可用或对象存储，数据默认只保留 3 天，**不得把这套 Compose 用于生产或可被其他主机访问的环境**。

## 启动

先启动可观测栈：

```bash
docker compose -f docker-compose-dev-observability.yaml up -d
```

四个 Luna DevOps 进程都直接在宿主机运行，API 和 Worker 读取根目录 `.env`，Agent 读取 `luna-agent/.env.local`；两处均使用同一个 Collector 地址：

```env
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

数据库依赖单独启动：

```bash
docker compose -f docker-compose-dev-db.yaml up -d
```

Collector 不可达不会阻止业务运行，但对应遥测数据不会进入本地后端。
Prometheus 使用主机 `9090` 端口；启用这套栈时应保持 API 的兼容 Prometheus 监听器关闭（`METRICS_ENABLED=false`），业务指标仍会通过 OTLP 进入 Collector。

## 入口

| 服务 | 本地地址 | 用途 |
| --- | --- | --- |
| Grafana | `http://localhost:3000` | Dashboard 与 Explore；匿名只读，编辑登录为 `admin` / `admin` |
| Prometheus | `http://localhost:9090` | 指标查询与 Target 状态 |
| Loki | `http://localhost:3100` | Luna API/Grafana 的日志查询根地址 |
| Tempo | `http://localhost:3200` | Luna API/Grafana 的 Trace 查询根地址 |
| OTel HTTP | `http://localhost:4318` | 宿主机进程的统一 OTLP 上报根地址 |
| OTel gRPC | `localhost:4317` | 仅供需要 gRPC 的本地客户端 |

Agent 观测面板还需要平台管理员在“全局设置 → AI 助手 → AI 高级设置”中填写查询地址：

```text
Prometheus: http://localhost:9090
Loki:       http://localhost:3100
Tempo:      http://localhost:3200
```

这些地址由宿主机上的 Luna API 查询，不应填写 Collector 的上报地址。保存并开启 Agent 可观测后，运营面板会读取周期指标、分页轮次和 Tempo Span 时间轴。

## 最小检查

```bash
curl --fail http://localhost:9090/-/ready
curl --fail http://localhost:3100/ready
curl --fail http://localhost:3200/ready
curl --fail http://localhost:3000/api/health
curl --fail http://localhost:9464/metrics
```

在发起一条真实 API 请求或 Agent 对话后，再检查：

1. Prometheus `Status → Target health` 中两个 Collector target 均为 `UP`；
2. Grafana Explore 中能从 Tempo 按 `resource.service.name` 查询 Trace；
3. Grafana Explore 中能从 Loki 按 `{service_name="luna-agent"}` 查询日志；
4. Prometheus 能查询 `luna_devops_` 前缀的业务指标。

## 停止与清理

停止容器但保留本地数据：

```bash
docker compose -f docker-compose-dev-observability.yaml down
```

如需删除全部本地 Grafana、Prometheus、Loki 和 Tempo 数据，可在明确不再需要历史数据后手动执行：

```bash
docker compose -f docker-compose-dev-observability.yaml down --volumes
```

`--volumes` 会不可恢复地删除这套本地栈的全部观测数据。
