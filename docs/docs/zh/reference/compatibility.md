# 外部组件兼容矩阵

更新时间：2026-07-24。

这张表根据平台实际调用的外部接口整理，适合在安装、升级或排障前快速确认版本。“可用区间”是当前实现优先支持和测试的范围，“推荐版本”是新部署时更稳妥的选择。GitHub.com、DockerHub 这类 SaaS 没有可安装版本，因此以它们当前公开的 API 作为兼容边界。

## 兼容范围总览

| 外部组件 | 当前使用的接口或能力 | 可用区间 | 推荐版本 | 注意事项 |
| --- | --- | --- | --- | --- |
| GitHub.com / GitHub Enterprise Server | REST API、OAuth App、Webhook、仓库/分支/文件读取；请求带 `X-GitHub-Api-Version: 2022-11-28` | GitHub.com 当前版本；GHES `3.17 ~ 3.21`（截至 2026-07-01 的官方支持窗口） | GitHub.com 或 GHES `>= 3.18` | GHES 旧版本即使接口可用，也可能已经退出安全维护；3.17 将在 2026-08-25 关闭维护，升级前重点验证 OAuth 回调、Webhook 创建、`/user/repos` 和 contents API。 |
| Gitea | `/api/v1` REST API、OAuth2、仓库搜索、分支、contents、仓库 Webhook | `1.20.x ~ 1.25.x` | `1.25.x` 或当前稳定版 | Gitea 的 API 随实例版本发布；接入私有实例前先用实例自带 Swagger/OpenAPI 页面确认接口存在。 |
| GitLab | 当前不可用 | 暂不支持 | 不适用 | 请使用 GitHub 或 Gitea。 |
| Docker Hub | Docker Hub API v2 搜索仓库和读取 tag | Docker Hub 当前公开 API v2 | Docker Hub SaaS 当前版本 | Docker Hub 是 SaaS，不提供可安装版本范围；注意限流和网络可达性。 |
| Harbor | Harbor `/api/v2.0/search`、`/api/v2.0/projects/{project}/repositories/{repo}/artifacts`，失败后回退 Distribution API | `>= 2.0`，按 `2.10.x ~ 2.14.x` 优先验收 | `2.14.x` 或当前维护版 | Harbor 2.x 的 API 路径保持 `/api/v2.0`；私有部署建议保留 Basic/Auth Token 兼容测试。 |
| 通用 OCI/Docker Registry | Docker Registry HTTP API V2：`/v2/`、`/_catalog`、`/tags/list` | 兼容 Distribution API V2 或 OCI Distribution Spec `1.0 ~ 1.1` | 通过 OCI Distribution Spec 1.1 兼容测试的 registry | 只依赖基础 catalog/tag 能力；不同 registry 的 catalog 权限策略不同，无法列目录时仍可手动填写镜像。 |
| Kubernetes / K3s | 工作负载、构建任务、日志、终端、事件和运行状态 | Kubernetes `1.34 ~ 1.36`；更低版本不承诺兼容 | Kubernetes/K3s `1.34 ~ 1.36` | K3s 按其内置 Kubernetes 小版本判断。 |
| Metrics Server | `metrics.k8s.io/v1beta1` Pod metrics | 与所用 Kubernetes 版本兼容的 Metrics Server | 集群发行版推荐版本 | 缺失时资源实时指标降级，不影响构建和发布主流程。 |
| Kubernetes Gateway API | GatewayClass、Gateway、HTTPRoute 和路由过滤器 | Gateway API `1.0.0 ~ 1.6.x` | `v1.6.x` CRD | 需要安装 CRD 和兼容的 Gateway 控制器。 |
| Traefik Gateway API Provider | Kubernetes Gateway provider、Gateway/HTTPRoute 调谐 | Traefik `3.x` | Traefik `3.x` 最新稳定版 | 需要启用 `providers.kubernetesGateway` 并安装 Gateway API CRD。Traefik v2 的 Gateway API 支持不作为当前支持目标。 |
| cert-manager | `cert-manager.io/v1` Certificate | cert-manager `>= 1.0`；与当前 Kubernetes 搭配时按 cert-manager 官方支持矩阵选择 | cert-manager 当前维护版 | 运行集群支持手动 TLS Secret `certificateRefs`；HTTP Challenge 和 DNS-01 wildcard 模式可创建 Certificate 并把 Secret 引到 Gateway HTTPS listener。DNS Provider 凭据和 solver 由 Issuer/ClusterIssuer 维护。 |
| OpenID Connect Provider | OIDC Core 1.0、Discovery 1.0、OAuth2 Authorization Code、ID Token 校验 | 支持 OIDC Core 1.0 + Discovery 1.0 的 provider | Logto、Keycloak、Auth0、GitHub Enterprise OIDC 等标准实现 | `issuer` 必须能被 API 服务端访问；回调地址必须等于平台展示的 callback URL。 |
| PostgreSQL | PostgreSQL wire protocol、GORM、golang-migrate | PostgreSQL `14 ~ 18` | `17`，与 compose/Helm 默认一致 | 项目不支持 SQLite；生产环境建议启用备份和连接池限制。 |
| Redis | Redis 单实例、go-redis、Asynq 队列 | Redis `7.x ~ 8.x` | `8`，与 compose/Helm 默认一致 | 当前配置模型是单地址 Redis；Redis Cluster/Sentinel 不是第一阶段支持目标。 |
| BuildKit | `moby/buildkit:*rootless`、`buildctl-daemonless.sh`、`dockerfile.v0` frontend | 重点验收 `v0.24.x-rootless`；替换为 `v0.20+ rootless` 需自行 smoke test | `moby/buildkit:v0.24.0-rootless` | 构建 Job 使用 rootless BuildKit，不挂载宿主机 Docker socket。 |
| Prometheus | Prometheus text exposition format，抓取 API 独立 `/metrics` listener；Worker/Agent 使用 OTLP | Prometheus `2.40+` 或 `3.x` | 当前稳定版 | API 入口用于兼容抓取；完整平台指标由 Collector 汇入统一后端。 |
| Grafana | Dashboard JSON、运营面板 iframe 嵌入地址 | Grafana `9.x ~ 12.x` | 当前稳定版 | iframe 嵌入需要 Grafana 侧开启 `allow_embedding`，并自行处理认证和同源策略。 |
| SMTP | SMTP/STARTTLS 发送通知 | 支持标准 SMTP 的服务 | 企业邮箱、云厂商 SMTP 或自建 SMTP 当前稳定版 | SMTP 属于通知适配器；凭据必须按 Secret 处理。 |
| 自由 Webhook 通知 | 自定义方法、URL、JSON body 模板 | HTTP/HTTPS endpoint | 目标平台当前 Webhook API | 飞书、企业微信机器人等可以由 Webhook 模板快照生成；目标平台的验签和限流由对应适配器或用户配置负责。 |

## 升级验收建议

升级外部组件后，至少走一遍以下冒烟测试，不要只以“连接成功”作为验收结果：

1. GitHub/Gitea：OAuth 登录、仓库列表、分支列表、读取 Dockerfile、创建或重配 Webhook。
2. Registry：连接测试、搜索仓库、读取 tag、构建后推送镜像、运行集群拉取镜像。
3. Kubernetes/K3s：测试集群连接、创建构建 Job、创建 Deployment/Service、读取 Pod 日志、Web Console exec。
4. Gateway API：创建访问入口后确认 Gateway Accepted/Programmed、HTTPRoute Accepted/ResolvedRefs/Programmed。
5. OIDC：完成登录、绑定外部身份、校验 callback URL 和 issuer。
6. Prometheus/Grafana：抓取 API 兼容 metrics，并确认 API、Worker、Agent OTLP Metrics 已进入统一后端；导入 dashboard JSON 后确认 iframe 地址可访问。

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
