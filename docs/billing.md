# 计费系统

Liteyuki DevOps 的计费主体是项目空间。每个项目空间拥有一个 Credits 钱包，构建、运行、访问和存储等消耗都归集到项目空间。

## 当前实现

- 内部货币统一为 `credits`，金额使用 decimal 高精度数值存储，不使用浮点数表示费用。
- `project_wallets` 记录项目空间余额。
- `billing_usage_records` 记录已结算的原始用量。
- `billing_ledger_entries` 记录 append-only 账本流水，余额变动只通过新增流水体现。
- `billing_rate_rules` 保存计费规则，当前迁移时会写入一组 MVP 默认规则；平台管理员可在站点设置中调整单价和启停状态。
- 构建任务完成、失败、取消或超时后，只要 Kubernetes Build Job 已实际开始运行，就会按项目空间环境资源规格结算构建费用。
- worker 会定期补算最近已完成小时的运行态 CPU/内存费用；同一部署配置同一小时窗口重复执行时不会重复扣费。
- 账单页面提供项目空间筛选、余额、今日花费、本月花费、账本流水和用量记录。
- 平台管理员可以在账单页为项目空间写入充值或补偿流水；余额更新和账本写入在同一个数据库事务内完成。
- 站点设置提供计费配置入口，可调整 credits 展示名称、默认免费额度、低余额提醒阈值、欠费宽限期、是否允许欠费余额，以及余额不足时是否阻止新构建或部署变更。

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

最小计费时长为 1 分钟。环境没有显式资源规格时，构建结算使用 `500m` CPU 和 `512Mi` 内存作为保守默认值。

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

当站点设置开启 `billing.blockNewBuildsWhenInsufficient` 时，项目空间余额小于等于 `0` 会阻止新构建和重试构建。当开启 `billing.blockDeployChangesWhenInsufficient` 时，项目空间余额小于等于 `0` 会阻止新发布、回滚发布和部署配置变更。运行中的资源先继续保留，后续可基于欠费宽限期做通知、暂停或清理策略。

## 待补齐

- 数据卷按声明容量持续计费。
- 入口访问次数按时间窗口聚合计费。
- 站点管理员配置充值包与在线充值流程。
- 基于欠费宽限期处理运行中资源的通知、暂停和清理策略。
