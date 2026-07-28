# 账单与费用分析

账单页回答两个问题：账户还剩多少额度，以及费用具体花在了哪些项目、应用和部署配置上。你可以在这里查看余额、消费概览、费用分析和每一笔账单流水。

项目空间负责产生费用，真正扣款的是“计费归属账户”的用户钱包。归属人发生变化后，新费用由新的归属人承担，过去的账单流水不会跟着迁移。

项目空间概览页会展示“计费归属账户”，包含头像、名称和邮箱，用于确认当前项目空间后续扣费关联到哪个用户账户。

旧发布记录缺少部署配置引用时，平台只在能唯一匹配部署配置的场景下回填或继续计费，避免旧应用漏账。

旧部署配置缺少删除状态时，平台会按未删除资源规范化为可计费的 active 状态。

页面顶部的余额是当前用户账户的总览；周期花费、今日花费和待结算金额会跟随项目空间筛选范围统计。工具栏可以同时选择账单周期、用户账户和项目空间。周期预设包含本周、近 7 天、本月、近 30 天、本年和去年，也可以用日期选择器自定义范围。费用分析、账单流水、用量记录和周期分类消耗都会使用同一时间范围。

普通用户只能查看自己的账单。平台管理员进入页面时也先看到自己的数据，需要时再通过用户选择器切换到全部用户或指定用户；项目空间筛选会随当前用户范围一起更新。

平台管理员还可以在站点设置里配置现实货币单位和换算比例。账单页顶部概览会在 credits 后显示折算金额，例如 `1,012.24 Credits (1.01 CNY)`；这只是展示换算，底层结算仍使用 credits。

平台管理员进入用户管理时，用户列表会展示每个用户的钱包余额，便于快速判断账号余额状态；缺少钱包记录的用户按 0 credits 展示。

## 费用分析

费用分析按“项目空间 / 应用 / 部署配置”聚合已结算用量，展示总费用以及构建、运行、存储、访问和其他分类费用。

CPU、内存和存储按部署配置窗口结算；网关流量按访问路由窗口结算，并在费用分析中归回路由关联的部署配置；构建按 BuildRun 结算，并归回构建所属部署配置。

如果一条用量没有关联到应用或部署配置，页面会归到“未关联应用”或“未关联部署配置”。账单流水仍保留每一笔余额变动，适合审计；费用分析更适合判断哪个项目、应用或部署配置正在产生主要开销。

## 访问流量

访问费用按平台访问入口的响应出站流量计费。平台不会直接读取每个 Pod 的外网流量，也不会把集群内服务互访计入公网访问费用。

访问流量采集是可选能力，默认不安装。账单页不会直接读取 Kubernetes，也不会用安装表里的历史状态判断探针是否在线；它只读取 API 进程维护的短 TTL 运行态状态。探针启动后会向平台发送 hello，并在每轮采集前刷新 heartbeat；如果平台没有收到有效 heartbeat，账单页会显示未部署。探针尚未成功上报过正向流量时间窗口时，账单页会显示等待上报，此时构建、运行和存储费用仍正常展示。平台管理员可以从“应用市场”安装 `Luna Gateway Traffic Probe` 平台组件，并选择目标运行集群。

网关或外部采集器需要按 GatewayRoute 和时间窗口上报响应字节数，平台按 `gateway.egress_gib` 规则换算为 credits 并写入账单。Gateway Traffic Probe 使用独立系统组件 Token 上报，后端会校验上报的 GatewayRoute 是否属于该探针所在运行集群，避免跨集群伪造用量。请求次数当前只保留为审计和后续防滥用分析，默认不扣费。

内置 Gateway Traffic Probe 当前采用 Traefik Prometheus metrics 模式：探针在集群内读取平台创建的 HTTPRoute，使用 `luna.devops/gateway-route-id` 标签建立路由映射，再抓取 Traefik metrics 中的响应字节数和请求数 counter，按分钟窗口计算增量并回报平台。默认 metrics 地址为 `http://traefik.<Gateway 命名空间>.svc.cluster.local:9100/metrics`；如果集群中的 Traefik Service 名称、命名空间或 metrics 端口不同，可以在安装模板时填写“Traefik Metrics 地址”覆盖默认值。Traefik 需要启用 Prometheus metrics，并开启 router/service 标签，例如 `--metrics.prometheus.addrouterslabels=true` 和 `--metrics.prometheus.addserviceslabels=true`，同时保证探针 Pod 可以访问该 endpoint。

### Traefik Prometheus metrics for Gateway Traffic Probe

Gateway Traffic Probe 依赖 Traefik Prometheus 指标里的 router/service 标签把流量归属到平台 HTTPRoute。只暴露 entrypoint 级别指标是不够的；如果探针日志长期显示：

```text
gateway traffic scrape completed routes=9 matchedRoutes=0 windows=0 reportableWindows=0 reportedWindows=0
```

说明探针能读取 HTTPRoute，也能抓到 metrics，但 metrics 里的标签无法匹配到平台路由。此时需要在 Traefik 侧开启 Prometheus metrics，并开启 router/service labels。

K3s 内置 Traefik 推荐通过 `HelmChartConfig` 覆盖 values。确认集群里存在 `kube-system/traefik` HelmChart 后，创建或更新：

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

如果 Traefik 是自行 Helm 安装，请在 Traefik chart values 中设置：

```yaml
metrics:
  prometheus:
    addRoutersLabels: true
    addServicesLabels: true
```

如果 Traefik 是手写 Deployment/args 管理，请确保等价参数存在：

```text
--metrics.prometheus=true
--metrics.prometheus.addrouterslabels=true
--metrics.prometheus.addserviceslabels=true
```

修改 Traefik 静态配置通常会触发 Traefik Pod 滚动重启，期间入口流量可能短暂受影响。生产环境建议先在低峰期操作，并保留原始配置回滚方式。

配置后先确认探针能访问 metrics endpoint：

```bash
kubectl -n <probe-namespace> exec -it <probe-pod> -- sh -c '
wget -qO- "$TRAEFIK_METRICS_URL" | grep -E "traefik_.*(requests|responses).*total" | head -30
'
```

输出里应能看到带 `router="...@kubernetesgateway"` 或 `service="..."` 的指标。再对照平台 HTTPRoute：

```bash
kubectl get httproute -A \
  -l app.kubernetes.io/managed-by=luna-devops \
  -o custom-columns='NS:.metadata.namespace,NAME:.metadata.name,ROUTE_ID:.metadata.labels.luna\.devops/gateway-route-id,HOST:.spec.hostnames[*]'
```

产生真实访问流量后，探针日志应从 `matchedRoutes=0` 变为至少 `matchedRoutes>0`，并在有响应字节增量时出现 `gateway traffic window reported`。账单页会在收到首个正向流量窗口后从“等待上报”变为正常访问费用。

参考文档：

- [Traefik Prometheus metrics](https://doc.traefik.io/traefik/observability/metrics/prometheus/)
- [K3s HelmChartConfig](https://docs.k3s.io/add-ons/helm)
