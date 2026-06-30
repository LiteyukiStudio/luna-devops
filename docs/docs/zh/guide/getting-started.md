# 部署平台

部署文档已经拆成三个入口：

- [Kubernetes (Helm) 部署](/guide/deployment/kubernetes-helm)
- [Docker Compose 部署](/guide/deployment/docker-compose)
- [二进制部署](/guide/deployment/binary)

推荐优先使用 Kubernetes (Helm) 或 Docker Compose。二进制部署只适合调试、离线排障或特殊环境验证。

## 可选：启用 Metrics

平台默认不暴露可观测端口。部署时如需让 Prometheus 抓取平台自身指标，可以显式开启：

```bash
METRICS_ENABLED=true
```

开启后 API 默认暴露 `:9090/metrics`，Worker 默认暴露 `:9091/metrics`。需要调整端口或路径时再配置 `METRICS_ADDR` 和 `METRICS_PATH`。

Helm 部署可以同时启用 metrics Service、ServiceMonitor 和 Grafana dashboard ConfigMap：

```bash
helm upgrade --install liteyuki-devops ./charts/liteyuki-devops \
  --set metrics.enabled=true \
  --set metrics.service.enabled=true \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.grafanaDashboard.enabled=true
```

内置 dashboard 文件位于 `charts/liteyuki-devops/dashboards/liteyuki-devops-overview.json`，也可以直接导入 Grafana。

Grafana、Prometheus 查询、OpenTelemetry、Loki 和 Alertmanager 这类外部上报或跳转能力不提供默认地址；只有配置了真实 endpoint/base URL 后才应启用。
