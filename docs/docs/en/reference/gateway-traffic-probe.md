# Gateway Traffic Probe

Luna Gateway Traffic Probe is an optional platform component that measures response egress for Luna DevOps-managed HTTPRoutes and records `gateway.egress_gib` usage. Do not install it when traffic billing is not required.

## Install and configure

Search for **Luna Gateway Traffic Probe** in App Marketplace and install it:

1. Select the target runtime cluster.
2. Confirm the image. The default is suitable for testing; pin a version tag in production.
3. Choose whether Luna DevOps should create a dedicated ServiceAccount and RBAC. Otherwise, grant read access to Gateway API routes yourself.
4. Enter the platform `API_BASE_URL` and confirm that the probe cluster can reach it.
5. Enter `TRAEFIK_METRICS_URL` when the Traefik metrics endpoint is not the default.

Installation generates a dedicated `REPORT_TOKEN` and stores it as a Secret without displaying plaintext. Do not edit this token or the runtime cluster ID manually.

## Required configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `API_BASE_URL` | Entered during installation | Sets the platform API used for reports; use an HTTP(S) URL reachable from the cluster. |
| `TRAEFIK_METRICS_URL` | In-cluster Traefik `/metrics` | Sets the Traefik metrics source; use a Prometheus metrics HTTP(S) URL. |
| `GATEWAY_NAMESPACE` | `kube-system` | Limits where the probe reads Traefik; use a Kubernetes Namespace name. |
| `SCRAPE_INTERVAL` | `1m` | Sets the metrics collection interval; use a Go duration of at least `10s`. |
| `ROUTE_REFRESH_INTERVAL` | `1m` | Sets the route refresh interval; use a Go duration of at least `10s`. |
| `PROBE_ADDR` | `:9090` | Sets the probe health and metrics listen address; use `IP:port` or `:port`. |

Traefik must expose Prometheus metrics with router and service labels. After installation, check the workload on the application deployment page, then verify the probe `/healthz`, logs, and billing-page collection status.

## How it works

The probe periodically reads HTTPRoutes, maps Traefik response-byte and request counters to Luna DevOps routes, and reports the interval delta. Luna DevOps verifies cluster ownership and de-duplicates repeated windows. Reports contain no request body, source IP, or user information.
