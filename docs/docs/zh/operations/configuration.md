# 配置与连接

平台配置分成两类：给用户看的公开配置，以及连接外部系统的后台配置。

## 公开配置

公开配置会影响控制台展示，例如：

- 站点标题。
- Logo 和 Favicon。
- 登录页副标题。
- 主题和语言偏好。

这些内容可以放心展示给前端，但不要放 Token、密码或内部地址。

## Git Provider

Git Provider 用来连接 GitHub 或 Gitea。配置完成后，用户可以绑定仓库、接收 Webhook，并按分支或标签触发构建。

删除 Git Provider 时，平台会同步删除属于该 Provider 的 Git 凭据。删除前请确认相关仓库绑定和构建链路不再依赖这些凭据。

如果你只是想先跑通部署，可以暂时跳过 Git Provider，直接使用已有镜像。

## 镜像站

镜像站用于拉取或推送镜像。常见选择包括 Harbor、Gitea Registry 和 DockerHub。

删除镜像站时，平台会同步删除属于该镜像站的凭据。删除前请确认部署配置、构建任务或运行集群拉取镜像不再依赖这些凭据。

需要自动构建时，部署配置会使用镜像站推送凭据；只部署已有镜像时，重点确认运行集群能拉取目标镜像。

镜像站凭据可以配置“镜像仓库模板”和“镜像 Tag 模板”。它们只用于创建部署配置时填充默认推送位置；部署配置保存后会把仓库和 Tag 保存为快照，不会继续跟随凭据模板变化。常用写法例如仓库模板 `devopsns/{project}-{app}-{stage}`，Tag 模板 `{projectSlug}-{appSlug}-{stage}`，新建部署配置时会得到类似 `devopsns/blog-api-prod:blog-api-prod` 的默认镜像引用。

仓库模板和 Tag 模板都只渲染创建部署配置时已知的静态变量：`{registryNamespace}`、`{project}`、`{projectSlug}`、`{app}`、`{appSlug}`、`{stage}`、`{target}`。如果 Tag 模板使用 `{commit}`、`{branch}` 等构建时变量，创建部署配置时会回落为 `latest`，避免后续构建被凭据模板隐式改写。

## 运行集群

运行集群是发布目标。平台会把 Release 转换成 Kubernetes 资源，并把状态、日志和诊断信息展示回来。

运行集群也维护访问入口的默认域名后缀、外层访问协议、外层访问端口和 Gateway API 默认值。访问入口按部署配置所在集群生成默认域名、补全短域名前缀，并返回控制台访问链接；因此多个集群可以分别接入不同 GatewayClass、共享 Gateway 或不同根域名。

网关配置分为“对外展示”和“集群内部”两层：

- 外层访问协议和外层访问端口只影响控制台生成的访问 URL。HTTP `80` 和 HTTPS `443` 不会显示在访问地址里；如果配置为非标端口，访问地址会显示 `:端口`。它们不会修改 Kubernetes Gateway listener，也不会触发证书申请。
- Gateway listener 名称和端口是集群内部 Gateway/Controller 承接流量的配置，默认 `web:8080` 和 `websecure:8443`。普通项目用户不需要选择端口或 listener。

访问入口默认绑定哪个 listener 由外部 TLS 模式决定：选择“Gateway 终止 TLS”时绑定 HTTPS listener，例如 `websecure`；选择“上游代理已终止 TLS”时绑定 HTTP listener，例如 `web`，因为进入 Gateway 的流量已经是明文 HTTP。HTTPS listener 始终按 HTTPS/TLS 入口生成，用于匹配 Traefik `websecure` 这类启用 TLS 的 entryPoint。

如果外层 Nginx/CDN/负载均衡器已经占用宿主机 `80/443`，可以让它转发到集群 Gateway 的内部端口：上游已终止 TLS 时转到 HTTP listener，例如 `8080`；需要 Gateway 终止 TLS 时转到 HTTPS listener，例如 `8443`。平台生成的访问地址仍按运行集群的外层访问协议和外层访问端口展示。

集群资源页会分页展示平台管理的命名空间、工作负载、服务、配置、密钥和存储资源；分页总数只统计当前用户有权查看的资源。工作负载页以 Deployment 为主行，展开后展示该 Deployment 下的 Pod，Pod 子行不参与分页计数。

如果 API 或 worker 在容器里运行，kubeconfig 里的地址必须能从容器访问，不要直接使用宿主机专用的 `127.0.0.1`。

运行集群也承担 Kubernetes 构建 Job。小团队默认每个运行集群最多同时运行 4 个构建 Job；项目空间默认最多同时运行 2 个构建。超过额度时，新构建会保持排队并自动重试，不会立刻标记为失败。

## 密钥

Secret、Token 和 Registry Credential 不会明文回显。编辑时留空表示“不修改已有值”，需要替换时输入新值并保存。
