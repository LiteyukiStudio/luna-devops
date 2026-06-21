# 计费系统

Liteyuki DevOps 的余额归属到用户钱包。项目空间保存当前计费归属人，构建、运行、访问和存储等费用会在结算时扣到当时的归属人，并保留项目空间作为费用来源。

## 当前实现

- 内部货币统一为 `credits`，金额使用 decimal 高精度数值存储，不使用浮点数表示费用。
- 面向普通用户的页面使用站点配置 `billing.creditsDisplayName` 展示货币单位；站点设置里的计费规则仍显示内部 credits，避免管理员误解底层结算单位。
- 站点设置可配置 `billing.fiatCurrencyUnit` 和 `billing.creditsPerFiatUnit`，用于平台管理员在账单概览中查看现实货币折算金额；该换算只用于展示，底层账本仍只存储 credits。
- `user_wallets` 记录用户余额。
- `projects.billing_owner_user_id` 记录项目空间当前计费归属人。
- `billing_usage_records` 记录已结算的原始用量，并固化当时的扣费用户。
- `billing_ledger_entries` 记录 append-only 账本流水，余额变动只通过新增流水体现。
- `billing_rate_rules` 保存计费规则，当前迁移时会写入一组 MVP 默认规则；平台管理员可在站点设置中调整单价和启停状态。
- 构建任务完成、失败、取消或超时后，只要 Kubernetes Build Job 已实际开始运行，就会按 BuildRun 快照中的构建资源规格结算构建费用。
- worker 会定期补算最近已完成小时的运行态 CPU/内存费用和声明容量存储费用；同一部署配置同一小时窗口重复执行时不会重复扣费。
- 访问费用按平台访问入口的响应出站流量结算；网关或外部采集器按 GatewayRoute 和时间窗口上报响应字节数，平台换算为 GiB 后扣费。同一路由同一窗口重复上报不会重复扣费。
- 账单页面提供项目空间筛选、用户余额、今日花费、本月花费、待结算金额、本月分类消耗、低余额提醒、账本流水和用量记录。
- 平台管理员可以在账单页为指定用户账户写入充值或补偿流水；余额更新和账本写入在同一个数据库事务内完成。
- 站点设置提供计费配置入口，可调整 credits 展示名称、默认免费额度、低余额提醒阈值、欠费宽限期、是否允许欠费余额，以及余额不足时是否阻止新构建或部署变更。
- 平台本体不内置支付宝、微信、Stripe 等在线支付渠道；后续只提供受信任第三方系统调用的幂等充值 API。

## 构建计费

构建费用按 CPU、内存和运行时长估算：

```text
build credits = duration_minutes * (cpu_cores * build.cpu_vcpu_minute + memory_gib * build.memory_gib_minute)
```

默认规则：

| Meter | 默认单价 |
| --- | ---: |
| `build.cpu_vcpu_minute` | `10 credits` |
| `build.memory_gib_minute` | `2 credits` |

最小计费时长为 1 分钟。部署配置直接保存构建 CPU 和内存规格；构建触发时会读取该部署配置当时的构建规格并写入 BuildRun 快照，历史构建不会因为后续规格变化而重算。手动触发构建时默认继承部署配置的构建规格，也可以临时覆盖。

前端在部署配置和手动触发构建表单中会按当前计费规则估算构建单价：

```text
estimated build price per minute = cpu_cores * build.cpu_vcpu_minute + memory_gib * build.memory_gib_minute
```

禁用某个计费规则后，该 meter 的单价按 `0 credits` 处理；历史账本流水不会因为后续调价或禁用而重算。

## 运行态计费

运行态费用按部署配置自身的副本数、CPU 和内存规格估算。worker 每 10 分钟补算最近 6 个已完成小时窗口，按部署配置创建时间和发布开始时间对首个窗口做截断：

```text
runtime cpu credits = replicas * cpu_cores * duration_hours * runtime.cpu_vcpu_hour
runtime memory credits = replicas * memory_gib * duration_hours * runtime.memory_gib_hour
```

默认规则：

| Meter | 默认单价 |
| --- | ---: |
| `runtime.cpu_vcpu_hour` | `30 credits` |
| `runtime.memory_gib_hour` | `6 credits` |

只有存在 `running` 或 `succeeded` 发布记录的启用部署配置会进入运行态计费。未发布的部署配置不会扣费。

前端在项目环境和部署配置表单中会按当前计费规则估算运行单价：

```text
estimated runtime price per hour = replicas * (cpu_cores * runtime.cpu_vcpu_hour + memory_gib * runtime.memory_gib_hour)
```

## 存储计费

存储费用按部署配置中启用的运行数据卷声明容量计费，不读取底层 PVC 的实际使用量。worker 与运行态计费一起按已完成小时窗口结算，并换算为 `gib_day`：

```text
storage credits = capacity_gib * duration_days * storage.gib_day
```

默认规则：

| Meter | 默认单价 |
| --- | ---: |
| `storage.gib_day` | `1 credits` |

多数据卷会合并容量计费，例如 `/data/app1 20Gi` 和 `/data/app2 40Gi` 在一个小时窗口中的用量为 `60Gi * 1/24 day`。关闭运行数据保留后不再产生新的存储计费；已有历史账单不会重算。

## 访问流量计费

访问费用按平台访问入口的响应出站流量计费，不读取 Pod 网卡总流量，也不把集群内服务互访计入公网访问费用。所有外部 HTTP/HTTPS 访问都应经过平台创建的 GatewayRoute/Ingress，再由网关日志或外部采集器上报用量：

```text
gateway credits = response_bytes / 1024 / 1024 / 1024 * gateway.egress_gib
```

默认规则：

| Meter | 默认单价 |
| --- | ---: |
| `gateway.egress_gib` | `1 credits` |
| `gateway.requests_1000` | `0 credits`，默认禁用 |

上报接口要求平台管理员或 `billing:write` Access Token：

```bash
curl -X POST "$DEVOPS_BASE_URL/api/v1/billing/gateway-traffic" \
  -H "Authorization: Bearer $DEVOPS_BILLING_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "routeId": "gwr_xxx",
    "responseBytes": 536870912,
    "requestCount": 1200,
    "periodStart": "2026-06-21T10:00:00Z",
    "periodEnd": "2026-06-21T10:05:00Z"
  }'
```

`routeId + periodStart` 构成用量窗口的幂等键；重复提交同一窗口会返回已结算状态，不会重复扣费。`requestCount` 当前写入 metadata 供审计和后续防滥用分析，默认不产生费用。

## 事务边界

一次费用结算必须在同一个数据库事务中完成：

1. 幂等检查用量记录。
2. 读取项目空间当前计费归属人，并创建或锁定该用户钱包。
3. 写入用量记录。
4. 写入账本流水。
5. 更新钱包余额。

任一步失败都整体回滚。历史账本不修改；后续充值、退款或补偿都通过新的反向流水表达。

## 充值、补偿与余额风控

平台管理员可在账单页对用户账户写入两类人工流水：

- `credit`：充值，金额必须为正数。
- `adjustment`：补偿或调账，金额可以为正数或负数。

人工流水不会修改历史账单；所有修正都通过新的账本记录表达。

后续如需接入在线支付，由第三方支付/运营系统负责订单、支付渠道、退款、发票和对账；平台只提供受信任服务端调用的充值接口。充值接口必须支持幂等键，例如使用第三方订单号作为 `idempotencyKey`，确保支付平台重复回调时同一笔订单只入账一次。退款或撤销同样不修改原充值流水，而是通过补偿/扣减接口写入反向流水。

外部系统通过平台管理员创建的 `billing:write` Access Token 调用：

```bash
curl -X POST "$DEVOPS_BASE_URL/api/v1/billing/external-transactions" \
  -H "Authorization: Bearer $DEVOPS_BILLING_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "usr_xxx",
    "amountCredits": "100.5",
    "type": "credit",
    "idempotencyKey": "payment-order-20260620-0001",
    "description": "第三方订单 payment-order-20260620-0001 入账"
  }'
```

`type=credit` 表示充值，金额必须为正数；`type=adjustment` 可用于补偿、扣减或退款，金额可以为正数或负数。同一个用户钱包内重复提交相同 `idempotencyKey`、相同金额和类型时会返回第一次生成的账本流水，不会重复入账；同一个幂等键被不同金额或类型复用会被拒绝。

项目空间计费归属人发生转移后，新产生的费用归新归属人；转移前已经产生的用量和流水仍保留在原归属人账下，不追溯迁移。

当站点设置开启 `billing.blockNewBuildsWhenInsufficient` 时，计费归属人余额小于等于 `0` 会阻止新构建和重试构建。当开启 `billing.blockDeployChangesWhenInsufficient` 时，计费归属人余额小于等于 `0` 会阻止新发布、回滚发布和部署配置变更。运行中的资源先继续保留，后续可基于欠费宽限期做通知、暂停或清理策略。

## 待补齐

- 网关访问日志采集器：从 Traefik、ingress-nginx 或 Caddy access log/metrics 读取响应字节数并定期调用用量上报接口。
- 基于欠费宽限期处理运行中资源的通知、暂停和清理策略。
