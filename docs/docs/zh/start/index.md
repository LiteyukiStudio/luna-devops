# 部署平台

根据运行环境选择一种部署方式：

- [Kubernetes (Helm) 部署](/start/install/kubernetes)
- [Docker Compose 部署](/start/install/docker-compose)
- [二进制部署](/start/install/binary)

日常使用优先选 Kubernetes (Helm) 或 Docker Compose。只有在调试、离线排障或特殊环境验证时，才建议直接运行二进制。

## 可选：启用 Metrics

平台默认关闭指标端口。需要让 Prometheus 抓取 API 和 Worker 指标时，再显式开启：

```bash
METRICS_ENABLED=true
```

开启后 API 默认暴露 `:9090/metrics`，Worker 默认暴露 `:9091/metrics`。需要调整端口或路径时再配置 `METRICS_ADDR` 和 `METRICS_PATH`。

Helm 部署可以同时启用 metrics Service 和 ServiceMonitor：

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --set metrics.enabled=true \
  --set metrics.service.enabled=true \
  --set metrics.serviceMonitor.enabled=true
```

Grafana dashboard 文件位于 `grafana/dashboards/luna-devops-overview.json`，可以直接导入 Grafana。

如果希望在 DevOps 控制台里查看 Grafana 大盘，平台管理员可以在“站点设置”中填写“运营面板地址”。该地址应使用 Grafana dashboard 或 panel 的 iframe 嵌入地址；Grafana 侧需要允许 iframe 嵌入。

Grafana、Prometheus 查询、OpenTelemetry、Loki 和 Alertmanager 都需要连接真实的外部服务，因此平台不会为它们猜测默认地址。请先准备好 endpoint 或 base URL，再开启对应功能。
