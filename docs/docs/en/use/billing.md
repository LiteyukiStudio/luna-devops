# Billing and Spend Analysis

The billing page answers two questions: how much credit remains, and which projects, applications, or deployment configs are driving the spend. It brings balances, summaries, detailed analysis, and ledger entries into one place.

Project spaces generate spend, while charges are deducted from the wallet of the current billing owner. After ownership changes, new charges go to the new owner; previous ledger entries stay with the user who originally paid them.

The project-space overview shows the billing owner account with avatar, name, and email so users can confirm which user account future charges are linked to.

For legacy releases without a deployment target reference, the platform backfills or bills them only when the matching deployment target is unambiguous.

Legacy deployment targets without a delete status are normalized as active when they are not deleted.

The top balance card summarizes the current user account. Period spend, today's spend, and pending spend follow the selected project-space scope. The toolbar controls billing period, user account, and project space. Presets include this week, the last 7 days, this month, the last 30 days, this year, and last year, with a custom date range available when needed. Spend analysis, ledger entries, usage records, and category totals all use the same time range.

Regular users can only view their own billing data. Platform administrators also start on their own account, then use the user selector when they need all users or one specific user. The project-space filter updates with the selected user scope.

Platform administrators can also configure a real-world currency unit and conversion ratio in site settings. The top summary cards show the converted amount after credits, for example `1,012.24 Credits (1.01 CNY)`. This is display-only conversion; the ledger still settles in credits.

When platform administrators open user management, the user list shows each user's wallet balance for quick account checks. Users without a wallet record are displayed as 0 credits.

## Spend analysis

Spend analysis groups settled usage by project space, application, and deployment config. It shows total spend plus build, runtime, storage, gateway, and other spend.

CPU, memory, and storage are settled by deployment-config windows. Gateway traffic is settled by access-route windows and attributed back to the route's deployment config. Build usage is settled by BuildRun and attributed back to the build's deployment config.

Usage that is not linked to an application or deployment config is grouped as “Unassigned application” or “Unassigned deployment config”. Ledger entries remain the audit trail for every balance change, while spend analysis is the faster way to find which project, application, or deployment config is driving cost.

## Gateway traffic

Gateway charges are based on response egress traffic from platform-managed access routes. The platform does not read every Pod's external traffic directly, and cluster-internal service calls are not counted as public gateway traffic.

Traffic collection is optional and is not installed by default. The billing page does not query Kubernetes directly, and it no longer trusts historical installation-table status to decide whether the probe is online. It reads a short-TTL runtime state maintained by the API process instead. The probe sends a hello when it starts and refreshes the heartbeat before each scrape. If the platform has not received a valid heartbeat, the billing page shows the probe as not deployed. If the probe is alive but has not reported a positive traffic window yet, the billing page shows it as waiting for report while build, runtime, and storage spend remain available. Platform administrators can install the `Luna Gateway Traffic Probe` platform component from “App Marketplace” and choose the target runtime cluster.

A gateway or external collector reports response bytes by GatewayRoute and time window. The platform converts the bytes to GiB with the `gateway.egress_gib` billing rule and writes settled usage records. Gateway Traffic Probe reports with a dedicated system component token, and the backend checks that the reported GatewayRoute belongs to the probe's runtime cluster to prevent cross-cluster usage spoofing. Request count is currently stored for audit and future anti-abuse analysis; it is disabled for billing by default.

The built-in Gateway Traffic Probe currently uses Traefik Prometheus metrics mode. It lists platform-managed HTTPRoutes in the cluster, maps them by the `luna.devops/gateway-route-id` label, scrapes Traefik response-byte and request counters, calculates per-minute deltas, and reports the window back to the platform. The default metrics endpoint is `http://traefik.<Gateway namespace>.svc.cluster.local:9100/metrics`; if the Traefik Service name, namespace, or metrics port is different in the cluster, override it with the “Traefik Metrics URL” template parameter during installation. Traefik must expose Prometheus metrics with router/service labels enabled, such as `--metrics.prometheus.addrouterslabels=true` and `--metrics.prometheus.addserviceslabels=true`, and the probe Pod must be able to reach that endpoint.

### Traefik Prometheus metrics for Gateway Traffic Probe

Gateway Traffic Probe relies on router/service labels in Traefik Prometheus metrics to attribute traffic back to platform HTTPRoutes. Entrypoint-level metrics alone are not enough. If the probe logs keep showing:

```text
gateway traffic scrape completed routes=9 matchedRoutes=0 windows=0 reportableWindows=0 reportedWindows=0
```

the probe can list HTTPRoutes and scrape metrics, but the metrics labels cannot be matched to platform routes. Enable Prometheus metrics and router/service labels on Traefik.

For the packaged K3s Traefik, use `HelmChartConfig` to override values. After confirming that the `kube-system/traefik` HelmChart exists, create or update:

```yaml
apiVersion: helm.cattle.io/v1
kind: HelmChartConfig
metadata:
  name: traefik
  namespace: kube-system
spec:
  valuesContent: |-
    metrics:
      prometheus:
        addRoutersLabels: true
        addServicesLabels: true
```

If Traefik is installed with Helm directly, set the Traefik chart values:

```yaml
metrics:
  prometheus:
    addRoutersLabels: true
    addServicesLabels: true
```

If Traefik is managed as a hand-written Deployment or argument list, make sure the equivalent flags are present:

```text
--metrics.prometheus=true
--metrics.prometheus.addrouterslabels=true
--metrics.prometheus.addserviceslabels=true
```

Changing Traefik static configuration usually rolls Traefik Pods, so ingress traffic may be briefly affected. For production clusters, apply this during a low-traffic window and keep the original configuration available for rollback.

After updating Traefik, first verify that the probe can read the metrics endpoint:

```bash
kubectl -n <probe-namespace> exec -it <probe-pod> -- sh -c '
wget -qO- "$TRAEFIK_METRICS_URL" | grep -E "traefik_.*(requests|responses).*total" | head -30
'
```

The output should contain metrics with `router="...@kubernetesgateway"` or `service="..."` labels. Then compare them with platform HTTPRoutes:

```bash
kubectl get httproute -A \
  -l app.kubernetes.io/managed-by=luna-devops \
  -o custom-columns='NS:.metadata.namespace,NAME:.metadata.name,ROUTE_ID:.metadata.labels.luna\.devops/gateway-route-id,HOST:.spec.hostnames[*]'
```

After real access traffic is generated, probe logs should move from `matchedRoutes=0` to at least `matchedRoutes>0`, and `gateway traffic window reported` should appear when response-byte counters increase. The billing page changes from “Waiting for report” to normal gateway spend after the first positive traffic window is received.

References:

- [Traefik Prometheus metrics](https://doc.traefik.io/traefik/observability/metrics/prometheus/)
- [K3s HelmChartConfig](https://docs.k3s.io/add-ons/helm)
