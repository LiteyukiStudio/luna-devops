# Compatibility

Updated: 2026-08-31.

The following versions are the current priority support and test ranges. Prefer a maintained stable release for new deployments and run the smoke tests at the end after upgrades.

## Source and registries

| Component | Supported range | Notes |
| --- | --- | --- |
| GitHub | GitHub.com; GHES `3.17 ~ 3.21` | Verify OAuth, webhooks, and repository reads |
| Gitea | `1.20.x ~ 1.25.x` | Prefer the current stable release |
| GitLab | Not supported yet | Use GitHub or Gitea |
| Docker Hub | Current public API v2 | Consider rate limits and connectivity |
| Harbor | `>= 2.0`; priority tests on `2.10.x ~ 2.14.x` | Prefer a maintained release |
| Generic OCI Registry | Distribution API V2; OCI Distribution `1.0 ~ 1.1` | Enter a full image reference when catalog listing is unavailable |

## Runtime and build

| Component | Supported range | Notes |
| --- | --- | --- |
| Kubernetes / K3s | Kubernetes `1.34 ~ 1.36` | Evaluate K3s by its embedded Kubernetes version |
| Metrics Server | A version compatible with the cluster | Absence affects live resource metrics only |
| Gateway API | `1.0.0 ~ 1.6.x` | Requires CRDs and a compatible controller |
| Traefik | `3.x` | Enable the Kubernetes Gateway Provider |
| cert-manager | `>= 1.0` | Must also support the current Kubernetes version |
| PostgreSQL | `14 ~ 18` | `17` recommended; SQLite is unsupported |
| Redis | `7.x ~ 8.x` | Cluster and Sentinel are not currently supported |
| BuildKit | `v0.20+ rootless` | Default and priority test target is `v0.24.0-rootless` |

## Identity, notifications, and observability

| Component | Supported range | Notes |
| --- | --- | --- |
| OIDC Provider | OIDC Core 1.0 + Discovery 1.0 | `issuer` and callback URLs must be reachable and match |
| Prometheus | `2.40+` or `3.x` | API can be scraped; complete metrics can also use OTLP |
| Grafana | `9.x ~ 12.x` | iframe embedding requires Grafana configuration and authentication |
| SMTP | Standard SMTP / STARTTLS | Handle credentials as Secrets |
| Webhook notifications | HTTP / HTTPS endpoint | Configure target authentication and rate limits explicitly |

## Verify after upgrades

1. Git Provider: complete OAuth, read repositories and branches, and create a webhook.
2. Registry: search images, read tags, push a build, and pull it from a runtime cluster.
3. Kubernetes: create a build Job and Deployment, then read status, logs, and terminal access.
4. Gateway: confirm that Gateway and HTTPRoute become accepted and programmed.
5. OIDC: complete a real sign-in and verify the callback URL.
6. Observability: confirm that API, Worker, and Agent telemetry is queryable.

Use each component's official support matrix and the current Luna DevOps Release notes when a more exact compatibility decision is required.
