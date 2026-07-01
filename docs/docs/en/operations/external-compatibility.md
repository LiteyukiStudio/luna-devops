# External Component Compatibility Matrix

Last updated: 2026-07-01.

This page documents the external APIs and platform components that the current Liteyuki DevOps codebase integrates with. "Supported range" means the range that should be prioritized for validation with the current implementation. "Recommended version" is the preferred choice for new deployments or troubleshooting. SaaS platforms are documented by their current public API because there is no installable server version.

## Compatibility overview

| External component | Interface or capability used | Supported range | Recommended version | Notes |
| --- | --- | --- | --- | --- |
| GitHub.com / GitHub Enterprise Server | REST API, OAuth App, Webhooks, repository/branch/content reads; requests send `X-GitHub-Api-Version: 2022-11-28` | Current GitHub.com; GHES `3.17 ~ 3.21` as of the 2026-07-01 support window | GitHub.com or GHES `>= 3.18` | Older GHES versions may still expose the endpoints but be out of security support. GHES 3.17 closes down on 2026-08-25. Validate OAuth callback, webhook creation, `/user/repos`, and contents API after upgrades. |
| Gitea | `/api/v1` REST API, OAuth2, repository search, branches, contents, repository webhooks | `1.20.x ~ 1.25.x` | `1.25.x` or current stable | Gitea's REST API is released with the instance. Check the instance Swagger/OpenAPI page before connecting older private deployments. |
| GitLab | Model enum only; provider is not implemented | Not supported | Not applicable | Do not present GitLab as available yet. Add a separate GitLab REST API v4 compatibility entry when implementing it. |
| Docker Hub | Docker Hub API v2 repository search and tag listing | Current Docker Hub public API v2 | Current Docker Hub SaaS | Docker Hub is SaaS and has no installable version range. Watch for rate limits and network reachability. |
| Harbor | Harbor `/api/v2.0/search`, `/api/v2.0/projects/{project}/repositories/{repo}/artifacts`, with Distribution API fallback | `>= 2.0`, validate primarily with `2.10.x ~ 2.14.x` | `2.14.x` or current maintained release | Harbor 2.x keeps the `/api/v2.0` path. Keep Basic/Auth Token compatibility in smoke tests. |
| Generic OCI/Docker Registry | Docker Registry HTTP API V2: `/v2/`, `/_catalog`, `/tags/list` | Distribution API V2 compatible or OCI Distribution Spec `1.0 ~ 1.1` | Registry that passes OCI Distribution Spec 1.1 compatibility | The platform only depends on basic catalog/tag APIs. Some registries disable catalog listing; manual image input still works. |
| Kubernetes / K3s | `core/v1`, `apps/v1`, `batch/v1`, `networking.k8s.io/v1`, Pod logs/exec/events, dynamic client | Official support follows `client-go v0.36.x`: Kubernetes `1.34 ~ 1.36`; lower versions are not guaranteed | Kubernetes/K3s `1.34 ~ 1.36` | The code uses `k8s.io/client-go v0.36.2`. For K3s, map the K3s release to its embedded Kubernetes minor version. |
| Metrics Server | `metrics.k8s.io/v1beta1` Pod metrics | Metrics Server compatible with the Kubernetes version | Distribution-recommended Metrics Server version | Missing metrics degrade live resource metrics only; build and release flows still work. |
| Kubernetes Gateway API | `gateway.networking.k8s.io/v1` GatewayClass, Gateway, HTTPRoute, HTTPRoute filters | Gateway API `1.0.0 ~ 1.6.x` | `v1.6.x` CRDs, close to the code dependency | The code depends on `sigs.k8s.io/gateway-api v1.6.0`; Ingress is no longer the main access route path. |
| Traefik Gateway API Provider | Kubernetes Gateway provider and Gateway/HTTPRoute reconciliation | Traefik `3.x` | Latest stable Traefik `3.x` | Enable `providers.kubernetesGateway` and install Gateway API CRDs. Traefik v2 Gateway API support is not a current target. |
| cert-manager | `cert-manager.io/v1` Certificate | cert-manager `>= 1.0`; choose a release supported by the current Kubernetes version | Current maintained cert-manager release | Gateway API listeners are emitted as HTTP or HTTPS based on external TLS mode. HTTPS certificateRefs automation remains a follow-up item. |
| OpenID Connect Provider | OIDC Core 1.0, Discovery 1.0, OAuth2 Authorization Code, ID Token verification | Provider supporting OIDC Core 1.0 + Discovery 1.0 | Standard implementations such as Logto, Keycloak, Auth0, or GHES OIDC | The `issuer` must be reachable by the API service. The callback URL must match the one shown by the platform. |
| PostgreSQL | PostgreSQL wire protocol, GORM, golang-migrate | PostgreSQL `14 ~ 18` | `17`, matching compose/Helm defaults | SQLite is not supported. Production deployments should configure backups and connection limits. |
| Redis | Single Redis endpoint, go-redis, Asynq queues | Redis `7.x ~ 8.x` | `8`, matching compose/Helm defaults | The current configuration model uses one Redis address. Redis Cluster/Sentinel are not first-phase targets. |
| BuildKit | `moby/buildkit:*rootless`, `buildctl-daemonless.sh`, `dockerfile.v0` frontend | Primarily validated with `v0.24.x-rootless`; replacing with `v0.20+ rootless` requires smoke tests | `moby/buildkit:v0.24.0-rootless` | Build Jobs use rootless BuildKit and do not mount the host Docker socket. |
| Prometheus | Prometheus text exposition format, scraping API/Worker independent `/metrics` listeners | Prometheus `2.40+` or `3.x` | Current stable | The platform exposes metrics but does not use Prometheus as business state. |
| Grafana | Dashboard JSON and operations iframe URL | Grafana `9.x ~ 12.x` | Current stable | iframe embedding requires Grafana-side `allow_embedding` and proper authentication / origin policy handling. |
| SMTP | SMTP/STARTTLS notification sending | Standard SMTP service | Enterprise mail, cloud SMTP, or current stable self-hosted SMTP | SMTP is a notification adapter. Credentials must be stored as secrets. |
| Generic webhook notification | Custom method, URL, and JSON body templates | HTTP/HTTPS endpoint | Current webhook API of the target platform | Feishu and WeCom bots can be created as webhook template snapshots. Signing and rate limits belong to the adapter or user configuration. |

## Key interface notes

### Git platforms

The current Git provider implementation supports GitHub and Gitea only. GitHub uses REST API version `2022-11-28`; GitHub Enterprise Server should stay within the official support window and include the same REST API behavior. Gitea uses the instance `/api/v1`, so private Gitea upgrades should rerun repository list, OAuth, webhook creation, and file read smoke tests.

GitLab is currently only a model enum, not an available provider. Implementing it later should add a separate compatibility entry for GitLab REST API v4, OAuth, and webhooks.

### Registries

Registries are handled in three groups:

- Docker Hub uses Docker Hub API v2 and is validated against the current SaaS API.
- Harbor uses Harbor `/api/v2.0` for project/repository search and artifact/tag reads.
- Other registries use Docker Registry HTTP API V2 or OCI Distribution Spec basics.

If a registry disables catalog listing, users can still manually enter the image repository and tag. Search and tag suggestions degrade gracefully.

### Kubernetes and Gateway

Runtime cluster support follows the `client-go v0.36.x` official compatibility window: Kubernetes `1.34 ~ 1.36`. For K3s, use the embedded Kubernetes minor version rather than only the K3s release number.

Access routes now use Gateway API HTTPRoute as the main path. Clusters must install Gateway API CRDs and run a Gateway API capable controller. The current validation target is Traefik 3.x with its Kubernetes Gateway provider, while the platform model remains generic Gateway API instead of Traefik-specific annotations.

### Database, queue, and builds

Compose and Helm defaults use `postgres:17-alpine` and `redis:8-alpine`. When using managed external services, prefer the same major versions or a documented backward-compatible version. The default build executor is `moby/buildkit:v0.24.0-rootless`; if replacing it, validate Git clone, Dockerfile frontend, registry login, push, cache import/export, and log collection.

## Upgrade smoke tests

When upgrading external components, run at least these smoke tests:

1. GitHub/Gitea: OAuth login, repository list, branch list, Dockerfile read, webhook create or reconfigure.
2. Registry: connection test, repository search, tag listing, build push, runtime image pull.
3. Kubernetes/K3s: cluster connection test, build Job creation, Deployment/Service creation, Pod log read, Web Console exec.
4. Gateway API: after creating an access route, verify Gateway Accepted/Programmed and HTTPRoute Accepted/ResolvedRefs/Programmed.
5. OIDC: complete login, bind external identity, validate callback URL and issuer.
6. Prometheus/Grafana: scrape API/Worker metrics, import dashboard JSON, and open the iframe URL.

## References

- [GitHub REST API versions](https://docs.github.com/en/rest/about-the-rest-api/api-versions?apiVersion=2022-11-28)
- [GitHub Enterprise Server releases](https://docs.github.com/en/enterprise-server@3.18/admin/all-releases)
- [Gitea API usage](https://docs.gitea.com/development/api-usage)
- [Docker Registry HTTP API V2](https://distribution.github.io/distribution/spec/api/)
- [OCI Distribution Specification](https://specs.opencontainers.org/distribution-spec/)
- [Docker Hub API reference](https://docs.docker.com/reference/api/hub/latest/)
- [Harbor API explorer](https://goharbor.io/docs/2.14.0/working-with-projects/using-api-explorer/)
- [Kubernetes client-go compatibility](https://github.com/kubernetes/client-go#compatibility-matrix)
- [Kubernetes version skew policy](https://kubernetes.io/releases/version-skew-policy/)
- [Gateway API versioning](https://gateway-api.sigs.k8s.io/concepts/versioning/)
- [Traefik Kubernetes Gateway provider](https://doc.traefik.io/traefik/providers/kubernetes-gateway/)
- [cert-manager release policy](https://cert-manager.io/docs/releases/)
- [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html)
- [PostgreSQL versioning policy](https://www.postgresql.org/support/versioning/)
- [BuildKit rootless mode](https://github.com/moby/buildkit/blob/master/docs/rootless.md)
- [Prometheus exposition formats](https://prometheus.io/docs/instrumenting/exposition_formats/)
- [Grafana dashboard JSON model](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/view-dashboard-json-model/)
