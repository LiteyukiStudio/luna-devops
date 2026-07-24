# 平台功能怎么配合

Luna DevOps 把代码、镜像、集群和访问入口连成一条交付路径。第一次使用时，只需要先理解这条主线：

```text
项目空间 -> 应用 -> 部署配置 -> 构建 -> 发布 -> 访问入口
```

想直接照着做一次，可以看 [部署上线一个 Web 项目](/operations/deploy-web-project)。那一页用 `snowykami/neo-blog` 作为示例，从创建项目空间一直走到公网访问。

## 先看主流程

### 1. 项目空间

项目空间是团队和资源的边界。成员、应用、部署配置、构建记录、发布记录和访问入口都归属到某个项目空间。

常见划分方式：

- 一个产品一个项目空间。
- 一个小团队一个项目空间。
- 一个客户或演示环境一个项目空间。

添加成员时，Owner/Admin 需要搜索平台用户并从候选列表选择；平台不会把随手输入的邮箱直接创建成成员。

### 2. 应用

应用代表一个可部署服务。一个仓库可以拆成多个应用，例如 monorepo 里的 API、Web、Worker。

应用本身只保存服务基本信息。构建来源、镜像、环境变量、资源规格和发布策略都放在部署配置里。

应用详情里的“拓扑”页会实时读取当前应用的 Kubernetes 资源关系。默认只展示 `Gateway -> HTTPRoute -> Service -> Deployment/StatefulSet -> Pod` 主链路；打开“依赖资源”后，还会显示 HPA、ConfigMap、Secret 和 PVC。拓扑不保存到数据库，每次打开或刷新都会重新从运行集群计算，因此也能反映集群外手动删除资源后的实际状态。Secret 节点只返回名称和状态，不读取或展示内容。

项目空间还可以维护应用之间的逻辑关系。只有项目中已经存在关系时，普通成员才会看到项目空间“拓扑”页；Owner/Admin 可以从高级入口添加第一条关系，避免不使用这项能力的团队被额外配置打扰。

关系分为两种：

- **服务引用**会影响源部署配置。平台根据目标部署配置的 Kubernetes Service 生成稳定的集群内地址，并在源服务下一次发布时注入环境变量。
- **仅记录关系**只用于说明调用、读写、发布或消费关系，不会修改部署配置，也不会创建 Kubernetes 资源。

服务引用首版只支持同一项目空间、同一运行集群，并且需要明确选择源和目标部署配置。平台不会把密码或 Token 拼进地址；数据库凭据等敏感信息仍应使用项目空间 Secret。保存时可以留在拓扑页，也可以直接前往源部署配置的发布页选择镜像并完成二次确认。未创建新发布前，运行中的 Pod 不会发生变化。

诊断会检查目标 Service、端口、EndpointSlice 和可能影响通信的 NetworkPolicy。它只读取 Kubernetes 元数据，不主动连接业务端口，也不会读取 Secret 内容。被其他服务引用的应用或部署配置不能直接删除，需要先停用或删除对应引用。

### 3. 部署配置

部署配置决定这个应用怎么构建、怎么运行、发布到哪里。

第一次创建时，优先填这些内容：

- 构建来源：从仓库构建，或直接使用已有镜像。
- 环境阶段：开发、测试、预发或生产。
- 镜像站：构建产物推到哪里。
- 服务端口：可以维护多个端口，第一个端口作为默认端口。
- 运行配置：副本、CPU、内存、项目空间公共配置和部署级覆盖。
- 构建策略：超时时间，以及构建成功后是否自动发布。

部署配置表单按“基础部署、构建设置、运行配置、发布策略、部署钩子、运行数据、高级 Kubernetes 配置”组织。仓库来源才会展示构建设置；运行配置紧跟构建设置，并集中维护运行资源和配置注入。各区块的长说明收纳在标题旁的说明按钮中，保持表单紧凑。

如果服务刚接入，建议先用已有镜像创建一次 Release。确认 Pod 能运行、访问入口能打开后，再接入 Git Provider 和自动构建，排查会轻松很多。

### 4. 构建与发布

构建会生成镜像，发布会把镜像部署到运行集群。

每次创建 Release，平台都会更新 Kubernetes Pod Template 的发布指纹，所以即使镜像 tag 没变，也会触发滚动更新。默认情况下，配置变化只重启 Pod，不强制重新拉镜像；如果 Release 来自新的构建产物且 tag 没变，平台会临时使用 `imagePullPolicy: Always`。

如果你确认远端镜像内容已经变化，但 tag 没变，可以在部署配置操作菜单里选择“拉取最新镜像部署”。

## 部署配置里的高级项

大多数服务保持默认值即可。只有镜像或运行环境有特殊要求时，再展开高级配置。

### Kubernetes 运行参数

| 场景 | 可以调整什么 |
| --- | --- |
| 容器启动不走默认入口 | `command`、`args`、`imagePullPolicy` |
| 需要健康检查 | readiness、liveness、startup Probe |
| 需要有状态语义 | 从 Deployment 切换为 StatefulSet |
| 需要启动前后动作 | Kubernetes Lifecycle `postStart` / `preStop` |
| 需要辅助容器 | initContainer 和 sidecar |
| 需要限制资源上限 | CPU / 内存 limit |
| 需要自动伸缩 | HPA CPU/内存目标和 behavior |
| 需要固定 UID/GID 或更强隔离 | securityContext、只读根文件系统、capabilities |
| 需要指定节点或容忍污点 | nodeSelector、tolerations、affinity、topologySpreadConstraints |
| 需要调整 Service 或存储 | Service 类型、annotations、PVC storageClass/accessMode/volumeMode |

复杂结构字段使用 Kubernetes 原生 JSON，例如 Probe、Toleration、Affinity 和 TopologySpreadConstraint。简单键值字段支持 JSON 对象或 `KEY=VALUE` 多行文本。

### 存储注意事项

PVC 的 `storageClassName` 和 `accessMode` 只会在首次创建数据卷时写入。已有 PVC 后续只支持扩容容量，不会自动迁移或重建。

`emptyDir` 数据会随 Pod 销毁，不参与平台数据导出。已有 PVC 可以导出，但平台不会管理它的容量和生命周期。

### 配置、变量和钩子

删除构建变量或运行配置集时，平台会从仍引用它们的部署配置中移除对应引用，避免继续指向不可维护的配置项。

部署配置引用项目空间公共配置时有两种策略：

- 跟随引用：公共配置更新后，下一次发布读取最新内容。
- 使用快照：保存部署配置时冻结当前内容，后续公共配置变化不影响它。

项目空间钩子只是可复用脚本库。真正执行时，需要在部署配置的“部署钩子”里绑定到构建前后、镜像推送前后、部署前后等阶段。部署前钩子适合做数据库 migration、seed 或一次性修复命令。

Hook 会以 Kubernetes Job 运行。平台保存运行记录和日志；集群里的 Job/Pod 只短期保留用于排查，成功默认 5 分钟后清理，失败默认 24 小时后清理。

## Web Console 与数据导出

### Web Console

Web Console 的项目空间总开关默认开启，项目 Owner/Admin 可以关闭。部署配置只能继承项目空间设置或单独关闭，不能绕过项目空间总开关。

这个开关只决定是否允许进入终端，不会放宽权限或 MFA：

- 应用发布页终端：项目 Owner、Admin、Developer 可用。
- 集群资源页 Pod 终端：仅平台管理员可用。

运行命令审计只保存命令摘要、长度、容器和退出码，不记录原始命令正文。交互终端审计保存连接目标和结果，不记录终端输入输出。

### 数据导出

部署配置的数据导出只支持浏览器 cookie 会话，并且需要项目 Owner/Admin。个人访问令牌即使有数据导出 scope，也不能直接下载运行数据。

下载前平台会签发 60 秒一次性票据，并绑定当前用户、session、项目空间、应用和部署配置。生产多副本通过 Redis 保存票据哈希；Redis 不可用时拒绝导出。

导出只包含平台托管 PVC 或已有 PVC，`emptyDir` 不参与导出。

## 访问入口

访问入口负责把域名、路径、TLS 和后端服务连接起来。创建后，平台会展示下发状态和检查结果，方便确认服务是否真的能访问。

### 域名和端口

域名后缀来自部署配置所属运行集群。管理员可以在集群上维护多个后缀，创建访问入口时选择其中一个；短域名前缀和自动生成域名都会使用所选后缀。

创建或启用访问入口时，平台会先检查部署配置对应的 Kubernetes Service 和端口是否存在。如果 Service 被手动删除，需要先重新发布部署配置恢复运行态资源。

### TLS 怎么选

| 场景 | 推荐模式 |
| --- | --- |
| CDN 或外层反向代理已经处理 HTTPS | 外层访问协议设为 `https`，外部 TLS 模式选择“上游代理已终止 TLS” |
| 集群 Gateway 自己终止 HTTPS | 外部 TLS 模式选择“Gateway 终止 TLS”，并配置已有 Kubernetes TLS Secret |
| 使用 cert-manager HTTP-01 | 选择 HTTP Challenge 证书模式，并提前准备好 Issuer/ClusterIssuer |
| 使用 DNS-01 通配符证书 | 在运行集群配置通配符证书 Secret，让同一后缀复用证书 |

平台只引用运行集群配置的 `Issuer` 或 `ClusterIssuer`，不会自行创建 ACME 账号。默认名称 `letsencrypt-http01` 只是 Issuer 资源名，实际 CA、邮箱、账号 Secret 和 solver 都由该 Issuer 配置决定。

### Gateway API

访问入口底层使用 Kubernetes Gateway API：运行集群维护一个平台管理的 `Gateway`，每条访问入口在项目命名空间下生成一个 `HTTPRoute`，并转发到部署配置对应的 `Service`。

集群需要先安装 Gateway API CRD。Traefik 集群还需要启用 `--providers.kubernetesGateway`。

### 转发头和真实协议

如果链路是：

```text
CDN HTTPS -> Nginx HTTP -> Traefik HTTP -> Pod
```

推荐在 Traefik entryPoint 配置 `forwardedHeaders.trustedIPs` 信任上游代理，并让上游传递 `X-Forwarded-Proto=https`。

对于 Logto/OIDC 这类依赖外部 URL 的应用，如果后端看到的是 `http`，可能生成错误 issuer 或 redirect URL。必要时可以在运行集群选择 upstream TLS + overwrite，让平台通过 HTTPRoute RequestHeaderModifier 注入 `X-Forwarded-Proto=https` 和 `X-Forwarded-Port=443`。
