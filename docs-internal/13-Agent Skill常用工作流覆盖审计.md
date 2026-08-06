# Agent Skill 常用工作流覆盖审计

## 1. 口径

本审计以公开使用文档、控制台主要页面和业务 API 中用户高频旅程为样本，不以单个
CRUD operation 数量计算。一个工作流只有同时具备以下内容才计为“Skill 已覆盖”：

1. 发现可信目标和当前状态。
2. 选择或收集缺失参数。
3. 调用真实业务操作；缺少工具时明确阻塞。
4. 处理批准、MFA、冲突、失败和异步状态。
5. 使用权威读取工具回读。
6. 按业务验收标准给出完成、进行中或阻塞结论。

“Skill 已覆盖”不代表当前 Agent Tool Catalog 已开放对应写工具。工具未注册时，
Skill 必须阻止模型虚构执行；工具开放进度仍以 Tool Catalog 和 `TODO.md` 为准。

## 2. 覆盖矩阵

| # | 常用工作流 | 主要 Skill reference | 完成证据 | Skill |
| ---: | --- | --- | --- | --- |
| 1 | 发现、创建、更新和删除项目空间 | `projects-applications` | 项目空间回读、成员/依赖与删除终态 | 已覆盖 |
| 2 | 添加、调整和移除项目成员 | `projects-applications`、`security-administration` | 成员与角色回读 | 已覆盖 |
| 3 | 创建、更新和删除应用 | `projects-applications` | 应用归属、来源与依赖回读 | 已覆盖 |
| 4 | 从应用市场模板安装并部署 | `delivery-orchestration`、`projects-applications` | 安装记录、应用、部署配置、Release 与工作负载 | 已覆盖 |
| 5 | 从已有镜像部署 | `delivery-orchestration`、`source-build-release` | 镜像 Digest、Release、工作负载与 Service | 已覆盖 |
| 6 | 从代码仓库构建并部署 | `delivery-orchestration`、`source-build-release` | 仓库绑定、BuildRun、制品、Release 与工作负载 | 已覆盖 |
| 7 | 分析公开仓库或部署文档并预填配置 | `delivery-orchestration`、`source-build-release` | 来源可追溯、推断可编辑、Secret 留空 | 已覆盖 |
| 8 | 连接 Git Provider/账号、绑定仓库和 Webhook | `source-build-release`、`integrations-automation` | 账号/绑定回读与真实触发 BuildRun | 已覆盖 |
| 9 | 连接镜像站、凭据、查找仓库与 Tag | `source-build-release` | 连接测试、Tag/Digest 与拉取范围 | 已覆盖 |
| 10 | 配置构建环境、模板和变量集 | `source-build-release`、`integrations-automation` | 预览、配置回读与 Secret 隔离 | 已覆盖 |
| 11 | 触发、取消、重试和诊断构建 | `source-build-release`、`diagnostics-observability` | BuildRun/Job 终态、日志与制品 | 已覆盖 |
| 12 | 创建 Release 或拉取最新镜像重新部署 | `runtime-deployment`、`source-build-release` | revision、镜像与 Rollout/Pod 就绪 | 已覆盖 |
| 13 | 更新运行配置、扩缩容和重启 | `runtime-deployment` | 有效配置、新 Pod 与就绪状态 | 已覆盖 |
| 14 | 回滚发布 | `runtime-deployment` | 新 Release 与工作负载运行历史成功版本 | 已覆盖 |
| 15 | 连接、测试和管理运行集群 | `runtime-deployment` | 连接测试与实时 Kubernetes 观察 | 已覆盖 |
| 16 | Web Console、运行命令和数据导出 | `runtime-deployment`、`security-administration` | MFA/审计与会话或导出流结果 | 已覆盖 |
| 17 | 创建、修改和删除访问入口 | `gateway-networking` | HTTPRoute、后端、DNS/TLS 与外部响应 | 已覆盖 |
| 18 | 诊断 DNS、证书、Gateway 和后端不可达 | `gateway-networking`、`diagnostics-observability` | 首个失败边界与修复后回读 | 已覆盖 |
| 19 | 诊断应用未启动、崩溃或不健康 | `diagnostics-observability`、`runtime-deployment` | Release、Pod、Service、事件和日志证据 | 已覆盖 |
| 20 | 配置部署钩子并检查 HookRun | `integrations-automation` | 阶段绑定、HookRun/日志与主流程结果 | 已覆盖 |
| 21 | 创建服务引用或拓扑关系 | `integrations-automation`、`projects-applications` | 关系、源新 Release 与真实 Service 可达性 | 已覆盖 |
| 22 | 配置通知渠道、模板和规则 | `integrations-automation` | 渠道测试、真实事件与投递终态 | 已覆盖 |
| 23 | 用户、角色、注册与认证 Provider 管理 | `security-administration` | 用户/策略/Provider 回读与登录验证 | 已覆盖 |
| 24 | MFA、OAuth 应用、Grant 和 Access Token | `security-administration` | purpose 绑定验证、状态回读与撤销结果 | 已覆盖 |
| 25 | 查看费用、诊断异常费用和调整费率/钱包 | `security-administration` | 同范围余额、流水、用量和规则 | 已覆盖 |
| 26 | 站点配置与外部连接设置 | `security-administration` | 配置回读与对应连接测试 | 已覆盖 |
| 27 | 数据保留预览与清理 | `security-administration` | 预览、批准、清理统计与审计 | 已覆盖 |
| 28 | 安装和诊断平台系统组件 | `security-administration`、`runtime-deployment` | 组件应用、Release 与实时工作负载 | 已覆盖 |

按上述口径，28 个常用工作流均有完整流程与验收指导，教学覆盖率为
`28 / 28 = 100%`，达到“至少 95%”目标。

## 3. 当前执行能力差距

当前 Agent Tool Catalog 主要开放平台概览、项目空间与常见状态列表、应用市场搜索、
公开网络读取和项目空间创建。多数写操作仍未注册，因此：

- Skill 会正确收集参数、选择卡片类型和定义验收标准。
- 没有对应 operation 时必须报告“尚未执行”，不能输出成功回执。
- 只有 Tool Catalog、用户权限、批准/MFA 和业务 API 均可用时，工作流才能自动执行。
- 最优先的能力缺口仍是应用模板安装、应用/部署配置创建、构建触发、Release 创建和
  状态回读；它们决定三条应用交付主路径能否真正闭环。

## 4. 回归样例

后续 Prompt/Skill 测试至少覆盖：

- 模板、镜像、源码三种交付来源选择与不同参数表单。
- 源码仓库文档读取、可信预填、BuildRun、Release 和部署验收。
- 应用崩溃诊断、构建失败诊断和网关不可达诊断。
- Webhook、Hook、服务引用和通知投递的真实触发验收。
- 用户/权限、MFA、账单、数据保留和系统组件管理。
- 缺少写工具、异步进行中、回读失败和达到安全上限时的准确终止状态。
