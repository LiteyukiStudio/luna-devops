# 部署配置字段格式与取值范围

本页汇总部署配置中有格式约束的字段。字段含义和操作步骤见[创建部署配置](../use/workflow/deployment)；完整 API 字段以 OpenAPI 为准。

## 基本信息

| 字段 | 允许值 / 格式 | 默认 | 说明 |
| --- | --- | --- | --- |
| 阶段 | `dev`、`test`、`staging`、`prod` | `dev` | 创建后不可修改；`production` 会被规范化为 `prod`。其他值返回 `deployment.stage_invalid`。 |
| 名称 | 最长 120 个字符，可用中文和符号 | 阶段 | 仅用于展示，也是镜像模板 `{target}` 的来源。 |
| 工作负载类型 | `Deployment`、`StatefulSet` | `Deployment` | |

## 资源与副本

| 字段 | 允许值 / 格式 | 默认 |
| --- | --- | --- |
| 每副本 CPU 配额 | 正的 Kubernetes 数量，例如 `250m`、`0.5`、`1`、`2` | `1` |
| 每副本内存配额 | 正的 Kubernetes 数量，例如 `512Mi`、`1Gi` | `1Gi` |
| 构建 CPU | 同 CPU 配额 | `2` |
| 构建内存 | 同内存配额 | `4Gi` |
| 副本数 | 整数，`≥ 0`；`0` 表示停止全部 Pod | `1` |
| 自动伸缩最小副本 | 整数，`≥ 1` | `1` |
| 自动伸缩最大副本 | 整数，`≥` 最小副本 | 等于最小副本 |
| CPU / 内存目标利用率 | 整数 `0`–`1000`（百分比），`0` 表示不使用该指标；启用自动伸缩时至少一项大于 `0` | `0` |
| 构建超时 | 控制台按分钟填写；API 为秒，`60`–`86400` | 30 分钟 |

资源数量格式无效或不为正数时返回 `deployment.resource_quantity_invalid`。`Mi`、`Gi` 是二进制单位，`M`、`G` 是十进制单位；`1G` 小于 `1Gi`。CPU 和内存是配额，实际 requests/limits 由运行集群的资源分配策略计算。

## 服务端口

- 每个部署配置最多 16 个端口，端口号 `1`–`65535`，不能重复。
- 第一个端口是默认端口；需要暴露指标等附加端口时继续添加。
- 端口名称会转为小写，下划线和空格替换为 `-`，其他符号被去掉，最长 63 个字符，不能重复；留空时第一个端口命名为 `http`，其余为 `port-<端口号>`。

## 自动发布触发规则

分支匹配和标签匹配使用同一语法：

- 留空匹配全部；多个规则用逗号分隔，例如 `main,release-*`。
- `*` 匹配任意不含 `/` 的字符，`?` 匹配单个字符；`*` 单独出现时匹配全部。
- `release-*` 不匹配 `release/1.0`，需要写成 `release/*`。

## 容器启动

| 字段 | 格式 |
| --- | --- |
| 启动命令、启动参数 | JSON 字符串数组，例如 `["sh","-c","echo ok"]`；或每行一个参数。 |
| 删除的 Linux Capabilities | JSON 字符串数组或每行一个，例如 `ALL`。 |
| 镜像拉取策略 | `Always`、`IfNotPresent`、`Never`；留空由平台按发布动作决定。 |

非 JSON 写法同时按换行和英文逗号拆分；参数本身包含逗号时，必须使用 JSON 数组。JSON 格式错误会导致发布失败。

## Kubernetes JSON 字段

以下字段填写对应 Kubernetes 对象的 JSON，保存时校验结构：

| 字段 | JSON 类型 |
| --- | --- |
| 就绪 / 存活 / 启动探针 | `Probe` 对象 |
| Lifecycle | `Lifecycle` 对象 |
| Tolerations | `Toleration` 数组 |
| Affinity | `Affinity` 对象 |
| 拓扑分布约束 | `TopologySpreadConstraint` 数组 |
| 自动伸缩行为 | `HorizontalPodAutoscalerBehavior` 对象 |
| 初始化容器、辅助容器 | `Container` 数组 |

就绪探针示例：

```json
{"httpGet":{"path":"/healthz","port":8080},"initialDelaySeconds":5,"periodSeconds":10}
```

## 键值字段

节点选择器和 Service 注解支持两种写法：

- JSON 对象，例如 `{"kubernetes.io/arch":"amd64"}`；
- 每行一个 `KEY=VALUE`，空行和 `#` 开头的行会被忽略。

## 安全上下文

| 字段 | 允许值 |
| --- | --- |
| 运行用户 UID、运行用户组 GID、文件系统组 GID | 非负整数，留空表示使用镜像默认值 |
| 文件系统组变更策略 | `Always`、`OnRootMismatch` |
| Service 会话亲和 | `None`、`ClientIP` |

以下设置会被拒绝：初始化容器或辅助容器启用 `privileged`、开启 `allowPrivilegeEscalation`、添加 Linux Capabilities、指定自定义 ServiceAccount 或自动挂载其 Token、使用 `ClusterIP` 以外的 Service 类型、设置 `externalTrafficPolicy`。

## 源码构建

| 字段 | 格式 | 默认 |
| --- | --- | --- |
| Dockerfile 路径 | 相对仓库根目录的路径 | `Dockerfile` |
| 构建上下文 | 相对仓库根目录的目录 | `.` |
| 目标镜像 | `仓库路径:tag`，可用变量见[镜像仓库路径与 Tag 模板](./image-templates) | 由推送凭据模板生成 |
| Build Args | 每行 `KEY=value`，见[构建参数与构建变量](./build-variables) | 空 |
