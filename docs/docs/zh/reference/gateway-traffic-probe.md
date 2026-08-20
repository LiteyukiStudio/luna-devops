# 网关流量探针

`Luna Gateway Traffic Probe` 是部署在目标运行集群内的可选采集组件，用于统计平台访问入口的响应出站流量，供账单按 `gateway.egress_gib` 计量。不需要按流量计费的集群可以不安装。

## 安装

在应用市场搜索「Luna Gateway Traffic Probe」，点击安装后：

1. 选择目标运行集群。
2. 按需修改镜像地址；默认使用 `liteyukistudio/devops-gateway-traffic-probe:nightly`，如使用 Harbor、DockerHub 代理或私有镜像，替换为完整镜像引用。平台始终使用 `Always` 拉取策略，确保每次部署都解析最新镜像。
3. 按需勾选「由平台创建 ServiceAccount 与 RBAC 权限」；勾选后平台会为探针创建专用 ServiceAccount 并授予读取 Gateway API 路由的权限，不勾选则使用命名空间默认账号，由你自行配置 RBAC。
4. 填写 `API_BASE_URL` 和可选的 `TRAEFIK_METRICS_URL`；安装时平台生成专用上报令牌并写入密钥存储，探针通过环境变量 `REPORT_TOKEN` 引用，不回显明文。

安装后探针作为普通应用部署到目标集群的系统命名空间，可在项目空间的应用列表中查看部署状态、编辑环境变量或重新部署。

## 工作原理

探针按固定间隔（默认 1 分钟）执行一个采集周期：

1. 通过集群内 ServiceAccount 读取 Gateway API，列出该集群的 HTTPRoute，得到需要统计的平台路由。
2. 抓取 Traefik 的 Prometheus 指标端点，按 router/service 标签把计数器匹配到平台路由。
3. 对每条路由的响应字节数和请求数计数器做差分，得到本周期的流量窗口；计数器因重启回退时按当前值计。
4. 把有流量的窗口上报到平台 API，平台结算为用量。已上报过的窗口幂等去重，重复上报返回 `already_settled`，不会重复计费。

只统计平台管理的 HTTPRoute 产生的响应出站流量，集群内服务互访不计入。

## 上报与鉴权

- 安装探针时，平台生成一个随机上报令牌（形如 `lyd_probe_<hex>`），真实值只在安装时下发到集群，作为 Secret 注入探针的 `REPORT_TOKEN`。平台侧只保存该令牌的加密副本与 SHA-256 哈希。
- 探针每次上报都携带 `Authorization: Bearer <REPORT_TOKEN>`。平台用哈希反查安装记录完成鉴权，**不存储明文令牌**，日志与遥测也不会记录令牌。
- 上报流量前，平台还会校验目标路由确实属于该探针所在的运行集群，防止一个集群的令牌上报其他集群的路由。
- 上报窗口包含路由 ID、响应字节数、请求数和时间范围；不包含请求内容、来源 IP 或用户信息。

## 配置项

探针通过环境变量配置，安装时已写入合理默认值，通常只需确认以下几项：

需要调整指标地址或采集间隔时，可在探针应用的部署配置中编辑对应环境变量。不要修改 `REPORT_TOKEN` 或运行集群标识；如未在安装时勾选平台创建 RBAC，也不要修改 ServiceAccount 配置。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `API_BASE_URL` | 安装时注入 | 平台 API 地址，探针必须能访问 |
| `REPORT_TOKEN` | 安装时注入 | 上报令牌，来自平台 Secret，不要手改 |
| `RUNTIME_CLUSTER_ID` | 安装时注入 | 探针所在运行集群 ID |
| `TRAEFIK_METRICS_URL` | `http://traefik.<gateway-namespace>.svc.cluster.local:9100/metrics` | Traefik Prometheus 指标地址 |
| `GATEWAY_NAMESPACE` | `kube-system` | Traefik 所在命名空间，用于拼接默认指标地址 |
| `SCRAPE_INTERVAL` | `1m` | 采集间隔，最小 10s |
| `ROUTE_REFRESH_INTERVAL` | `1m` | 路由刷新间隔，最小 10s |
| `PROBE_ADDR` | `:9090` | 探针自身 `/healthz` 与 `/metrics` 监听地址 |

## 前置条件

- Traefik 已启用 Prometheus 指标，且指标包含 router 和 service 标签；缺少标签时探针无法把流量匹配到平台路由。
- 探针所在集群能访问平台 API 的 `API_BASE_URL`。

## 排障

探针自身暴露 `/healthz` 和 `/metrics`（`PROBE_ADDR`），可用于检查路由数、最近一次抓取/上报时间和当前错误。

账单页的流量采集状态含义：

- **未部署**：目标集群尚未安装探针。
- **等待上报**：探针已就绪，但平台尚未收到有效流量数据。检查探针日志、Traefik 指标标签，以及探针到平台 API 的网络连通性。
- **不可用**：平台当前无法读取目标集群状态。

采集异常时，先确认探针日志中的鉴权或网络错误，再核对 Traefik 指标是否含 router/service 标签。修改生产网关配置可能造成短暂流量中断，请在低峰期操作并保留回滚方式。
