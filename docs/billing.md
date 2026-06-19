# 计费系统

Liteyuki DevOps 的计费主体是项目空间。每个项目空间拥有一个 credits 钱包，构建、运行、访问和存储等消耗都归集到项目空间。

## 当前实现

- 内部货币统一为 `credits`，金额使用 decimal 高精度数值存储，不使用浮点数表示费用。
- 面向普通用户的页面使用站点配置 `billing.creditsDisplayName` 展示货币单位；站点设置里的计费规则仍显示内部 credits，避免管理员误解底层结算单位。
- `project_wallets` 记录项目空间余额。
- `billing_usage_records` 记录已结算的原始用量。
- `billing_ledger_entries` 记录 append-only 账本流水，余额变动只通过新增流水体现。
- `billing_rate_rules` 保存计费规则，当前迁移时会写入一组 MVP 默认规则；平台管理员可在站点设置中调整单价和启停状态。
- 构建任务完成、失败、取消或超时后，只要 Kubernetes Build Job 已实际开始运行，就会按 BuildRun 快照中的构建资源规格结算构建费用。
- worker 会定期补算最近已完成小时的运行态 CPU/内存费用和声明容量存储费用；同一部署配置同一小时窗口重复执行时不会重复扣费。
- 账单页面提供项目空间筛选、余额、今日花费、本月花费、待结算金额、本月分类消耗、低余额提醒、账本流水和用量记录。
- 平台管理员可以在账单页为项目空间写入充值或补偿流水；余额更新和账本写入在同一个数据库事务内完成。
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

最小计费时长为 1 分钟。部署配置选择项目空间内已有环境作为构建环境；构建触发时会读取该环境当时的 CPU 和内存并写入 BuildRun 快照，历史构建不会因为后续环境规格变化而重算。手动触发构建时默认继承部署配置的构建环境，也可以临时切换到其他环境。部署配置未显式设置时默认使用运行环境。

前端在部署配置和手动触发构建表单中会按当前计费规则估算构建单价：

```text
estimated build price per minute = cpu_cores * build.cpu_vcpu_minute + memory_gib * build.memory_gib_minute
```

禁用某个计费规则后，该 meter 的单价按 `0 credits` 处理；历史账本流水不会因为后续调价或禁用而重算。

## 运行态计费

运行态费用按部署配置所在环境的资源规格估算。worker 每 10 分钟补算最近 6 个已完成小时窗口，按部署配置创建时间和发布开始时间对首个窗口做截断：

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

## 事务边界

一次费用结算必须在同一个数据库事务中完成：

1. 幂等检查用量记录。
2. 创建或锁定项目空间钱包。
3. 写入用量记录。
4. 写入账本流水。
5. 更新钱包余额。

任一步失败都整体回滚。历史账本不修改；后续充值、退款或补偿都通过新的反向流水表达。

## 充值、补偿与余额风控

平台管理员可在账单页对项目空间写入两类人工流水：

- `credit`：充值，金额必须为正数。
- `adjustment`：补偿或调账，金额可以为正数或负数。

人工流水不会修改历史账单；所有修正都通过新的账本记录表达。

后续如需接入在线支付，由第三方支付/运营系统负责订单、支付渠道、退款、发票和对账；平台只提供受信任服务端调用的充值接口。充值接口必须支持幂等键，例如使用第三方订单号作为 `idempotencyKey`，确保支付平台重复回调时同一笔订单只入账一次。退款或撤销同样不修改原充值流水，而是通过补偿/扣减接口写入反向流水。

当站点设置开启 `billing.blockNewBuildsWhenInsufficient` 时，项目空间余额小于等于 `0` 会阻止新构建和重试构建。当开启 `billing.blockDeployChangesWhenInsufficient` 时，项目空间余额小于等于 `0` 会阻止新发布、回滚发布和部署配置变更。运行中的资源先继续保留，后续可基于欠费宽限期做通知、暂停或清理策略。

## 待补齐

- 入口访问次数按时间窗口聚合计费。
- 外部充值 API、服务令牌权限和幂等键防重复入账。
- 基于欠费宽限期处理运行中资源的通知、暂停和清理策略。
