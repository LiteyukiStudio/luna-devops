# 功能地图

Liteyuki DevOps 把代码、镜像、集群和访问入口串成一条交付路径。你不需要一开始理解每个底层组件，只需要知道平台里这些模块各自负责什么。

想直接照着做一次，可以看 [部署上线一个 Web 项目](/operations/deploy-web-project)。那一页用 `snowykami/neo-blog` 作为示例，从项目空间、应用、部署配置、构建到访问入口完整走一遍。

## 项目空间

项目空间是团队和资源的边界。成员、应用、部署配置、构建记录、发布记录和访问入口都归属到某个项目空间。

添加项目空间成员时，Owner/Admin 需要先按用户名或邮箱搜索平台用户，再从候选列表中选择一个或多个用户添加；平台不会把自由输入的邮箱文本直接当作成员创建。

常见用法：

- 一个产品一个项目空间。
- 一个小团队一个项目空间。
- 一个客户或演示环境一个项目空间。

## 应用

应用是一个可部署服务。一个仓库可以对应多个应用，例如 monorepo 里的 API、Web、Worker。

应用主要保存服务的基本信息，真正的构建方式、镜像、环境变量和发布策略放在部署配置里。

应用概览会按部署配置汇总运行规格，展示副本数、CPU、内存和已启用的数据卷容量，便于快速判断当前应用的资源占用。

## 部署配置

部署配置回答“这个应用要怎么交付”：

- 从仓库构建，还是直接使用已有镜像。
- 发布到哪个环境。
- 部署配置所属阶段，例如开发、测试、预发或生产。
- 使用哪个镜像站。
- 服务监听哪些端口。部署配置可以维护多个服务端口，第一个端口作为默认端口；例如业务 HTTP 使用 `8080`，Prometheus 指标使用 `9001`。
- 构建规格和超时时间。默认构建超时为 30 分钟，可以在部署配置或手动触发构建时临时调整。
- 构建成功后是否自动发布。

部署配置的高级 Kubernetes 配置默认折叠，只在应用镜像需要特殊运行条件时使用。当前支持：

- 容器启动：覆盖 `command` / `args`，设置 `imagePullPolicy`，配置 readiness、liveness 和 startup Probe。
- 工作负载：默认使用 Deployment；需要稳定 Pod 名称、顺序滚动或有状态语义时，可以在高级区切换为 StatefulSet。平台会同步处理发布渲染、HPA 指向、运行态检查、重启和资源清理。
- 生命周期：通过 Kubernetes Lifecycle JSON 配置 `postStart` 和 `preStop`。
- 初始化与辅助容器：通过受控 Container JSON 数组配置 initContainer 和 sidecar。平台会裁剪 `hostPort`、外部 `envFrom`、外部 Secret 引用和提权能力等危险字段，并统一注入当前部署配置对应的 ConfigMap/Secret。
- 资源上限：在 CPU / 内存 request 之外按需设置 limit；留空时不设置 limit。
- 自动伸缩：启用 HPA 后，平台会创建 `autoscaling/v2 HorizontalPodAutoscaler`，按 CPU 或内存平均利用率目标调整副本数，并可通过 HPA behavior JSON 控制扩缩容速度和稳定窗口。运行集群需要 metrics-server 或等价指标 API。
- 安全上下文：设置 `runAsUser`、`runAsGroup`、`fsGroup`、`fsGroupChangePolicy`、只读根文件系统、`allowPrivilegeEscalation` 和 capabilities。像 OpenList 这类镜像如果需要固定 UID/GID 写入数据目录，可以在这里配置。
- 调度：设置 `nodeSelector`、`tolerations`、基础 `affinity`、`topologySpreadConstraints` 和 `priorityClassName`。
- Service 与存储：Service 默认仍是 ClusterIP，高级区可设置 Service 类型、annotations、`appProtocol`、会话亲和和外部流量策略；开启运行数据后可设置 PVC `storageClassName`、`accessMode` 和 `volumeMode`。数据卷来源支持平台托管 PVC、已有 PVC 和临时 `emptyDir`。

复杂结构字段使用 Kubernetes 原生 JSON，例如 Probe、Toleration、Affinity 和 TopologySpreadConstraint。简单键值字段支持 JSON 对象或 `KEY=VALUE` 多行文本。

PVC 的 `storageClassName` 和 `accessMode` 只会在首次创建数据卷时写入；已有 PVC 后续只支持扩容容量，不会自动迁移或重建存储卷。
`emptyDir` 数据随 Pod 生命周期销毁，不参与平台数据导出；已有 PVC 可以导出，但平台不会管理它的容量和生命周期。
当前 StatefulSet 主路径复用平台托管 PVC / 已有 PVC / emptyDir 模型，不自动生成 `volumeClaimTemplates`；需要每个副本独立持久卷的场景会在后续高级编排阶段单独设计。

仓库 Webhook 绑定在应用仓库上。Git 平台推送 push/tag 事件后，平台会在该应用下查找使用同一仓库绑定、已启用且未删除的部署配置，再按分支匹配和标签匹配规则创建构建记录。部署配置本身不单独创建外部 Webhook，这样同一个仓库事件只进入平台一次。

选择仓库 Dockerfile 时，平台会尝试读取 Dockerfile 中的 `EXPOSE` 指令并自动填充服务端口列表；如果服务有多个 HTTP 端口，可以在部署配置中继续添加。创建访问入口时需要从该部署配置暴露的端口中选择目标端口。

删除构建变量或运行配置集时，平台会从仍引用它们的部署配置中移除对应引用，避免部署配置继续指向不可维护的配置项。

部署配置引用项目空间公共配置时支持两种策略：“跟随引用”会在公共配置更新后提示重新部署，下一次发布会读取最新公共配置；“使用快照”会在保存部署配置时冻结当前公共配置内容，后续公共配置更新不会影响该部署配置。两种策略下 Secret 都仍由平台密钥存储保存，部署配置只保存密钥引用或快照中的密钥引用。

项目空间钩子只负责维护可复用脚本库，真正执行由部署配置里的“部署钩子”绑定决定。部署配置可以把同一个项目钩子绑定到构建前后、镜像推送前后、部署前后等阶段，并在配置内调整执行顺序。`部署前` 钩子会在运行配置 ConfigMap/Secret 写入后、应用 Deployment 滚动发布前执行，适合处理数据库 migration、seed 或必须在应用容器启动前完成的一次性命令。

部署阶段 Hook 会以 Kubernetes Job 运行。平台会保存 Hook 运行记录和日志；集群里的 Hook Job/Pod 只短期保留用于排查现场：成功记录默认 5 分钟后清理，失败记录默认 24 小时后清理。

删除部署配置时，平台会先删除绑定到该部署配置的访问入口，再清理对应的 Kubernetes 工作负载、Service 和可选数据卷，避免入口继续指向不存在的服务。

访问域名创建时默认启用；如果只是想暂时停止公网入口，可以关闭访问，平台会保留域名配置并撤销对应 HTTPRoute，之后重新启用即可恢复下发。

## 构建与发布

构建会生成镜像，发布会把镜像部署到运行集群。

每次创建新 Release 时，平台都会更新 Kubernetes Pod Template 的发布指纹，确保即使目标镜像 tag 没变也会触发滚动更新。默认情况下，配置变化只会重启 Pod，不会强制重新拉取镜像；当 Release 来自新的构建产物且镜像 tag 没变时，平台才会临时使用 `imagePullPolicy: Always`，避免固定 tag 复用节点缓存导致旧版本继续运行。

如果你确认远端镜像内容已经变化，但镜像 tag 没变，可以在部署配置的操作菜单中选择“拉取最新镜像部署”。这会创建一次新的 Release，并在本次 rollout 中强制重新拉取镜像。

平台创建 Kubernetes Deployment 时，selector 只用于识别当前部署配置对应的工作负载，并在后续发布中保持稳定；项目空间、应用、环境和 Release 等归属信息会写入资源 labels 或 Pod Template annotations，不会通过修改 selector 触发发布。这样可以避免 Kubernetes `spec.selector` 不可变导致的更新失败。

第一次体验建议先用已有镜像创建 Release；等访问入口和运行状态确认没问题，再接入 Git Provider 和自动构建。

应用部署列表会通过 SSE 每秒刷新运行指标。指标来源是 Kubernetes 标准 `metrics.k8s.io` Pod Metrics API，因此运行集群需要安装 metrics-server。CPU 百分比和内存占用按当前用量除以“环境规格 × 副本数”计算；如果集群没有暴露指标，页面会显示暂无指标。

## 访问入口

访问入口负责把域名、路径、TLS 和后端服务连接起来。创建后，平台会展示下发状态和检查结果，方便你确认服务是否真的能访问。

域名后缀来自部署配置所属运行集群。管理员可以在集群上维护多个可用后缀，创建访问入口时只选择其中一个；短域名前缀和留空自动生成都会使用所选后缀，完整自定义域名仍可直接填写。

运行集群里的“外层访问协议”和“外层访问端口”只影响控制台展示和打开访问入口时使用的 URL。如果外层 CDN 或反向代理已经提供 HTTPS，可以把外层访问协议设为 `https`，同时外部 TLS 模式选择“上游代理已终止 TLS”，平台会让 HTTPRoute 绑定内部 HTTP listener，不会因此额外申请集群内证书。

如果 HTTPS 由集群 Gateway 自己终止，管理员需要把运行集群的外部 TLS 模式设为“Gateway 终止 TLS”，并配置已有 Kubernetes TLS Secret。平台会在下发访问入口时确保共享 Gateway 的 HTTPS listener 引用该 Secret，访问入口对应的 HTTPRoute 会默认绑定 HTTPS listener。

访问入口的“HTTP Challenge 证书”模式依赖运行集群 cert-manager 配置。平台会创建 Certificate，并把 Certificate 生成的 Secret 引用到共享 Gateway HTTPS listener。Worker 会周期同步 cert-manager Ready 条件、失败信息和 `notAfter`：应用“访问”列表在 TLS 模式旁展示无、申请中、已启用、失败或已过期，悬停状态可查看失败原因、过期时间和实际引用的 Issuer。

平台只引用运行集群配置的 `Issuer` 或 `ClusterIssuer`，不会自行创建 ACME 账号。默认名称 `letsencrypt-http01` 只是 Issuer 资源名；实际 CA、ACME 邮箱、账号 Secret 和 HTTP-01 solver 均由该 Issuer 的 `spec.acme` 配置决定。

如果运行集群启用了 DNS-01 通配符证书，平台会优先把通配符证书 Secret 一起挂到 HTTPS listener。此模式适合外层网关转发到集群内部端口、没有公网 HTTP-01 入口或希望同一域名后缀复用一张证书的场景。

平台访问入口底层使用 Kubernetes Gateway API：运行集群维护一个平台管理的 `Gateway`，每条访问入口在项目命名空间下生成一个 `HTTPRoute` 并转发到部署配置对应的 `Service`。集群需要先安装 Gateway API CRD；Traefik 集群需要启用 `--providers.kubernetesGateway`。

创建或启用访问入口时，平台会先检查部署配置对应的 Kubernetes `Service` 和端口是否存在。若管理员手动删除了 Service，平台会提示先重新发布部署配置来恢复运行态资源，而不会在访问入口链路里自动创建 Service。这样可以避免访问入口使用过期的部署配置偷偷修复运行态漂移。

运行集群可以维护默认 Gateway 配置，包括 Controller 类型、GatewayClass、Gateway 名称/命名空间、外部 TLS 模式、转发头策略、可信代理 CIDR 以及默认请求/响应头。创建访问入口时，基础配置默认只展示部署配置、域名、路径、服务端口和 TLS；需要细调网关行为时再展开高级配置，覆盖单条路由的 Parent Gateway、路径匹配方式、请求/响应头、URL rewrite、redirect 和后端权重。

如果链路是 `CDN HTTPS -> Nginx HTTP -> Traefik HTTP -> Pod`，推荐在 Traefik entryPoint 配置 `forwardedHeaders.trustedIPs` 信任上游代理，并让上游传递 `X-Forwarded-Proto=https`。对于 Logto/OIDC 这类依赖外部 URL 的应用，如果后端看到的是 `http`，可能会生成错误 issuer 或 redirect URL；可在运行集群选择 upstream TLS + overwrite 作为兜底，让平台通过 HTTPRoute RequestHeaderModifier 注入 `X-Forwarded-Proto=https` 和 `X-Forwarded-Port=443`。
