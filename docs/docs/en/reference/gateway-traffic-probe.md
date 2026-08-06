# Gateway Traffic Probe

`Luna Gateway Traffic Probe` is an optional collector deployed into a target runtime cluster. It measures response egress traffic from platform-managed routes so billing can meter it as `gateway.egress_gib`. Clusters that do not need traffic-based billing do not need it.

## How it works

The probe runs a collection cycle on a fixed interval (1 minute by default):

1. It reads the Gateway API through the in-cluster ServiceAccount and lists the HTTPRoutes of the cluster, resolving the platform routes to measure.
2. It scrapes the Traefik Prometheus metrics endpoint and matches counters to platform routes by router/service labels.
3. For each route, it diffs the response-bytes and request counters to produce the traffic window for the period. If a counter resets after a restart, the current value is used.
4. It reports windows that carry traffic to the platform API, which settles them as usage. Reported windows are deduplicated idempotently; a repeated report returns `already_settled` and is never billed twice.

Only response egress traffic from platform-managed HTTPRoutes is counted. Traffic between services inside the cluster is excluded.

## Reporting and authentication

- When the probe is installed, the platform generates a random report token (shaped like `lyd_probe_<hex>`). The real value is delivered to the cluster only at install time and injected into the probe as the `REPORT_TOKEN` secret. The platform stores only an encrypted copy and the SHA-256 hash of the token.
- Every report carries `Authorization: Bearer <REPORT_TOKEN>`. The platform authenticates by looking up the installation via the token hash and **never stores the plaintext token**; the token is also kept out of logs and telemetry.
- Before settling, the platform verifies that the reported route belongs to the probe's own runtime cluster, so a token from one cluster cannot report routes of another.
- A reported window contains the route ID, response bytes, request count, and time range. It contains no request bodies, source IPs, or user information.

## Configuration

The probe is configured through environment variables. Sensible defaults are written at install time; usually you only need to confirm the following:

| Variable | Default | Description |
| --- | --- | --- |
| `API_BASE_URL` | set at install | Platform API address the probe can reach |
| `REPORT_TOKEN` | set at install | Report token from a platform secret; do not edit |
| `RUNTIME_CLUSTER_ID` | set at install | ID of the runtime cluster hosting the probe |
| `TRAEFIK_METRICS_URL` | `http://traefik.<gateway-namespace>.svc.cluster.local:9100/metrics` | Traefik Prometheus metrics address |
| `GATEWAY_NAMESPACE` | `kube-system` | Traefik namespace, used to build the default metrics address |
| `SCRAPE_INTERVAL` | `1m` | Collection interval, minimum 10s |
| `ROUTE_REFRESH_INTERVAL` | `1m` | Route refresh interval, minimum 10s |
| `PROBE_ADDR` | `:9090` | Listen address for the probe's own `/healthz` and `/metrics` |

## Prerequisites

- Traefik exposes Prometheus metrics with router and service labels. Without these labels the probe cannot match traffic to platform routes.
- The cluster hosting the probe can reach the platform API at `API_BASE_URL`.

## Troubleshooting

The probe exposes `/healthz` and `/metrics` on `PROBE_ADDR`, reporting route count, last scrape/report timestamps, and the current error.

Meaning of the collection status on the billing page:

- **Not deployed**: the probe is not installed in the target cluster.
- **Waiting for reports**: the probe is ready but the platform has not received valid traffic data. Check probe logs, Traefik metric labels, and connectivity from the probe to the platform API.
- **Unavailable**: the platform cannot read the target cluster status.

When collection fails, first look for authentication or network errors in the probe logs, then confirm Traefik metrics include router/service labels. Changing a production gateway can briefly interrupt traffic; do it during a suitable window and keep a rollback path.
