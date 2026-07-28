# Deploy the Platform

Choose the deployment path that matches your environment:

- [Kubernetes (Helm)](/en/start/install/kubernetes)
- [Docker Compose](/en/start/install/docker-compose)
- [Binary](/en/start/install/binary)

Use Kubernetes (Helm) or Docker Compose for normal installations. Run the binaries directly only for debugging, offline troubleshooting, or unusual environments.

## Optional: enable metrics

Metrics listeners are disabled by default. Enable them only when Prometheus needs to scrape API and Worker metrics:

```bash
METRICS_ENABLED=true
```

The API then listens on `:9090/metrics`, while the Worker uses `:9091/metrics`. Set `METRICS_ADDR` or `METRICS_PATH` only when you need different ports or paths.

Helm can also create metrics Services and a ServiceMonitor:

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --set metrics.enabled=true \
  --set metrics.service.enabled=true \
  --set metrics.serviceMonitor.enabled=true
```

The dashboard source is `grafana/dashboards/luna-devops-overview.json`, and it can be imported directly into Grafana.

To show a Grafana dashboard inside the DevOps console, a platform administrator can set the Operations Dashboard URL in Site Settings. Use a Grafana dashboard or panel iframe URL, and enable iframe embedding in Grafana.

Grafana, Prometheus queries, OpenTelemetry, Loki, and Alertmanager all connect to real external services, so the platform cannot guess useful defaults. Configure an endpoint or base URL before enabling each integration.
