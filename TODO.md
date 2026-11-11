# TODO

## 优化方向

- [ ] 访问入口支持一个域名多个应用走不同的/path使用

## 多租户安全

- [ ] 为项目 Namespace 下发 ResourceQuota、LimitRange 与 Pod Security Admission；限制副本数、HPA 上限、项目构建并发和每用户项目空间数量。
- [ ] 拒绝租户自填 `tolerations`、`PriorityClass` 和平台节点选择器，或改为由平台后台配置的可选集合。
- [ ] 构建 Job 与租户工作负载分离到独立 Namespace 和节点池，避免 Unconfined seccomp/AppArmor 的构建 Pod 与租户生产 Pod 共节点。
- [ ] 自定义域名接入前做 DNS TXT 归属验证；`gateway_routes.host` 加唯一约束；TLS Secret 名不再由 host 有损转换，避免碰撞。
- [ ] 构建可选的推送凭据不再包含平台全局凭据，或仅限平台管理员配置的项目空间使用；构建 Hook 编辑权限与凭据可见性对齐。
- [ ] 收紧 project scope 资源的使用判定：只允许在资源已绑定的当前项目中使用；`normalizeOwnerScope` 遇到未知 scope 拒绝而非视为 `global`。
- [ ] `token:manage` 不允许授权给第三方 OAuth 应用；PAT 支持绑定项目空间。
- [ ] 对齐开发者角色的 `deployment:exec`、`secret:update` 与 `secret:view_value` 权限模型。
- [ ] 每个项目空间配置独立 imagePullSecrets，并限制部署镜像地址只能来自项目可用的镜像仓库。

## 网络策略

数据库保存期望配置，Kubernetes NetworkPolicy 只是确定性渲染的产物。项目隔离、默认拒绝私网/保留网段/节点/API、集群追加拒绝网段、构建出站按集群配置、CNI 主动探测（只提示不拦截）、项目空间入站（项目内互通 / 按服务引用放行）与出站（公网 / 白名单 / 禁止外联）模式、部署配置“内部服务”、集群级出站例外与项目空间授权（审计、成员与访问入口防跳板提示）、跨项目共享（双向确认，持有例外的提供方需集群维护者批准）、平台生成 Pod 的宿主机隔离校验、整对象覆盖与清理残留、5 分钟周期修正和实时状态已上线；以下为剩余阶段。

- [ ] 平台控制面（API、PostgreSQL、Redis）不在私网时的显式硬底线；CNI 支持时评估改用 BaselineAdminNetworkPolicy。
- [ ] 构建 Pod 移出项目命名空间（需要 Unconfined seccomp/AppArmor）后，为项目命名空间启用 Pod Security `enforce=baseline`，作为平台生成 Pod 隔离校验之外的集群侧兜底。
- [ ] 同步触发补强：配置变更经 outbox 投递（替代当前入队失败依赖周期兜底），节点变化由 informer 即时触发，漂移修正写审计；前端增加“同步中”状态。
- [ ] 网络隔离上线说明（含构建出站从 `BUILD_EGRESS_MODE` / `BUILD_*_EGRESS_*` 迁到集群配置，升级后需在集群页重新填写）：升级后部署需要集群凭据能读取 Node 与 `default/kubernetes` 端点并管理项目命名空间 NetworkPolicy；依赖其他命名空间服务或私网数据库的已有应用会被阻断，需在发布说明写明并给出排障路径。
- [ ] apiserver 地址改读 EndpointSlice（`Endpoints` 已弃用）。
- [ ] 隔离状态接口与同步批量化：一次 List 全集群受管 NetworkPolicy、一次查询集群全部项目与系统组件，避免项目空间多时超时；同步时复用 List 结果，不再逐个 Get。
- [ ] 集群 DNS 放行支持 NodeLocal DNS 等非 `kube-system/k8s-app=kube-dns` 的地址（集群配置项）。
- [ ] 后续：出站例外组（多条规则打包授权给多个项目空间）、按域名出站（Cilium `toFQDNs` 或平台出站代理）、公网来源 IP 白名单（网关中间件实现）。

## 计费与配额

- [ ] 结算按未结算的 runtime_observations 驱动，不再只扫描 `active` 部署配置，修复删除重建逃避当前小时计费。
- [ ] 部署 Hook Job 设置资源 requests/limits 并计入运行费用。
- [ ] 余额不足默认拦截新建与扩容；定义透支上限和欠费后的缩容/停服流程，并在公开文档说明。
- [ ] 为停留在 `reconciliation_required` / `hold_deficit` 的 AI 预扣实现对账推进，避免长期占用余额导致其他扣费失败。
- [ ] 运行费按实际运行的 Pod 计量，Pending/CrashLoop 不按期望副本全额扣费；实现或移除 `gateway.requests_1000` 计量项。
- [ ] 运行观测按集群并发采样，单个集群不可达不中断其他目标；为 runtime_observations 制定保留与清理策略。
- [ ] 用户自有集群与平台集群使用不同计价规则。
- [ ] 计费主体被移出项目空间后的归属转移规则；评估组织/团队钱包与自助充值入口。

## 交付链路可靠性

- [ ] Worker 重启不删除在途构建 Job；构建跟踪改由 reconciler 接管孤儿 Job 并回写终态。
- [ ] 同一部署配置的发布串行化（发布租约 + `(target, revision)` 唯一约束）；重试不重复执行 preDeployment Hook。
- [ ] 取消与被新版本取代时同步停止对应 Kubernetes Job，而不只更新数据库。
- [ ] 数据库写入与任务投递之间引入 outbox，避免 Redis 抖动后 Webhook 构建卡在 queued 且重投被当作重复。
- [ ] Scheduler 只在单个 Worker 上运行或周期任务加 Unique，避免多副本重复执行。
- [ ] 构建日志改为追加写入，日志流中断时不把仍在运行的构建判为失败。
- [ ] 空 ClusterID 在创建时固化为当时的默认集群，避免默认集群变更后漂移。

## 产品与文档

- [ ] 确定产品定位（私有化部署或公有 SaaS），统一 AGENTS.md、代码中的邮箱自助注册和公开文档的注册口径。
- [ ] 公开文档补充租户路径：注册 → 创建项目空间 → 第一个应用上线；说明余额、欠费与配额行为。
- [ ] 默认项目空间名称改由前端 i18n 生成，后端不返回本地化文案。
- [ ] 统一 GitLab 支持状态：实现 Provider，或从 OpenAPI 枚举、UI 和文档中移除。
- [ ] 补充密码找回、账号注销，以及服务条款、隐私政策和 SLA 页面。
- [ ] 钩子环境变量由 `LITEYUKI_*` 迁移到符合运行标识约束的前缀，同步文档与模板。
