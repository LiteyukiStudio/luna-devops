# Deploy the Platform

Choose the deployment path that matches your environment:

- [Kubernetes (Helm)](/en/start/install/kubernetes)
- [Docker Compose](/en/start/install/docker-compose)
- [Binary](/en/start/install/binary)

Use Kubernetes (Helm) or Docker Compose for normal installations. Run the binaries directly only for debugging, offline troubleshooting, or unusual environments.

## Optional: enable metrics

The Prometheus compatibility listener is disabled by default. Enable it only when Prometheus needs to scrape API metrics:

```bash
METRICS_ENABLED=true
```

Only API then listens on the dedicated `:9090/metrics` endpoint. Worker and Agent export metrics through `OTEL_EXPORTER_OTLP_ENDPOINT` and do not open separate metrics ports. Set `METRICS_ADDR` or `METRICS_PATH` only to change the API endpoint.

Helm can also create metrics Services and a ServiceMonitor:

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --set metrics.enabled=true \
  --set metrics.service.enabled=true \
  --set metrics.serviceMonitor.enabled=true
```

The dashboard source is `grafana/dashboards/luna-devops-overview.json`, and it can be imported directly into Grafana. The complete dashboard expects API, Worker, and Agent OTLP Metrics in one metrics backend; scraping API `/metrics` alone provides only the API compatibility metrics.

To show a Grafana dashboard inside the DevOps console, a platform administrator can set the Operations Dashboard URL in Site Settings. Use a Grafana dashboard or panel iframe URL, and enable iframe embedding in Grafana.

Grafana, Prometheus queries, OpenTelemetry, Loki, and Alertmanager all connect to real external services, so the platform cannot guess useful defaults. Configure an endpoint or base URL before enabling each integration.
