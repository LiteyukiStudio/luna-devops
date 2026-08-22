# 网关流量探针

Luna Gateway Traffic Probe 是可选平台组件，用于统计 Luna DevOps 管理的 HTTPRoute 响应出站流量，并按 `gateway.egress_gib` 计量。不按流量计费时无需安装。

## 安装与配置

在应用市场搜索“Luna Gateway Traffic Probe”，选择安装：

1. 选择目标运行集群。
2. 确认镜像地址；默认镜像适合测试，生产环境建议固定版本 Tag。
3. 选择是否由平台创建专用 ServiceAccount 和 RBAC。若不选择，需要自行授予读取 Gateway API 路由的权限。
4. 填写平台 `API_BASE_URL`，并确认探针所在集群可以访问。
5. 如果 Traefik 指标地址不是默认值，填写 `TRAEFIK_METRICS_URL`。

安装时平台会生成专用 `REPORT_TOKEN` 并写入密钥存储，不回显明文。不要手动修改该 Token 或运行集群 ID。

## 必要配置

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `API_BASE_URL` | 安装时填写 | 设置探针上报的平台 API；填写集群可访问的 HTTP(S) URL。 |
| `TRAEFIK_METRICS_URL` | 集群内 Traefik `/metrics` | 设置 Traefik 指标来源；填写 Prometheus metrics HTTP(S) URL。 |
| `GATEWAY_NAMESPACE` | `kube-system` | 限定探针读取 Traefik 的命名空间；填写 Kubernetes Namespace 名称。 |
| `SCRAPE_INTERVAL` | `1m` | 设置指标采集间隔；填写不小于 `10s` 的 Go duration。 |
| `ROUTE_REFRESH_INTERVAL` | `1m` | 设置路由刷新间隔；填写不小于 `10s` 的 Go duration。 |
| `PROBE_ADDR` | `:9090` | 设置探针健康检查与指标监听地址；填写 `IP:端口` 或 `:端口`。 |

前置条件是 Traefik 已开启 Prometheus 指标，且指标包含 router 和 service 标签。安装后在应用部署页检查工作负载，并通过探针 `/healthz`、日志和账单页的采集状态验证。

## 工作机制

探针定期读取集群中的 HTTPRoute，把 Traefik 的响应字节与请求计数器匹配到平台路由，再将本周期增量上报给 Luna DevOps。平台会校验路由所属集群并对重复窗口幂等去重；上报不包含请求正文、来源 IP 或用户信息。
