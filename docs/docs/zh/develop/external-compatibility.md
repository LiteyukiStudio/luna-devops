# 外部组件兼容矩阵

更新时间：2026-07-24。

这张表根据平台实际调用的外部接口整理，适合在安装、升级或排障前快速确认版本。“可用区间”是当前实现优先支持和测试的范围，“推荐版本”是新部署时更稳妥的选择。GitHub.com、DockerHub 这类 SaaS 没有可安装版本，因此以它们当前公开的 API 作为兼容边界。

## 兼容范围总览

| 外部组件 | 当前使用的接口或能力 | 可用区间 | 推荐版本 | 注意事项 |
| --- | --- | --- | --- | --- |
| GitHub.com / GitHub Enterprise Server | REST API、OAuth App、Webhook、仓库/分支/文件读取；请求带 `X-GitHub-Api-Version: 2022-11-28` | GitHub.com 当前版本；GHES `3.17 ~ 3.21`（截至 2026-07-01 的官方支持窗口） | GitHub.com 或 GHES `>= 3.18` | GHES 旧版本即使接口可用，也可能已经退出安全维护；3.17 将在 2026-08-25 关闭维护，升级前重点验证 OAuth 回调、Webhook 创建、`/user/repos` 和 contents API。 |
| Gitea | `/api/v1` REST API、OAuth2、仓库搜索、分支、contents、仓库 Webhook | `1.20.x ~ 1.25.x` | `1.25.x` 或当前稳定版 | Gitea 的 API 随实例版本发布；接入私有实例前先用实例自带 Swagger/OpenAPI 页面确认接口存在。 |
| GitLab | 仅保留模型枚举，当前 provider 未实现 | 暂不支持 | 不适用 | 前端或 API 不应宣称 GitLab 已可用；后续实现时需单独补版本矩阵。 |
| Docker Hub | Docker Hub API v2 搜索仓库和读取 tag | Docker Hub 当前公开 API v2 | Docker Hub SaaS 当前版本 | Docker Hub 是 SaaS，不提供可安装版本范围；注意限流和网络可达性。 |
| Harbor | Harbor `/api/v2.0/search`、`/api/v2.0/projects/{project}/repositories/{repo}/artifacts`，失败后回退 Distribution API | `>= 2.0`，按 `2.10.x ~ 2.14.x` 优先验收 | `2.14.x` 或当前维护版 | Harbor 2.x 的 API 路径保持 `/api/v2.0`；私有部署建议保留 Basic/Auth Token 兼容测试。 |
| 通用 OCI/Docker Registry | Docker Registry HTTP API V2：`/v2/`、`/_catalog`、`/tags/list` | 兼容 Distribution API V2 或 OCI Distribution Spec `1.0 ~ 1.1` | 通过 OCI Distribution Spec 1.1 兼容测试的 registry | 只依赖基础 catalog/tag 能力；不同 registry 的 catalog 权限策略不同，无法列目录时仍可手动填写镜像。 |
| Kubernetes / K3s | `core/v1`、`apps/v1`、`batch/v1`、`networking.k8s.io/v1`、Pod logs/exec/events、dynamic client | 官方支持区间随 `client-go v0.36.x` 对齐 Kubernetes `1.34 ~ 1.36`；理论最低不承诺 | Kubernetes/K3s `1.34 ~ 1.36` | 当前代码使用 `k8s.io/client-go v0.36.2`。K3s 按其内置 Kubernetes 小版本判断。旧集群可能能跑基础 Deployment/Job，但不作为支持承诺。 |
| Metrics Server | `metrics.k8s.io/v1beta1` Pod metrics | 与所用 Kubernetes 版本兼容的 Metrics Server | 集群发行版推荐版本 | 缺失时资源实时指标降级，不影响构建和发布主流程。 |
| Kubernetes Gateway API | `gateway.networking.k8s.io/v1` 的 GatewayClass、Gateway、HTTPRoute、HTTPRoute filters | Gateway API `1.0.0 ~ 1.6.x` | 安装与代码依赖接近的 `v1.6.x` CRD | 当前代码依赖 `sigs.k8s.io/gateway-api v1.6.0`，主路径不再创建 Ingress。 |
| Traefik Gateway API Provider | Kubernetes Gateway provider、Gateway/HTTPRoute 调谐 | Traefik `3.x` | Traefik `3.x` 最新稳定版 | 需要启用 `providers.kubernetesGateway` 并安装 Gateway API CRD。Traefik v2 的 Gateway API 支持不作为当前支持目标。 |
| cert-manager | `cert-manager.io/v1` Certificate | cert-manager `>= 1.0`；与当前 Kubernetes 搭配时按 cert-manager 官方支持矩阵选择 | cert-manager 当前维护版 | 运行集群支持手动 TLS Secret `certificateRefs`；HTTP Challenge 和 DNS-01 wildcard 模式可创建 Certificate 并把 Secret 引到 Gateway HTTPS listener。DNS Provider 凭据和 solver 由 Issuer/ClusterIssuer 维护。 |
| OpenID Connect Provider | OIDC Core 1.0、Discovery 1.0、OAuth2 Authorization Code、ID Token 校验 | 支持 OIDC Core 1.0 + Discovery 1.0 的 provider | Logto、Keycloak、Auth0、GitHub Enterprise OIDC 等标准实现 | `issuer` 必须能被 API 服务端访问；回调地址必须等于平台展示的 callback URL。 |
| PostgreSQL | PostgreSQL wire protocol、GORM、golang-migrate | PostgreSQL `14 ~ 18` | `17`，与 compose/Helm 默认一致 | 项目不支持 SQLite；生产环境建议启用备份和连接池限制。 |
| Redis | Redis 单实例、go-redis、Asynq 队列 | Redis `7.x ~ 8.x` | `8`，与 compose/Helm 默认一致 | 当前配置模型是单地址 Redis；Redis Cluster/Sentinel 不是第一阶段支持目标。 |
| BuildKit | `moby/buildkit:*rootless`、`buildctl-daemonless.sh`、`dockerfile.v0` frontend | 重点验收 `v0.24.x-rootless`；替换为 `v0.20+ rootless` 需自行 smoke test | `moby/buildkit:v0.24.0-rootless` | 构建 Job 使用 rootless BuildKit，不挂载宿主机 Docker socket。 |
| Prometheus | Prometheus text exposition format，抓取 API/Worker 独立 `/metrics` listener | Prometheus `2.40+` 或 `3.x` | 当前稳定版 | 平台只暴露指标，不依赖 Prometheus 写回业务状态。 |
| Grafana | Dashboard JSON、运营面板 iframe 嵌入地址 | Grafana `9.x ~ 12.x` | 当前稳定版 | iframe 嵌入需要 Grafana 侧开启 `allow_embedding`，并自行处理认证和同源策略。 |
| SMTP | SMTP/STARTTLS 发送通知 | 支持标准 SMTP 的服务 | 企业邮箱、云厂商 SMTP 或自建 SMTP 当前稳定版 | SMTP 属于通知适配器；凭据必须按 Secret 处理。 |
| 自由 Webhook 通知 | 自定义方法、URL、JSON body 模板 | HTTP/HTTPS endpoint | 目标平台当前 Webhook API | 飞书、企业微信机器人等可以由 Webhook 模板快照生成；目标平台的验签和限流由对应适配器或用户配置负责。 |

## 重点接口说明

### Git 平台

当前 Git provider 只实现 GitHub 和 Gitea。GitHub 使用 REST API version `2022-11-28`，GitHub Enterprise Server 应选择仍在官方支持期内、且包含该 REST API 行为的版本。Gitea 使用实例 `/api/v1`，由于 Gitea 的 REST API 随实例版本发布，私有实例升级前应在测试环境重新跑仓库列表、OAuth、Webhook 创建和文件读取。

GitLab 目前只是模型枚举，不是可用 provider。若后续实现 GitLab，应新增 GitLab REST API v4、OAuth、Webhook 的单独兼容项。

### 镜像仓库

镜像站分三类处理：

- Docker Hub 走 Docker Hub API v2，只能按 SaaS 当前 API 验收。
- Harbor 优先走 Harbor `/api/v2.0`，用于搜索项目/仓库和读取 artifact/tag。
- 其他 registry 走 Docker Registry HTTP API V2 或 OCI Distribution Spec 的基础接口。

如果 registry 禁止 catalog 列表，平台仍允许用户手动填写镜像仓库和 tag；搜索和 tag 建议会降级。

### Kubernetes 与 Gateway

运行集群的官方支持区间按 `client-go v0.36.x` 对齐 Kubernetes `1.34 ~ 1.36`。K3s 应看它内置的 Kubernetes 小版本，而不是只看 K3s 发行号。

访问入口主路径已经切换到 Gateway API HTTPRoute。集群必须提前安装 Gateway API CRD，并部署支持 Gateway API 的控制器。当前优先按 Traefik 3.x 的 Kubernetes Gateway provider 验收，但平台模型保留 Gateway API 的通用语义，不把业务字段写死到 Traefik annotation。

镜像站、单镜像站凭据和运行集群管理列表支持 `page`、`pageSize`、`sortBy`、`sortOrder` 查询参数。为兼容部署配置等少量选项读取场景，省略分页参数时仍返回原数组；管理页始终使用分页响应，避免资源数量增长后一次性加载全部记录。

### 数据库、队列和构建

PostgreSQL 和 Redis 的 compose/Helm 默认镜像分别是 `postgres:17-alpine` 和 `redis:8-alpine`。如使用外部托管服务，建议保持同一主版本或向后兼容版本。构建 executor 默认是 `moby/buildkit:v0.24.0-rootless`；替换 BuildKit 镜像时，至少验证 Git clone、Dockerfile frontend、registry 登录、push、cache import/export 和日志采集。

## 升级验收建议

升级外部组件后，至少走一遍以下冒烟测试，不要只以“连接成功”作为验收结果：

1. GitHub/Gitea：OAuth 登录、仓库列表、分支列表、读取 Dockerfile、创建或重配 Webhook。
2. Registry：连接测试、搜索仓库、读取 tag、构建后推送镜像、运行集群拉取镜像。
3. Kubernetes/K3s：测试集群连接、创建构建 Job、创建 Deployment/Service、读取 Pod 日志、Web Console exec。
4. Gateway API：创建访问入口后确认 Gateway Accepted/Programmed、HTTPRoute Accepted/ResolvedRefs/Programmed。
5. OIDC：完成登录、绑定外部身份、校验 callback URL 和 issuer。
6. Prometheus/Grafana：抓取 API/Worker metrics，导入 dashboard JSON，确认 iframe 地址可访问。

## 参考来源

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
