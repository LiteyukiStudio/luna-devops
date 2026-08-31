# Luna DevOps kubectl 兼容网关与落地方案

本文定义 Luna DevOps 对标准 `kubectl` 和 Kubernetes API 协议的完整兼容边界、架构决策与一次性交付
方案。已实现的数据结构、API 字段、资源目录和错误码最终以代码、OpenAPI 与自动化测试为事实源；本文
长期维护产品边界、调用链不变量和验收门禁。

本事项不采用只读子集、分批开放或半成品兼容。kubeconfig、Discovery、OpenAPI、资源读写、
Server-Side Apply、Watch、日志流、Exec、Attach、Port-forward、cp、授权自检、撤权、审计和
可观测必须在同一个交付事项中闭环；任一能力未完成时，整个事项不得标记完成或对用户开放。

## 0. 当前落地与验收状态（2026-08-31）

本轮已经把方案主体落实到代码变更集，但“代码已落地”不等于“完成门禁已通过”。现有运行集群默认
`kube_gateway_enabled=false`，必须由平台管理员逐集群显式启用；在第 19 节真实环境矩阵完成前，不把
当前源码描述为已经通过正式 kubectl 兼容认证。

当前变更集已覆盖：

- `000095_kubectl_gateway` 数据库迁移、Kube Credential/Binding、独立 `kube:*` Scope、审计元数据、
  RuntimeCluster 网关配置与可恢复删除状态，以及 kubectl 工作负载观察身份。
- Credential 四个管理接口、网关 GET/PUT、`features.kubectlGateway`、OpenAPI 敏感字段与 Agent 集中排除。
- `kubecatalog`、`kubepolicy`、`kubeproxy` 协议流水线，以及 Provider 的 Namespace/RBAC/TokenRequest/
  清理/观察、Worker 协调/补投/删除/计费接线。
- DeploymentTarget namespace override 和既有高风险工作负载输入的全链路收口，RuntimeCluster 非
  `active` 状态在管理/删除恢复之外失败关闭。
- Web 项目空间创建入口、账号 Credential 管理、集群网关配置和五语言文案；独立 Luna CLI 仓库中的
  `kubeconfig write/merge` 安全适配器及其契约测试。
- 中英文用户操作、CLI 入口、管理员反向代理与安全限制文档。

已经纳入的包级、契约和前端自动化不能替代真实环境验收。更新本文时仍未完成的是：用可销毁
PostgreSQL、Redis、Kubernetes、实际部署所用反向代理、真实 `kubectl` 和临时外部 OTel 栈跑完第 19 节
全矩阵，以及已登录浏览器的桌面/移动交互验收。因此 `TODO.md` 继续保留完整调用链和真实 E2E 两项，
第 20 节完成门禁仍未满足；最终测试结果以本事项交付记录和 CI 输出为准。

## 1. 目标与完整交付范围

Luna DevOps 提供一个 Kubernetes API 兼容网关。用户将网关地址写入 kubeconfig，使用标准
`kubectl` 管理自己有权限的项目空间及其中的应用资源。客户端始终连接 Luna DevOps，平台保存的
真实 kube-apiserver 地址、上游 kubeconfig 和上游身份不得下发给用户。

完整交付必须覆盖：

- API Discovery、聚合 Discovery、Kubernetes OpenAPI v2/v3 和 `kubectl explain`。
- `get`、`list`、`watch`、`describe`、`wait`、`top` 和全部标准输出格式。
- `create`、`apply`、`diff`、`edit`、`replace`、`patch`、`delete`、`deletecollection` 和 `dry-run`。
- Client-Side Apply、Server-Side Apply、JSON Patch、JSON Merge Patch 和 Strategic Merge Patch。
- `scale`、`autoscale`、`set image/env/resources` 及常用 `rollout` 操作。
- `logs`、`logs -f`、`exec`、`attach`、`port-forward` 和依赖 Exec 的 `cp`。
- 项目空间级、应用过滤级 kubeconfig 生成、下载、列出、过期和撤销。
- `kubectl auth can-i`、`can-i --list` 和 `auth whoami`。
- Kubernetes 原生 Status、Table、Protobuf、分页、Watch、WebSocket 和 SPDY 协议语义。
- 身份撤销、项目成员变化、项目或集群停用后的普通请求与长连接收口。
- API、数据库、Secret Store、Kubernetes Provider、流式连接和最终副作用的全链路审计与可观测。
- `kubectl config`、`completion`、`kustomize`、插件加载等纯客户端能力无需服务端适配，继续按 kubectl
  自身语义工作；`kubectl proxy` 可以作为本地客户端代理使用，但经它发出的请求仍受本方案全部限制。

这里的“完整兼容”是指上述能力在 Luna DevOps 授权的项目资源范围内全部可用，不代表把平台变成
集群管理员入口。Node、集群 RBAC、CRD 定义、Webhook、CSR 等集群级管理能力不属于项目用户的
kubectl 能力。

## 2. 现状与同一事项内的前置收口

现有代码已经提供以下基础：

- `client-go` typed、dynamic 和 `rest.Config` 客户端，以及统一的 Kubernetes Trace Transport。
- RuntimeCluster 上游 kubeconfig 的安全校验、Secret Store 保存和不回显边界。
- 平台角色、项目角色、Access Token/OAuth Scope、项目成员权威回读和集群可用性判断。
- `app.kubernetes.io/managed-by=luna-devops`、项目、应用、部署目标和 Release 等资源标签。
- 运行资源列表、YAML、Event、日志、非交互 Exec 和浏览器终端能力。
- WebSocket 终端的身份、项目权限和资源归属定时复核。

实现网关时必须在同一事项中收口以下现存差异：

1. **Namespace 唯一事实源。** Worker 实际部署固定使用 `Project.KubernetesNamespace`，但部分 API
   仍优先读取 `DeploymentTarget.Namespace`。必须删除目标级 namespace override 的现行输入、响应、
   OpenAPI、前端和分支逻辑；kubeconfig、授权、部署、日志、终端和观察统一只认项目 namespace。
2. **认证与平台路由 Scope 解耦。** 当前 Bearer 认证同时按 Gin `FullPath` 计算平台 Scope，未知通配
   路由会落入 `system:unmapped`。需要拆出可复用的 Token 身份校验，再由 Kube Authorizer 按 Kubernetes
   verb、GVR 和 subresource 执行 Scope 校验。
3. **资源管理来源明确。** 新增 `luna.devops/management-source=platform|kubectl` 标签；现有平台资源
   归为 `platform`，从 kubectl 创建的资源归为 `kubectl`。两类资源都必须具有不可变的项目归属标签。
4. **上游代理身份最小化。** kubectl 流量不得直接使用集群管理 kubeconfig 作为裸代理身份。启用网关时
   由管理凭据安装固定的受限 ServiceAccount、非资源 URL 权限和项目 namespace RoleBinding；代理使用
   短期 TokenRequest 凭据，管理凭据只负责安装、刷新和回收这些平台组件。

应用当前没有独立成员 ACL。应用级 Binding 只增加资源过滤范围，最终权限仍来自所属项目角色；不得
在本事项中顺带建立第二套应用成员模型。

## 3. 总体架构

```text
kubectl
  -> /kube/v1/bindings/{bindingId}/...
  -> Kube Credential 认证
  -> Binding、用户、项目成员、应用和集群权威回读
  -> Kubernetes RequestInfo 解析
  -> Kube Authorizer 与中央资源策略
  -> Namespace、所有权标签和工作负载安全校验
  -> 协议感知反向代理
  -> 专用受限上游 rest.Config
  -> kube-apiserver
```

API 进程内已新增聚焦的 `internal/kubeproxy` 领域，职责拆分为：

- `Authenticator`：只验证 Kube Bearer、Token 状态、用户状态和 Binding 关系。
- `RequestInfoResolver`：解析 Kubernetes 路径、方法、查询参数和 Upgrade 类型。
- `Authorizer`：把 verb、API Group、resource、subresource 和对象归属映射到平台 Action。
- `ResourcePolicy`：维护内置资源目录、受保护资源和额外 namespaced GVR 规则。
- `OwnershipGuard`：注入并校验项目、应用和 management-source 标签。
- `WorkloadPolicy`：API 与 Worker 共用的租户工作负载安全校验器。
- `UpstreamClientFactory`：解析 Secret Store 中的集群配置并取得专用网关身份。
- `Proxy`：普通 HTTP、Watch、日志和 Upgrade 协议转发。
- `StreamAuthorizer`：长连接期间持续复核身份、Binding、项目和对象归属。
- `AuditRecorder`：记录请求或流的开始、拒绝、完成、取消和失败终态。

不使用 Kubernetes Aggregation Layer，不通过 Ingress 或 Gateway 模块承载对象级授权，也不把同步
kubectl 请求投递给 Worker。

## 4. Kube Credential、Binding 与 kubeconfig

### 4.1 数据模型

复用现有 `AccessToken` 的哈希、用户、Scope、过期和撤销能力，新增 `source=kubeconfig`。明文凭据
只在创建响应中出现一次，不写入数据库、日志、审计或遥测。

新增 `KubeAccessBinding`：

| 字段 | 含义 |
| --- | --- |
| `id` | 不可预测的 Binding ID，同时进入网关 URL |
| `access_token_id` | 对应的 kubeconfig AccessToken |
| `project_id` | 固定项目空间 |
| `runtime_cluster_id` | 固定运行集群 |
| `application_id` | 可空；存在时强制应用标签过滤 |
| `created_at` | 创建时间 |

约束：

- Token 删除、撤销或过期后，其全部 Binding 立即拒绝新请求。
- 项目、应用、用户或集群删除时级联清理 Binding，不让 Binding 阻塞业务对象删除。
- Binding 只保存上下文边界，不保存角色或权限快照；每个请求继续权威回读用户、成员关系、项目状态、
  集群 Scope、应用归属和 Token Scope。
- 同一 kubeconfig 可以包含多个项目、集群或应用 Context；它们可共用一个凭据，但必须拥有独立 Binding。
- Binding 不使用软删除；`access_token_id` 硬删除级联，其他业务资源软删除由删除终态显式清理。
- Project namespace 如果发生变化，删除该项目全部 Binding 并关闭相关流，要求用户重新生成 kubeconfig；
  不让旧文件中的默认 namespace 与服务端权威 namespace 静默分叉。
- 使用 `(access_token_id, project_id, runtime_cluster_id, coalesce(application_id, ''))` 唯一索引阻止重复
  Context，并分别为 Token、项目、集群和应用外键建立清理/查询索引。
- Credential 的 `active|expired|revoked` 为 `expires_at/revoked_at` 派生状态，不新增会漂移的状态列。

`AccessToken` 不增加明文字段，只增加稳定的来源常量 `kubeconfig`。普通个人令牌接口继续只查询
`source=personal`，OAuth 流程继续只处理 `source=oauth`；Kube 网关认证必须同时校验
`source=kubeconfig`，避免普通 PAT 即使偶然包含同名 Scope 也能进入 Kubernetes 协议入口。
反向也成立：`source=kubeconfig` 只能访问 `/kube/v1/bindings/`，普通平台 JSON API 在身份入口直接拒绝，
不能因为未来 Scope 名称重叠而把 kubeconfig 变成通用平台 Token。

`RuntimeCluster` 增加以下期望配置和可恢复删除字段，不持久化实时状态或
ServiceAccount Token：

| 字段 | 存储 | 含义 |
| --- | --- | --- |
| `kube_gateway_enabled` | boolean | 是否期望启用该集群的 kubectl 网关 |
| `kube_gateway_extra_resource_rules` | JSONB | 平台管理员允许的额外 namespaced GVR、subresource、verb 与现有业务 Action 映射 |
| `delete_status` | string | 复用 `active|deleting|delete_failed|deleted` 删除状态机 |
| `delete_message` | text | 只保存经裁剪的诊断信息，API 按稳定 code 向用户表达错误 |
| `delete_started_at` / `delete_finished_at` | timestamptz | 删除超时恢复和终态记录 |
| `kube_gateway_drain_until` | timestamptz nullable | Binding 撤销后的流排空截止时间，防止 Worker 在 API 长连接收敛前回收上游 RBAC |
| `kube_gateway_cleanup_completed_at` | timestamptz nullable | 持久化“上游已清理”阶段，保证 Secret 删除后重试不再依赖已丢失的凭据 |

`AuditLog` 增加可空 `metadata` JSONB。写入必须经过 Kube 专用白名单结构序列化，模型字段不直接接受
任意 map 或请求正文；旧的审计写入方法保持可用，新入口额外接收结构化元数据。

现有 `RuntimeObservation` 只以 `deployment_target_id` 作为资源身份，无法覆盖没有 DeploymentTarget 的
kubectl 工作负载。本事项需把观察身份扩展为：

| 字段 | 含义 |
| --- | --- |
| `management_source` | `platform` 或 `kubectl` |
| `resource_kind` | 固定允许目录中的工作负载 Kind |
| `resource_uid` | Kubernetes 对象 UID，用于区分同名重建 |
| `application_id` | 可空；来自不可变应用标签 |
| `deployment_target_id` | 平台资源保留，kubectl 资源可空 |

观察唯一约束改为运行集群、项目、资源 UID 与分钟窗口组合；观察记录仍是不可变计费输入，不作为实时状态
缓存。`BillingUsageRecord` 继续使用已有的通用 `resource_type/resource_id`，无需新增一套 kubectl 账单表。

数据库迁移统一使用当前下一个版本：

```text
migrations/000095_kubectl_gateway.up.sql
migrations/000095_kubectl_gateway.down.sql
```

迁移同时创建 Binding 表及索引、增加 RuntimeCluster 网关配置与删除状态字段、增加 Audit
字段、扩展 RuntimeObservation 身份，并删除
`deployment_targets.namespace`。由于 Project、Application 和 RuntimeCluster 使用软删除，业务删除终态还
必须显式清理 Binding；数据库外键级联只作为硬删除兜底。已有 RuntimeCluster 统一回填
`delete_status=active`、`kube_gateway_enabled=false`，新增列使用显式 `NOT NULL`/默认值或可空阶段时间，
不依赖 GORM AutoMigrate 猜测结构。

### 4.2 Scope

新增独立 Scope：

- `kube:read`：Discovery、OpenAPI、普通读取、Watch、日志、Metrics 和授权自检。
- `kube:write`：Create、Update、Patch、Apply、Delete 及写子资源；自动包含 `kube:read`。
- `kube:connect`：Exec、Attach、Port-forward、cp 和受控 Debug；自动包含 `kube:read`。

Kube Scope 只表示传输能力，不能替代现有业务 Action。每次授权同时要求 Kube Scope、项目角色和
资源对应的 `deployment:*`、`secret:*`、`volume:*`、`gateway:*` 或 `cluster:*` Action。

### 4.3 kubeconfig

生成结果使用平台可信公开地址：

```yaml
clusters:
  - name: luna/<project-id>/<cluster-id>/<application-id-or-all>
    cluster:
      server: https://<PUBLIC_BASE_URL>/kube/v1/bindings/<bindingId>
contexts:
  - name: luna/<project-id>/<cluster-id>/<application-id-or-all>
    context:
      cluster: luna/<project-id>/<cluster-id>/<application-id-or-all>
      user: luna/<credentialId>
      namespace: <Project.KubernetesNamespace>
users:
  - name: luna/<credentialId>
    user:
      token: <仅返回一次的 Kube Credential>
```

必须满足：

- Server 只从可信的 `PUBLIC_BASE_URL` 生成，禁止根据请求 Host 猜测。
- 不生成 `insecure-skip-tls-verify`，也不包含上游 CA、Token、证书或 Server 地址。
- Context 与 cluster entry 只使用稳定资源 ID，不使用可变 display name 或 identifier；项目级 Binding 以
  `all` 结尾，应用级 Binding 以 application ID 结尾，保证同一项目和集群可同时存在多个应用 Context。
- 网关只接受 Bearer，不接受平台 Cookie、Basic Auth、客户端证书或用户提供的上游 kubeconfig。
- 有效期只允许 1、7、30 天，默认 7 天，并允许用户随时撤销；下载响应使用
  `Cache-Control: no-store`。
- 用户需要自行以 `0600` 权限保存 kubeconfig；Web 只展示一次下载，不提供明文回看。

## 5. 管理 API 与协议入口

### 5.1 接口收口原则

新增六个普通平台 JSON 接口。Context 选择不再增加重复的候选接口：Web 与 CLI 继续使用现有分页接口
`GET /api/v1/runtime/clusters?projectId=...` 和
`GET /api/v1/projects/{projectId}/applications`，创建 Credential 时由后端最终校验项目、集群、应用
组合、网关状态和当前角色。

所有 Credential 管理接口使用 `token:manage` 作为普通平台 Bearer Scope，并继续校验当前用户所有权。
这只负责进入管理 API；创建出来的 Kube Credential 在协议入口使用独立 `kube:*` Scope。运行集群网关
配置只允许平台管理员，GET 映射 `cluster:read`，PUT 映射 `cluster:manage`。

接口总览：

| Method | Path | 类型 | 用途 |
| --- | --- | --- | --- |
| POST | `/api/v1/kube-credentials` | 平台 JSON API | 创建 Credential、Binding 和一次性 kubeconfig |
| GET | `/api/v1/kube-credentials` | 平台 JSON API | 分页列出当前用户 Credential |
| GET | `/api/v1/kube-credentials/{credentialId}/bindings` | 平台 JSON API | 分页列出 Credential 的 Context 元数据 |
| DELETE | `/api/v1/kube-credentials/{credentialId}` | 平台 JSON API | 撤销 Credential 及其全部 Context |
| GET | `/api/v1/runtime/clusters/{clusterId}/kube-gateway` | 平台 JSON API | 读取期望配置和实时网关状态 |
| PUT | `/api/v1/runtime/clusters/{clusterId}/kube-gateway` | 平台 JSON API | 全量替换网关期望配置并触发协调 |
| GET/POST/PUT/PATCH/DELETE/HEAD | `/kube/v1/bindings/{bindingId}` | Kubernetes 协议 | 处理绑定服务根路径兼容请求 |
| GET/POST/PUT/PATCH/DELETE/HEAD | `/kube/v1/bindings/{bindingId}/*kubePath` | Kubernetes 协议 | 处理 Discovery、OpenAPI、资源、Watch、Logs 和 Upgrade 请求 |

### 5.2 Credential 接口

#### `POST /api/v1/kube-credentials`

`operationId=createKubeCredential`。一次创建一个 `source=kubeconfig` 的 AccessToken 和一到多个 Binding。

请求：

```json
{
  "name": "开发环境",
  "expiresInDays": 7,
  "scopes": ["kube:read", "kube:write", "kube:connect"],
  "contexts": [
    {
      "projectId": "prj_xxx",
      "runtimeClusterId": "clu_xxx",
      "applicationId": "app_xxx"
    }
  ]
}
```

约束：

- `name` trim 后长度为 1～64；`expiresInDays` 只允许 1、7、30，默认 7，不允许永久 Kube Credential。
- `contexts` 必须为 1～20 项并按项目、集群、应用组合去重；`applicationId` 可空，空表示整个项目
  namespace。
- `kube:write`、`kube:connect` 规范化后自动包含 `kube:read`。
- 服务逐项校验项目有效、当前成员角色、集群可用于该项目、网关实时为 Ready、应用属于项目，并确保项目
  RoleBinding 已就绪；任一 Context 失败时整笔数据库事务回滚。Credential 不创建专属上游 RBAC，也不
  删除由集群协调器共享维护的项目 RoleBinding。
- 响应使用 `201` 和 `Cache-Control: no-store`，包含 `credential`、本次有界 `bindings` 和仅出现一次的
  `kubeconfig`；Schema 将 kubeconfig 标为敏感字段，日志和错误不得回显。

响应形状：

```json
{
  "credential": {
    "id": "tok_xxx",
    "name": "开发环境",
    "scopes": ["kube:read", "kube:write", "kube:connect"],
    "status": "active",
    "expiresAt": "2026-09-07T12:00:00Z",
    "createdAt": "2026-08-31T12:00:00Z",
    "bindingCount": 1
  },
  "bindings": [
    {
      "id": "kbd_xxx",
      "projectId": "prj_xxx",
      "runtimeClusterId": "clu_xxx",
      "applicationId": "app_xxx",
      "namespace": "project-demo",
      "contextName": "luna/prj_xxx/clu_xxx/app_xxx"
    }
  ],
  "kubeconfig": "apiVersion: v1\nkind: Config\n..."
}
```

不额外返回可复制的 `accessToken` 字段；唯一明文已经包含在一次性 kubeconfig 中，避免 Web 和 CLI 再
维护第二种敏感输出路径。

该操作由 Luna CLI 专用 kubeconfig 适配器消费，不生成会把明文写到普通 stdout 的通用命令，并在
Agent 集中 deny map 中记录“凭据材料必须由用户直接创建”的稳定原因。

#### `GET /api/v1/kube-credentials`

`operationId=listKubeCredentials`。分页列出当前用户的 Credential，支持：

```text
search
status=active|expired|revoked
page/pageSize
sortBy=name|createdAt|expiresAt|status
sortOrder=asc|desc
```

列表项只返回 ID、名称、规范化 Scope、状态、到期时间、创建时间和 `bindingCount`，不得内嵌无界 Binding
数组，也不得返回 TokenHash、明文 Token 或 kubeconfig。响应使用统一
`items/page/pageSize/sortBy/sortOrder/total/totalPages` 分页结构。

#### `GET /api/v1/kube-credentials/{credentialId}/bindings`

`operationId=listKubeCredentialBindings`。只允许 Credential 所有者分页读取 Context 元数据，列表项包含
Binding ID、项目、运行集群、可空应用、权威 namespace、Context 名和创建时间。namespace 在请求时从
Project 权威读取，不作为 Binding 快照持久化。
响应同样使用统一分页结构。

#### `DELETE /api/v1/kube-credentials/{credentialId}`

`operationId=revokeKubeCredential`。只允许所有者撤销，更新 `revoked_at` 并返回 `204`；对已撤销的同一
Credential 重复调用仍返回 `204`，未知或不属于当前用户的 ID 统一返回 `404`。本方案不提供编辑
Scope、追加 Context、单独重新下载或找回明文；需要改变内容时创建新 Credential 后撤销旧 Credential，
保持生命周期简单且可审计。

### 5.3 运行集群网关接口

#### `GET /api/v1/runtime/clusters/{clusterId}/kube-gateway`

`operationId=getRuntimeClusterKubeGateway`。返回：

```json
{
  "enabled": true,
  "extraResourceRules": [],
  "status": "ready",
  "observationCode": "",
  "lastCheckedAt": "2026-08-31T12:00:00Z"
}
```

`enabled` 和规则来自数据库期望配置；`status/observationCode/lastCheckedAt` 每次从 Kubernetes 实时观察，
使用 `Cache-Control: no-store`，不得落数据库或以 Query 长缓存维持旧状态。稳定状态为
`disabled|reconciling|ready|unavailable`。

#### `PUT /api/v1/runtime/clusters/{clusterId}/kube-gateway`

`operationId=updateRuntimeClusterKubeGateway`。全量替换启用状态与额外规则：

```json
{
  "enabled": true,
  "extraResourceRules": [
    {
      "apiGroup": "example.io",
      "apiVersion": "v1",
      "resource": "widgets",
      "subresources": ["status"],
      "verbs": ["get", "list", "watch"],
      "action": "deployment:read"
    }
  ]
}
```

规则总数上限为 50；GVR 必须经 Discovery 证明为 namespaced，verb 和 Action 必须来自白名单，固定拒绝项
不可被覆盖。接口保存期望配置并投递只携带 `clusterId` 的幂等 reconcile 任务，返回 `202`；Worker 执行时
重新读取最新期望，安装或回收 ServiceAccount、ClusterRole、ClusterRoleBinding 和 RoleBinding。任务投递
失败时返回 `503 kube_gateway.enqueue_failed`，已保存的期望配置保留为待协调状态；相同 PUT 即使配置未变
也必须重新投递，不能把它优化成无操作。
周期扫描同时协调已启用与已停用集群：前者自愈安装，后者自愈回收，避免停用请求在 Redis 短暂不可用时留下上游 RBAC。
已启用网关或上游清理尚未被实时观察证明完成时，普通 RuntimeCluster 更新必须拒绝替换 kubeconfig 或切换为非 Kubernetes 类型；管理员先停用网关、等待状态为 `disabled`，再更换连接。这个窄约束用于防止旧集群残留 ServiceAccount/RBAC，不引入双集群迁移状态机。

协调器根据 RuntimeCluster 当前 Scope 和活动项目清单预先收敛项目 RoleBinding；项目创建/删除、项目
namespace 或集群 Scope 变化时复用同一 `clusterId` 任务，其中 namespace 变化还先使旧 Binding 失效。
Credential 创建只检查所需 RoleBinding 的实时就绪状态，不在用户事务中创建或删除共享上游 RBAC。

该任务禁止使用 `asynq.Unique`：每个触发都必须留下后继任务。Runner 先取得以 `clusterId` 为键的
PostgreSQL session advisory lock，再重新读取集群配置、Scope、活动项目、项目 namespace 和中央 Catalog，
构造完整 `GatewayAccessSpec` 后调用 Provider；并发任务因此按集群串行，较新的任务最终总会以最新期望
覆盖旧结果。Worker 还每 5 分钟为全部 `delete_status=active` 的集群补投一次任务，修复 API 保存成功但 Redis 短暂不可用等
漏投情形。Provider 不查询平台数据库，只消费完整 Spec，并把 Spec hash 写入自有 Kubernetes 对象 annotation；
GET 以当前期望 Spec 与上游对象/annotation 实时比较 `ready|reconciling|unavailable`，不把 observed
generation 或当前状态写回数据库。

PUT 的 `202` 响应返回已保存的 `enabled/extraResourceRules` 和 `status=reconciling`；客户端随后调用 GET
取得实时终态。管理 API 使用稳定错误码：`kube_credential.scope_invalid`、
`kube_credential.context_invalid`、`kube_credential.not_found`、`kube_gateway.rule_invalid`、
`kube_gateway.disabled`、`kube_gateway.reconciling`、`kube_gateway.unavailable` 和
`kube_gateway.enqueue_failed`。Web 与 CLI 按 code
本地化，后端 message 只用于诊断；Kubernetes 协议入口仍只返回 `metav1.Status`。

#### RuntimeCluster 删除收口

运行集群删除复用现有 `resource:cleanup` 任务，不新建另一套删除 API 或队列协议。具体状态机为：

1. `DELETE /api/v1/runtime/clusters/{clusterId}` 保留现有的 DeploymentTarget 引用检查；通过后把
   `delete_status` 置为 `deleting`、写入 `delete_started_at`，在同一数据库事务中删除该集群的
   KubeAccessBinding，并把 `kube_gateway_drain_until` 设为当前时间加 35 秒，随后投递
   `ResourceCleanupPayload{resourceType:"runtime_cluster", resourceId:clusterId}`。此时不软删除行、不删除
   `kubeconfig_ref`，但普通选择器、Credential 创建和新 Kube 请求都把该集群视为不可用。
2. 投递失败时返回 `503 runtime_cluster.cleanup_enqueue_failed`，并把状态置为
   `delete_failed`；用户对同一 DELETE 重试即重置为 `deleting` 并重投，不增加“重试删除”特殊接口。
3. `resource:cleanup` 是删除状态机唯一 owner。API 长连接不在 Worker 进程内，因此 Worker 不声称
   “直接关闭”它们；Binding 删除和集群状态使每 30 秒的 StreamAuthorizer 复核失败，各 API
   副本自行取消上游 Context。35 秒是这一已有收敛窗口加 5 秒余量，不新增 Redis Pub/Sub
   或跨进程连接注册表。
4. Worker 使用与网关协调相同的集群 advisory lock，以 `Unscoped` 回读行和 Secret
   引用；若尚未到 `kube_gateway_drain_until`，用可取消 timer 等待剩余时间，超过截止时间后再按
   Catalog 回收所有 Luna 归属的 ServiceAccount、ClusterRole/Binding 和项目 RoleBinding。对象不存在按幂等
   成功处理；固定名称被非 Luna 对象占用时仍不删除它。
5. 只有 Provider 确认上游已无自有网关资源后，才原子写入
   `kube_gateway_cleanup_completed_at`。上游不可达、Secret 引用非空但无法解析，或权限不足时任务失败重试，
   保留行和 Secret；从未配置 kubeconfig 且无 Binding 的集群可直接记录上游清理完成。
6. 上游清理阶段已持久化后才删除 kubeconfig Secret；即使 Secret 删除后数据库终态写入失败，
   重试也能根据 `kube_gateway_cleanup_completed_at` 跳过需要凭据的上游阶段。最后在数据库事务中写入
   `deleted/delete_finished_at`、删除 ScopedResourceProjectBinding 并软删除 RuntimeCluster。

Asynq 常规重试用尽后复用现有 `markResourceCleanupFailed`；
`recoverStaleResourceCleanups` 额外扫描超时的 RuntimeCluster `deleting` 行并重投。
`kubectl_gateway_runner` 看到非 `active` 集群立即退出，不安装、回收或接管删除；
`kubectl_gateway_scheduler` 只补投活动集群的期望配置协调，不代理、不跳过这个删除恢复链路。

#### RuntimeCluster 活动性契约

迁移后只有 `delete_status=active` 的 RuntimeCluster 可以承载新业务副作用；`deleting`、
`delete_failed`、`deleted` 都不是可用集群，即使 Secret 尚在也不得继续部署、构建、网关协调、
资源观察或 Kube 请求。新增 `internal/runtimecluster/state.go` 提供状态常量、`IsActive` 和统一
GORM `ActiveScope`；除管理列表、DELETE/重试和 `resource:cleanup` 外，所有 RuntimeCluster 查询必须复用该入口。
契约测试覆盖 API 资源/终端/网关/卷、Worker 部署/构建/计费和 SystemComponent 等现有直接查询，
防止新入口绕开状态边界。

RuntimeCluster 管理列表和单项响应增加 `deleteStatus/deleteStartedAt/deleteFinishedAt`、
稳定 `deleteObservationCode` 和 `kubeGatewayEnabled`；不回显内部 `delete_message`、drain/cleanup 阶段时间或 Secret 引用。
管理列表可展示 `deleting/delete_failed`，但 Web 的集群、部署和 kubeconfig 候选器只允许选择 `active`；
`delete_failed` 行只提供重试删除和诊断入口。最终权威判断仍在后端 `ActiveScope`，不依赖前端过滤。

DELETE 契约与现有异步清理风格一致：成功记录状态并投递后返回无 body 的 `204`；正在删除返回
`409 runtime_cluster.delete_in_progress`，仍有 DeploymentTarget 引用返回 `409 runtime_cluster.in_use`，投递失败返回
`503 runtime_cluster.cleanup_enqueue_failed`，未知或已完成软删除的 ID 返回 `404 runtime_cluster.not_found`。

新增管理接口全部进入平台 OpenAPI。Credential 四个操作全部排除 Agent；网关 GET/PUT 作为管理员安全
配置也先进入集中 deny map，不向模型暴露原始或间接 Kubernetes 权限扩张入口。

### 5.4 Kubernetes 特殊协议入口

```text
GET|POST|PUT|PATCH|DELETE|HEAD /kube/v1/bindings/{bindingId}
GET|POST|PUT|PATCH|DELETE|HEAD /kube/v1/bindings/{bindingId}/*kubePath
```

路由使用显式方法集合，不注册无边界 `ANY`，并拒绝 `CONNECT`、`TRACE`、未知方法和未知 Upgrade。两个
通配入口不进入平台 OpenAPI、Web API Client、Luna CLI 自动命令或 Agent Tool Catalog；它们由独立
Kubernetes 协议契约和真实 kubectl E2E 验收。SelfSubjectAccessReview、SelfSubjectRulesReview 和
SelfSubjectReview 也是通配入口内部的本地实现，不另建平台 JSON API。

路由接线顺序是协议正确性的一部分：`router.go` 先安装 `/kube/` 前缀分流，再进入通用
CORS `OPTIONS` 短路和静态 UI。Gin 开启 `HandleMethodNotAllowed`，统一 Kube NoRoute/NoMethod
写入 `metav1.Status`；`static_ui.go` 的全局 NoRoute 在尝试 SPA fallback 前必须先委派
`/kube/` 请求。全局 NoRoute 只注册一次：`kube_proxy_handler.go` 只提供 responder，
`registerStaticUI` 组合 Kube 分支和 SPA fallback；即使 `staticFS=nil` 也仍安装 Kube NoRoute。这样未知
Kube 路径返回 404，`CONNECT/TRACE/OPTIONS` 和其他未注册方法
返回 405 并带 `Allow`，未知或格式错误的 Upgrade 返回 400，都不落入平台 JSON envelope、
Swagger、静态 Web UI 或 HTML 404。HEAD 使用与 GET 相同的认证授权和响应 header，但统一丢弃 body。

现有接口同步修改：

- `GET /api/v1/meta` 增加 `features.kubectlGateway`，供 Web 与 CLI 做服务端能力判断。
- RuntimeCluster 普通响应按第 5.3 节增加删除状态投影和持久化的 `kubeGatewayEnabled` 摘要，
  但不在集群列表中批量探测网关或 Kubernetes 实时状态。
- 应用删除只撤销该应用 Binding、长连接和应用标签资源，不删除项目共享 RoleBinding；项目删除、项目
  namespace 变化、集群解绑或网关禁用才协调对应 RoleBinding。
- 运行集群删除按第 5.3 节的可恢复删除状态机收口，上游资源清理完成前不删除 Secret。
- DeploymentTarget、部署配置 Bundle 和应用模板 OpenAPI 删除可写 namespace override。

## 6. Kubernetes RequestInfo 与默认拒绝

网关不能按 HTTP Method 粗略授权，必须解析：

```text
verb
apiGroup
apiVersion
resource
subresource
namespace
name
isResourceRequest
isWatch
isUpgrade
```

标准路径包括：

- `/version`、`/api`、`/api/{version}`、`/apis` 和 `/apis/{group}/{version}`。
- `/openapi/v2`、`/openapi/v3` 和 `/openapi/v3/*`。
- Core 与 named group 的 namespaced collection、object 和 subresource 路径。

路径解析必须基于原始转义路径，拒绝 `..`、重复分隔符、编码后的 `/`、反斜杠、NUL、绝对 URI 和
路径越界；不能先 `path.Clean` 再授权。未知路径返回 Kubernetes 404 Status；已知路径上的未支持方法
返回 405；未知或格式错误的 Upgrade 返回 400；已正确解析但不在资源策略中的 GVR/verb/subresource
返回 403。只有对象存在性或所有权隐藏场景才故意返回 404，避免把协议错误与授权拒绝混为一类。

Kubernetes verb 映射：

- GET 单对象为 `get`，集合为 `list`，`watch=true` 为 `watch`。
- HEAD 按对应 GET 路径执行 `get` 或 `list` 的同等鉴权，只返回响应头；HEAD 不允许 Watch、Logs follow
  或任何 Upgrade。
- POST 集合为 `create`，PUT 为 `update`，PATCH 为 `patch`。
- DELETE 单对象为 `delete`，集合为 `deletecollection`。
- Exec、Attach 和 Port-forward 等升级子资源为 `connect`。

## 7. 资源、Action 与角色边界

每个资源请求必须同时满足：

1. Credential 具有对应 `kube:*` Scope。
2. 用户仍启用，且仍是 Binding 项目的有效成员。
3. 项目角色经 `ProjectRoleAllows` 允许对应平台 Action。
4. 项目仍可使用目标运行集群，应用仍属于该项目。
5. GVR、subresource、verb 和对象所有权通过中央资源策略。

### 7.1 内置资源目录

| 类别 | 资源 | 主要 Action |
| --- | --- | --- |
| Workload | Pod、Deployment、StatefulSet、ReplicaSet、Job、CronJob、HPA、PDB | `deployment:read/update/restart/delete/exec` |
| Network | Service、Endpoints、EndpointSlice、Ingress、NetworkPolicy | `deployment:*` 或 `gateway:*` |
| Config | ConfigMap、Secret | `secret:read_summary/view_value/update` |
| Storage | PVC、项目可用 StorageClass 合成视图 | `volume:read/write/delete` |
| Gateway API | HTTPRoute、GRPCRoute | `gateway:read/manage` |
| Observation | Event、PodMetrics | `cluster:read` 或 `deployment:read` |
| Namespace | 当前项目 Namespace 合成视图 | `project:read` |
| Platform config | ResourceQuota、LimitRange、ServiceAccount 只读 | `cluster:read` |

目录按 GVR、subresource 和 verb 精确声明，不按整行统一放开。Endpoints/EndpointSlice、Event、
ResourceQuota、LimitRange 和 ServiceAccount 仅允许 Get/List/Watch，PodMetrics 仅允许 Get/List；Service
写入仍只允许第 10 节的 ClusterIP；Namespace、StorageClass 使用第 8 节的本地只读视图。上游 ClusterRole 中 Exec/Attach/
Port-forward 同时包含 WebSocket GET 与 SPDY POST 所需的 Kubernetes RBAC `get/create`，平台层仍统一
求值为 `connect`，且任何规则都不能使用 apiGroup、resource 或 verb `*`。

目录还必须显式记录 `bindingScope=project|application|both` 和集合隔离策略，不得对所有只读资源
猜测同一种标签行为：

- Event、ServiceAccount、ResourceQuota 和 LimitRange 是项目/namespace 共享资源，Get/List/Watch 只允许
  项目 Binding；应用 Binding 固定返回 403。ResourceQuota 即使意外带 application-id，其 `status.used`
  仍是整个 namespace 汇总，不因标签改成应用资源；隐式 `default` ServiceAccount 的引用例外也不
  授予应用 Binding 读取 ServiceAccount API 的权限。
- PodMetrics 的项目 Binding Get/List 可直接代理；应用 Binding List 强制合并 application-id selector，
  并只支持已纳入兼容矩阵、会保留 Pod 标签且执行 label selector 的 metrics provider。网关保存客户端
  Accept，向上游强制 `Accept: application/json` 与 `Accept-Encoding: identity`；只有 2xx 且 Kind 为
  PodMetricsList 的响应进入特例，使用 `16 MiB + 1` 的硬限制读取未压缩 canonical JSON，并校验每个
  PodMetrics 的 namespace 和 application-id。全部通过后，使用与仓库 Kubernetes minor 一致的
  `k8s.io/metrics` 类型、Kubernetes serializer、PodMetrics Table converter 和
  PartialObjectMetadata 投影按原客户端 Accept 重新编码 JSON、YAML、Protobuf、CBOR 或 Table；客户端
  请求网关不支持的表示时返回 Kubernetes 406。重编码后重新生成 Content-Type、Content-Length、
  Content-Encoding 和 Vary，删除上游实体 ETag，仅保留 Warning、Retry-After、Audit-ID 等非实体元数据。
  任一条目失配、缺少标签、超限、Kind 错误或上游 2xx Content-Type 无法解码时，在写出任何字节前丢弃
  整份响应并返回 `503 kube_gateway.metrics_selector_unavailable`；不得输出部分条目或回退为全项目读取。
  这是一致性 fail-closed 验证，不是会改变 List 结果的响应后过滤。上游非 2xx Kubernetes Status 不进入
  解码流程，连同 Warning、Retry-After 和 Audit-ID 原样返回。单对象 Get 还要先读同名 Pod 并验证其
  应用归属。应用 Binding 的 PodMetrics HEAD 在鉴权后必须向上游内部执行对应 GET，完成同一归属校验、
  表示协商和实体 Header 计算后只写状态与响应头；不能因 HEAD 没有上游响应体而绕过校验，也不能把
  编码后的响应体写给客户端。
- Endpoints/EndpointSlice 继续使用第 8 节的 Service 标签传播契约；失配集群 fail closed。

Ingress、HTTPRoute 和 GRPCRoute 写入额外要求 `gateway:manage`，并复用中央 GatewayPolicy 校验项目可用
域名、允许的 IngressClass/parent Gateway、应用 Binding 同应用或项目 Binding 同项目的 backend/TLS
Secret，以及 namespace。跨 namespace
backend、ReferenceGrant、Gateway 写入及未配置的 parentRef 固定拒绝，避免通过原生对象绕过 Luna 的
域名和网关归属边界。

### 7.2 角色与能力矩阵

| 能力 | Kube Scope | 平台 Action | 项目角色 |
| --- | --- | --- | --- |
| Discovery、OpenAPI、普通读取、Watch、Wait、Top | `kube:read` | 资源对应的 `*:read` | Viewer 及以上 |
| Pod Logs 和 `logs -f` | `kube:read` | `deployment:read` | Viewer 及以上 |
| Create、Update、Patch、Apply、Scale、Autoscale | `kube:write` | `deployment:update` 等资源 Action | Developer 及以上 |
| 删除 Pod、Eviction、Rollout Restart | `kube:write` | `deployment:restart` | Developer 及以上 |
| 删除工作负载、Service、配置或卷 | `kube:write` | 对应 `*:delete` Action | 以现有 Action 角色矩阵为准 |
| Exec、Attach、Port-forward、cp | `kube:connect` | `deployment:exec` | Developer 及以上 |
| Ephemeral container Debug | `kube:write` + `kube:connect` | `deployment:update` + `deployment:exec` | Developer 及以上 |
| Secret 原值读取 | `kube:read` | `secret:view_value` | Owner、Admin |
| Secret 写入和删除 | `kube:write` | `secret:update` | Owner、Admin |
| NetworkPolicy 写入 | `kube:write` | `cluster:manage` | Owner、Admin |
| `auth can-i`、`can-i --list`、`whoami` | `kube:read` | 本地求值 | 所有有效项目成员 |

Credential 创建时只能选择当前用户可授予自己的能力组合，但这只是创建校验；后续每个请求仍按当前
项目角色和 Action 重新判断，角色降低后旧 Credential 不保留原权限。

Secret 使用 Kubernetes 原生响应，不能伪装成已脱敏的完整兼容：

- Viewer、Developer 不允许 Secret `get/list/watch`。
- Owner、Admin 同时具有 `secret:view_value` 时才允许读取原值。
- Secret 创建、更新、Patch、Apply 和删除同时要求现有 `secret:update`。

本事项不新增 `secret:delete`；Secret 删除明确复用现有 `secret:update`，并照常要求 `kube:write`、Owner 或
Admin。`secret:view_value` 只控制直接调用 Kubernetes Secret API；拥有 `deployment:exec` 的 Developer
可能读取目标容器已经注入的环境变量或挂载内容，这是 Exec 能力本身的风险，Credential 创建页必须明确
提示，不能把 Secret API 的拒绝描述成对容器内 Secret 的完整保密。

额外 CR 规则只允许平台管理员为明确 namespaced 的 GVR 配置允许 verb 和现有平台 Action；规则数量
设定有限上限。额外规则不能覆盖固定拒绝列表，也不能允许集群级资源。

### 7.3 固定拒绝

以下能力即使是平台管理员使用项目 Binding 也拒绝：

- Node、PV、Namespace 写入、DaemonSet 和 Node Debug。
- Role、RoleBinding、ClusterRole、ClusterRoleBinding 及 ServiceAccount 写入或 token 子资源。
- CRD 定义、APIService、MutatingWebhookConfiguration、ValidatingWebhookConfiguration。
- CSR、TokenReview、普通 SubjectAccessReview、LocalSubjectAccessReview。
- Pod binding、任意资源 `proxy`、namespace `finalize`、集群级 Gateway 和 StorageClass 写入。
- `healthz`、`readyz`、`livez`、`metrics` 等 kube-apiserver 管理端点。
- 客户端 `Impersonate-*`，因此不支持 `kubectl --as`。

平台管理员若需要集群管理，使用现有运行集群管理能力或独立管理员 kubeconfig，不通过项目 Binding
扩大权限。

## 8. Namespace、应用过滤与所有权

Namespace 是租户硬边界，标签是对象所有权、应用过滤、观察、清理和计费边界：

- 每个 Binding 固定一个项目和该项目的权威 namespace；`-n` 不能改变边界，`-A` 一律拒绝。
- 应用 Binding 对 List、Watch 和 DeleteCollection 强制注入 application-id selector；项目 Binding
  允许访问项目级对象和该项目全部应用对象。
- Create/Apply 新对象时由网关注入 `managed-by`、project-id、management-source；应用 Binding 同时
  注入 application-id。
- 控制器生成对象的归属必须从源头闭环：Deployment 注入 `metadata` 和 `spec.template.metadata`；
  StatefulSet 额外注入 `spec.volumeClaimTemplates[*].metadata`；Job 注入自身和 Pod Template；CronJob 同时注入
  `metadata`、`spec.jobTemplate.metadata` 和 `spec.jobTemplate.spec.template.metadata`。这使生成的
  ReplicaSet、Pod、Job 和 StatefulSet PVC 继承项目/应用/management-source 标签，并进入同一列表、
  清理和 PVC 保留策略。
- Service 自身注入完整归属标签；本方案验证的 Kubernetes minor 中，标准 Endpoints 和
  EndpointSlice controller 会从 Service 复制非保留标签。真实 E2E 必须对每个支持 minor 断言两类派生对象
  保留 Luna 标签；若某集群的 controller 不符合，应用 Binding 对该派生资源返回 unavailable/拒绝，
  不改为响应后过滤。
- 请求显式携带冲突的保留标签返回 422；Update、四类 Patch 和 Apply 均不得删除或修改保留标签。
- List、Watch 和 DeleteCollection 把平台强制 selector 与用户 selector 做逻辑 AND，不能只在响应后
  过滤，否则会破坏分页、continue token、resourceVersion 和 Watch 一致性。
- 单对象 Get、Logs、Connect、Update、Patch 和 Delete 必须先权威读取对象并校验 namespace 与标签；
  范围外对象统一返回 NotFound，避免通过存在性差异探测其他项目。
- `/scale`、`status`、`ephemeralcontainers`、`eviction` 等子资源从父对象继承所有权检查。

Event 本身通常没有可靠的应用标签，且 message、note、regarding/related 等字段可能包含同 namespace
其他应用的信息。因此 Event Get/List/Watch 只允许项目 Binding，并依赖项目独占 namespace 作为硬边界；
应用 Binding 固定返回 403，不通过响应后筛选模拟应用级 Event。应用 Binding 下 `kubectl describe` 的
主对象仍按普通读取规则工作，但 Events 小节不可用；需要查看事件时使用项目 Binding。

Namespace 和 StorageClass 是两个受控的集群级例外：Namespace List/Get 只返回当前项目 namespace，
StorageClass List/Get 只返回 RuntimeCluster 资源策略允许项目使用的 class。它们由平台本地合成并兼容
JSON、Table、Protobuf 和协商到的 CBOR 响应，不给上游网关 ServiceAccount 增加全局
Namespace/StorageClass 读取权限；
对应 Watch 和全部写操作固定拒绝。

## 9. 读写与期望状态一致性

### 9.1 平台创建资源

平台数据库和发布配置继续是平台创建资源的期望状态。用户具有对应写权限时，kubectl 的 Update、
Patch、Apply、Scale、Rollout 和 Delete 可以立即改变 Kubernetes 当前状态，但：

- 网关不把原始对象反向翻译或回写成 DeploymentTarget、Release、Secret Set 或 GatewayRoute。
- 下一次平台发布、回滚、重建或协调可按数据库期望配置覆盖 kubectl 的运行态修改。
- kubeconfig 下载页和公开文档必须明确这一覆盖语义。
- 写操作仍经过工作负载安全校验、项目角色、业务 Action、审计和实时权威回读。

这避免为了兼容 kubectl 建立第二套通用 Manifest 数据库或复杂的双向同步。

### 9.2 kubectl 创建资源

kubectl 新建资源使用 `management-source=kubectl`：

- Kubernetes 保存其期望和当前状态，平台数据库不复制 Kubernetes status 或实时对象。
- Worker 不对其进行配置回写，但项目/应用删除和孤儿清理必须按保留标签覆盖这些资源。
- kubectl 创建的 PVC 仍遵守现有应用/项目删除数据策略：`deleteData=false` 时进入保留卷流程，不因
  management source 不同被直接删除；用户显式 `kubectl delete pvc` 则要求 `volume:delete` 并按原生结果
  执行。
- 平台集群资源页、资源观察和运行计费按项目标签纳入这些资源。
- 与平台保留名称或现有平台对象冲突时拒绝创建或接管。

### 9.3 Kubernetes 原生写语义

完整保留：

- `dryRun`、`fieldManager`、`force`、`fieldValidation`、resourceVersion 和 managedFields。
- Foreground、Background、Orphan propagationPolicy、gracePeriod 和删除 precondition。
- Client-Side Apply、Server-Side Apply、`kubectl diff`、Edit 和 Replace。
- JSON Patch、JSON Merge Patch、Strategic Merge Patch 和 Apply Patch Content-Type。
- `kubectl apply --prune`，但 DeleteCollection 和 Prune 仍受强制 selector 与逐对象删除 Action 约束。

`status` 默认只读；只有额外 namespaced CR 规则显式允许且资源为 kubectl 管理来源时才能写入。

写入校验不得自行实现一套 Kubernetes Patch 引擎。Create 和 Update 在保留原 Content-Type 的前提下向
对象及 Pod Template 注入保留标签；Apply 先在保留 field manager 的前提下完成同样注入。JSON、Merge、
Strategic Patch 先拒绝直接命中保留字段的操作。随后 Create、Update 和四类 Patch/Apply 都以最终将发送
的同一请求执行上游 `dryRun=All`，从 Admission 合并后的最终对象校验 namespace、所有权标签、PodSpec、
传递引用和 WorkloadPolicy，通过后才执行真实请求。客户端本身已指定 dry-run 时只返回第一次结果。
这样仍需覆盖两次请求间的 resourceVersion、generateName 和 Admission 结果差异测试，但不靠不完整的
本地 Patch 模拟器决定最终对象。

## 10. 工作负载安全策略

API 与 Worker 必须复用同一个租户工作负载校验器，避免平台表单与 kubectl 写入形成两套安全标准。
至少拒绝：

- `hostNetwork`、`hostPID`、`hostIPC`、`hostPath`、`privileged` 和 `allowPrivilegeEscalation=true`。
- 危险 Linux capability、`hostPort`、`nodeName`、未授权 ServiceAccount 和 Node Debug。
- Service 的 NodePort、LoadBalancer、ExternalName 和 `externalIPs`；外部入口继续使用 GatewayRoute。
- 绕过项目资源配额、集群 RuntimeCluster 资源策略和平台 Secret 引用边界的配置。

WorkloadPolicy 必须接收带 `context.Context` 的 `PolicyContext` 和 `ReferenceResolver`，在最终对象上解析
传递引用，不能只检查工作负载自身标签。至少覆盖：

- `env.valueFrom.secretKeyRef`、`envFrom.secretRef`、Secret/Projected Volume 和 `imagePullSecrets`。
- ConfigMap KeyRef、`envFrom.configMapRef`、ConfigMap/Projected Volume。
- PVC、`serviceAccountName`、显式 Projected ServiceAccount Token 和 CSI Secret provider。
- HPA `scaleTargetRef`、Ingress backend、HTTPRoute/GRPCRoute `backendRefs` 按 Binding 边界解析：
  应用 Binding 只能引用同一 application-id，项目 Binding 可以引用同项目其他应用，但两者都不得
  跨 namespace 或引用无项目归属对象。
- 应用 Binding 创建或修改 Service、PDB、NetworkPolicy 时，把 application-id 与用户的 selector 做逻辑
  AND，防止选择同 namespace 的其他应用 Pod；项目 Binding 保留项目内跨应用编排能力。
- 应用 Binding 下，被引用 Secret、ConfigMap、PVC 和其他 namespaced 对象必须具有相同 application-id；
  项目 Binding 允许同项目标签对象，但仍不能跨 namespace 或引用无项目归属的外部对象。

显式 Projected ServiceAccount Token 和任意 CSI Secret provider 在本方案中固定拒绝。普通应用的
ServiceAccount 规则按下文处理。ReferenceResolver 从绑定集群实时读取对象 metadata，不把引用结果写入
数据库或缓存；平台发布链路在生成 Secret/ConfigMap/PVC 后执行同一最终校验，kubectl 链路在 dry-run
结果上执行。

项目 namespace 同时启用兼容现有应用的 Kubernetes Pod Security Admission：

```text
pod-security.kubernetes.io/enforce=baseline
pod-security.kubernetes.io/enforce-version=v1.35
pod-security.kubernetes.io/warn=restricted
pod-security.kubernetes.io/warn-version=v1.35
pod-security.kubernetes.io/audit=restricted
pod-security.kubernetes.io/audit-version=v1.35
```

`v1.35` 是本方案验证范围中的最低 Kubernetes minor；兼容范围调整时统一更新。上述标签加入
`ProjectNamespaceLabels`，由普通 namespace Ensure 和网关协调器补齐既有 Luna 项目 namespace。上游
Admission 继续作为最终判断；平台不得吞掉其 Status、Warning 或字段错误，也不修改非 Luna 归属 namespace。

Namespaced `kubectl debug` 只允许受限 ephemeral container 或复制 Pod 模式；Node Debug 固定拒绝。
`ephemeralcontainers` 子资源同时要求 `kube:write + kube:connect`、`deployment:update + deployment:exec`
和 Developer 以上角色。在转发 `ephemeralcontainers` PATCH 前必须先读父 Pod，执行对象归属检查并调用
`runtimeaccess` 的 Web Console 策略：平台来源 Pod 同时检查项目与 DeploymentTarget 开关，kubectl 来源 Pod
检查项目开关；禁用时在产生任何上游副作用前返回 403。`kubectl debug --copy-to` 在协议上只是
普通 Pod Create，无法可靠识别为特殊 connect 动作，因此仍按普通 Pod Create 要求
`kube:write + deployment:update`；用户随后连接复制 Pod 时再要求 `kube:connect + deployment:exec`。

这不是只在 Kube 网关入口增加校验。现有平台部署链路仍允许用户配置
`allowPrivilegeEscalation=true`、任意 `capabilities.add`、任意 ServiceAccount，以及 NodePort 或
LoadBalancer Service；如果原样保留，用户可从平台表单绕过上面的统一策略。本事项必须同步修改 API
输入校验、Worker Spec 生成、Kubernetes Provider 和 Web 表单，使两条入口执行相同规则：

- `allowPrivilegeEscalation` 固定为 `false`，不再接受开启值。
- 本方案不允许增加 Linux capability，继续允许 `capabilities.drop`；以后如确有业务需求，只能在中央
  WorkloadPolicy 增加有限安全白名单，不能由两个入口分别判断。
- Service 只允许 `ClusterIP`；外部流量继续由 Luna GatewayRoute 管理。
- 普通应用的 Create 原始输入要求 `serviceAccountName` 缺失或为空，并显式设置
  `automountServiceAccountToken=false`；Kubernetes DryRun/Admission 可把最终对象的空值默认化为
  `serviceAccountName=default`，共享策略只在证明该值来自默认化时接受它。Update/Patch/Apply 未设置或
  未改动该字段时可保留现有的隐式 `default`；Create 中主动填写 `default` 或任何其他非空
  名称仍拒绝。只有平台
  明确批准的内部计划可以引用当前项目 namespace 的专用 ServiceAccount，且仍默认关闭
  Token 自动挂载。

“平台明确批准”仅指 SystemComponent 安装记录等可信内部计划显式声明的专用 ServiceAccount；普通应用
输入、Manifest annotation 或用户可写标签不能把 ServiceAccount 变成批准状态。现有
`ensurePlatformApplicationDependencies` 不得再因为任意 DeploymentTarget 填了名称就创建 ServiceAccount
和 RBAC。内部调用把批准名称作为不可由请求绑定的 `PolicyContext.TrustedServiceAccounts` 传入；kubectl
入口始终传空集合。`PolicyContext` 还要携带从原始操作提取的 ServiceAccount 字段出处，
避免把 API Server 默认化出的 `default` 误判为用户授权输入；不得单看 DryRun 后的最终对象做决定。

隐式 `default` 是 ServiceAccount 引用标签规则的唯一例外：只要名称为 `default`、出处证明 Create
原始字段为空或 Update/Patch/Apply 没有改动该字段，且
`automountServiceAccountToken=false`，ReferenceResolver 可以不要求该 ServiceAccount 自身具有 Luna
项目/应用标签。这个例外不传递给其他引用：显式 Projected ServiceAccount Token 仍拒绝；Admission
从 default ServiceAccount 注入到最终 Pod 的 `imagePullSecrets` 必须逐个通过原有同应用/同项目
Secret 归属校验，任何无标签或越界 Secret 都使请求返回 422。

这些限制会改变既有平台部署输入，必须同步删除 OpenAPI 与前端中的无效选项，并为旧期望配置返回稳定
校验错误；项目尚未发版，不保留继续生成高风险工作负载的兼容分支。

## 11. Discovery、OpenAPI 与普通 HTTP

Discovery 和 OpenAPI 从绑定集群读取并保持协议兼容：

- 支持 `/version`、legacy 与聚合 Discovery、OpenAPI v2/v3。
- 保留 Accept、Content-Type、Table、PartialObjectMetadata、Protobuf、CBOR、gzip、ETag、Vary、
  Warning、Retry-After 和 Audit-ID。
- kubeconfig 使用 URL path prefix 时，必须重写 OpenAPI v3 `serverRelativeURL` 和可能泄露上游地址
  的 Location；不能把上游 Host 暴露给客户端。
- Discovery 可以展示最终会被网关拒绝的资源类型；执行授权仍以 Authorizer 为准，不维护高成本的
  per-user OpenAPI 裁剪。
- 当前资源响应使用 `Cache-Control: no-store`；Discovery/OpenAPI 可沿用协议缓存。

平台自身拒绝必须返回 `metav1.Status`，不能套平台 JSON envelope：

- 400：路径、查询参数、Content-Type 或协议请求格式无效。
- 401：凭据无效、过期或撤销。
- 403：Scope、角色或资源策略拒绝。
- 404：对象不属于 Binding 范围。
- 405：Kube 前缀下的 HTTP Method 不受支持。
- 406：客户端只接受该资源不支持的响应表示。
- 409：resourceVersion、Apply ownership 等冲突。
- 413：请求体超过限制。
- 422：namespace、标签或工作负载校验失败。
- 429：请求或连接超过限制。
- 503/504：集群、Secret Store、TokenRequest 或上游不可用/超时。

除 OpenAPI/Location 安全重写、第 8 节本地合成资源和第 7.1 节应用 Binding PodMetrics 2xx
校验重编码外，上游 Status 和成功响应原样返回，不翻译为平台错误码；PodMetrics 上游非 2xx Status
也始终原样返回。

## 12. Watch、Logs 与 Upgrade 协议

### 12.1 Watch

- 不缓冲、不重排、不解码后重编码 Watch Event。
- 保留 `resourceVersion`、`resourceVersionMatch`、`allowWatchBookmarks`、`sendInitialEvents`、
  `timeoutSeconds` 和 410 Gone 语义。
- 强制限制最长连接时间，到期关闭并让 kubectl 重连，从而重新执行身份和成员校验。
- 客户端取消必须立即取消上游 Context，不留下 goroutine 或连接。

### 12.2 Logs

- 支持 container、previous、tail、since、timestamps、limitBytes 和 follow。
- `logs -f` 按流转发，不保存或扫描日志正文。
- 强制 `insecureSkipTLSVerifyBackend=false`，并保留上游 Status。

### 12.3 Exec、Attach、Port-forward 与 cp

- 使用 `k8s.io/apimachinery/pkg/util/proxy.UpgradeAwareHandler` 或同等官方协议实现，不手写帧协议。
- 同时兼容新版 WebSocket GET 与 SPDY POST 回退，正确转发 101 和 `Sec-WebSocket-Protocol`。
- Exec、Attach、Port-forward 同时要求 `kube:connect`、`deployment:exec` 和 Developer 以上角色。平台
  来源对象还必须满足现有 `project.WebConsoleEnabled && (target.WebConsoleEnabled == nil ||
  *target.WebConsoleEnabled)`；kubectl 来源对象没有 DeploymentTarget，只检查项目开关。长连接每次复核
  都重新执行该有效策略。
- `kubectl cp` 通过 Exec 与容器内 `tar` 工作；网关不得记录其 command 查询参数。容器缺少 `tar` 时
  保留 kubectl 标准失败语义。
- Port-forward 只允许 Binding 范围内 Pod，并将目标端口限制为容器声明端口或当前 Service 的目标端口。
- Token 撤销或过期、用户禁用、成员移除、项目停用、Binding 删除、应用归属变化或集群解绑后，
  StreamAuthorizer 取消上游 Context 并关闭连接。
- 权限复核使用固定 30 秒 ticker，并为凭据到期时间单独设置 timer；不新增 Redis Pub/Sub 撤权系统。
  因此普通请求立即使用最新权限，已建立流的撤权收敛上限为 30 秒，验收按这一明确窗口判断。

## 13. 本地授权 API

以下请求由平台根据同一 Authorizer 本地合成，不转发给上游网关 ServiceAccount：

- `authorization.k8s.io/v1 SelfSubjectAccessReview`。
- `authorization.k8s.io/v1 SelfSubjectRulesReview`。
- `authentication.k8s.io/v1 SelfSubjectReview`。

因此 `kubectl auth can-i`、`can-i --list` 和 `auth whoami` 必须反映 Luna 用户、当前 Binding、项目角色、
Token Scope 和资源策略，而不是错误返回上游 ServiceAccount 权限。普通 SubjectAccessReview、
LocalSubjectAccessReview、TokenReview 和查询他人身份固定拒绝。

WhoAmI 使用稳定的 `luna:<userId>` 和项目角色组，不返回邮箱、Access Token ID 或上游 ServiceAccount。

## 14. 代理安全边界

### 14.1 上游身份

- 集群管理 kubeconfig 只用于安装/更新固定网关 ServiceAccount、ClusterRole、RoleBinding 和请求
  短期 TokenRequest，不直接承载用户代理请求。
- 首次安装在创建 ServiceAccount 前幂等 Ensure `luna-system` namespace；新建时使用稳定的
  Luna system ownership 标签，已存在时只接受已明确归属 `luna-devops` 的 system namespace。同名但
  非 Luna 管理的 namespace 返回 `kube_gateway.system_namespace_conflict`，不接管。稳定身份使用
  `app.kubernetes.io/managed-by=luna-devops`、`luna.devops/scope=system`、`luna.devops/system=true` 和
  `luna.devops/system-component=luna-system`，不携带某一 RuntimeCluster ID，避免共享 namespace 被首个集群占有。
- 网关 ServiceAccount 的 ClusterRole 只声明支持的 namespaced 资源；每个项目 namespace 使用
  RoleBinding 限定作用域。Discovery 所需非资源 URL 使用独立、固定的最小 ClusterRoleBinding。
- 固定资源名为 `luna-system/luna-kubectl-gateway` ServiceAccount、
  `luna-kubectl-gateway-project` ClusterRole、`luna-kubectl-gateway-discovery` ClusterRole/
  ClusterRoleBinding，以及各项目 namespace 内的 `luna-kubectl-gateway` RoleBinding。
- Discovery ClusterRole 的非资源 URL 仅为 `/version`、`/api`、`/api/*`、`/apis`、`/apis/*`、
  `/openapi/v2`、`/openapi/v3`、`/openapi/v3/*`，verb 仅为 `get`；不使用 `/` 或 `*` 通配。
- 协调器只接管带 Luna 管理标签和匹配资源身份的对象；固定名称已被非 Luna 对象占用时返回
  `unavailable`，不得覆盖。禁用和删除也只回收已确认归属的对象。
- 禁用、集群删除和单个网关组件回收都不删除 `luna-system` namespace；它是可被其他 Luna
  SystemComponent 共享的容器，本功能只回收自己的带标签子资源。
- 每个新上游请求或流通过 TokenRequest 获取集群默认 API audience、10 分钟有效的 ServiceAccount
  Token；Token 只存在于当前请求内存，不落库、不写 Secret、不进入复用的管理 `rest.Config`。
- 项目解绑、删除或网关禁用时回收 RoleBinding；专用身份不得拥有 RBAC、CRD、Webhook、Node、PV、
  CSR 或任意 `*/*` 权限。

### 14.2 Header 与目标清洗

向上游删除并由服务端按需重建：

- `Authorization`、`Proxy-Authorization`、Cookie。
- 全部 `Impersonate-*`、`X-Remote-User/Group/Extra-*`。
- 客户端提供的 `Audit-ID`、`traceparent`、`tracestate` 和 `baggage`；Trace Context 只由平台在解析后按
  第 15 节重新注入。
- 外部 `Forwarded`、`X-Forwarded-*` 和所有 hop-by-hop Header。
- Upgrade 场景仅重建协议明确需要的 Connection、Upgrade 和 WebSocket/SPDY 协商 Header。

上游 Scheme、Host、端口、路径前缀和凭据只能来自 RuntimeCluster 与 Secret Store。用户不能提供
目标 URL、Host、cluster、namespace 或 kubeconfig。

### 14.3 有界资源使用

采用少量固定配置，并提供安全默认值：

| 配置 | 默认值 | 用途 |
| --- | --- | --- |
| 普通请求体上限 | 16 MiB | Apply、Patch 和对象创建 |
| 应用级 PodMetrics 校验缓冲 | 16 MiB 未压缩 canonical JSON | 上游强制 identity，在任何字节写回客户端前以 `limit+1` 完整校验 namespace 与应用标签，超限即 fail closed |
| 未认证请求 | 60 次/分钟/IP，burst 20 | 在数据库 Token 查询与 401 审计前抑制扫描 |
| 已认证普通请求 | 300 次/分钟/用户、1200 次/分钟/项目 | 保护 DB、Secret Store、TokenRequest 与 API Server |
| Watch 最长持续时间 | 30 分钟 | 定期重新鉴权 |
| Logs/Upgrade 最长持续时间 | 2 小时 | 收口遗留连接 |
| 流空闲超时 | 15 分钟 | 清理失活连接 |
| 流权限复核周期 | 30 秒 | 收敛 Token、成员和资源撤权 |
| 每用户/项目 Watch 并发 | 8 / 64 | 防止 Watch 连接耗尽 |
| 每用户/项目 Follow Logs 并发 | 4 / 32 | 防止日志流耗尽 |
| 每用户 Upgrade 并发 | 4 | 防止终端和端口转发滥用 |
| 每项目 Upgrade 并发 | 32 | 防止共享项目耗尽连接 |

当前平台没有覆盖全部普通 HTTP 的统一限流，因此 Kube Proxy 提供窄 `Limiter`：认证前按可信客户端 IP，
认证后按用户/项目和 transport class 判断。普通请求频率复用现有 Redis-backed rate limiter；Watch、
Logs 和 Upgrade 分开计数。数据流不落盘，不使用 Redis 或数据库缓存 Kubernetes 当前状态和
resourceVersion。连接计数先使用 API 进程内计数，避免引入分布式连接协调；多副本部署时总上限按副本数
线性放大，未完成对应容量验收前保持单 API 副本启用该功能。限流 key 和日志不得记录原始 Token、URL
或 Forwarded header，客户端 IP 只从服务端已配置的可信代理链解析。

## 15. 审计、日志、Trace 与 Metric

### 15.1 AuditLog

扩展 AuditLog 的可空结构化元数据，至少保存：

- actor、Credential、Binding、项目、应用、集群和 namespace。
- API Group、Version、resource、subresource、verb 和对象名。
- allow/deny、状态码、持续时间、请求 ID、Trace ID 和流终态。

Create、Update、Patch、Apply、Delete、Secret 原值读取、Exec、Attach、Port-forward、cp、Debug 和
有效 Credential 的全部拒绝必须持久审计；普通非敏感读取由 HTTP 访问日志和 Trace 覆盖。Watch、Logs
和 Upgrade 分别记录 open 与 close。六个管理 API 中，Credential 创建/撤销和网关 PUT 继续写普通平台
AuditLog，且不得把 kubeconfig、规则请求原文或 Token 写入 message/metadata。

会产生副作用或建立连接的请求先写 `attempt`；写入失败时 fail closed 并返回 503，不继续上游操作。终态
使用 `context.WithoutCancel` 派生 2 秒短超时完成，避免客户端断开同时取消 close 审计；终态更新失败时
保留 attempt 并输出可关联的安全错误日志/Metric。已认证拒绝在返回前记录；无效 Bearer 的 401 先经过
认证前限流，只记聚合日志和 Metric，不逐条写数据库，避免匿名请求放大 AuditLog。

禁止进入审计、日志和遥测：

- Authorization、Cookie、Kube Credential、上游 kubeconfig 和 ServiceAccount Token。
- 请求/响应正文、Secret、ConfigMap 原文、Apply Manifest 和 Patch。
- Exec command、stdin/stdout/stderr、cp 数据、日志正文和 Port-forward 数据。

### 15.2 Trace

```text
kube.gateway.request
  -> kube.gateway.authenticate
  -> kube.gateway.authorize
  -> database.*
  -> secret.resolve / kubernetes.token_request
  -> kubernetes.proxy
```

HTTP、Watch、Logs、WebSocket 和 SPDY 都继承 W3C Trace Context。流式请求只记录建连、首个上游
响应和终态，不为每个 Event、日志块或 Frame 创建 Span。Kubernetes Transport 继续隐藏真实 Host、
Path 和 Query，Span 名使用稳定模板。

Upgrade 直连不会经过普通 `RoundTripper.RoundTrip`，不能只依赖现有 Kubernetes Trace Transport。
只有 Kube 入口 `telemetry.go` 提取格式合法的入站 `traceparent/tracestate`、丢弃外部 baggage，
并建立唯一的脱敏 `kube.gateway.request` Server Span。Upgrade 包装器接收该 Context，只建立
`kubernetes.proxy` 子 Span；它从上游 Header 移除客户端提供的 Trace Context，再注入子 Span Context，
不得再建立第二个 Server Span。传给官方
`UpgradeAwareHandler` 的 Context 使用丢弃型 klog logger，阻止其 V(6) 输出真实 location、query、Header
或 Exec command；代理错误由平台 responder 转成安全 Status 和稳定错误日志。SPDY 与 WebSocket 都要
测试只有一个 Server Span、父子关系、失败 Span 状态和敏感值不出现。

现有通用 Gin 插桩会把实际 URL path 写入 HTTP Span。`/kube/v1/bindings/` 必须从通用 `otelgin`
入口排除，改由 Kube Proxy 创建固定名的 Server Span；否则 Binding ID、namespace 和对象名会进入遥测。
测试必须断言 Span name、attribute、日志和 Metric 均不出现 Binding ID、对象名、原始 path、query、
selector 或 Exec command。

### 15.3 Metric

至少提供请求总数、失败数、耗时、鉴权拒绝、上游失败、限流和 normal/watch/logs/upgrade 活跃连接。
Metric label 只使用有限的 transport、verb class、resource category、outcome 和 error code；用户、项目、
对象名、URL、Trace ID 和任意 CRD 名不得成为 label。

## 16. 前端、API、Worker、CLI 与 Agent 边界

### Web

- 项目空间提供“kubectl 访问”工具页，选择运行集群、可选应用、读/写/连接能力和有效期。
- 创建后只展示一次下载；账号设置分页列出并撤销 Credential。
- 运行集群设置提供完整网关启停和额外 namespaced GVR 配置。
- 页面明确展示平台资源的运行态覆盖语义、Credential 到期时间和 kubeconfig 文件保护提示。
- 所有标题、表单、提示、错误、状态和 aria-label 同步五语言 i18n。

### API

- 承担管理 API、认证、Binding、Authorizer、RequestInfo、所有权、工作负载校验、代理、流取消、
  审计和可观测。
- kubeconfig 下载与实时资源响应使用 `Cache-Control: no-store`。
- 普通管理接口保持 OpenAPI、Scope、分页、敏感字段和 Agent 排除契约。

### Worker

- 不参与同步 kube 请求，也不接收原始 Kubernetes 请求或流。新增的网关配置协调任务只携带
  `clusterId` 并在执行时回读最新期望；集群删除则复用现有 `resource:cleanup` 载荷和状态机，
  不由网关协调 Runner 兼任。
- 复用 namespace、保留标签、WorkloadPolicy 和删除清理规则。
- 安装、更新和回收网关 ServiceAccount、ClusterRole、ClusterRoleBinding、项目 RoleBinding，并实时观察
  网关就绪状态；任务载荷不得携带 kubeconfig、Token 或规则快照。
- 项目、应用删除和孤儿清理覆盖 `management-source=kubectl` 资源。
- 资源观察和计费按项目标签纳入 kubectl 创建的工作负载。

### Luna CLI

- 提供 `luna kubeconfig write` 和 `luna kubeconfig merge` 专用安全适配器；Credential 列出与撤销继续由
  普通 OpenAPI 命令提供。
- 合并 kubeconfig 前检测同名 Context 冲突，不盲覆盖用户现有配置。
- 文件以 `0600` 权限原子写入，Kube Credential 不进入 Luna CLI 自身配置。
- stdout 只返回目标路径、Context 名和 Credential ID，不输出 kubeconfig 或 Token；诊断写 stderr。

### Agent

- 不接收原始 Kubernetes 请求、kubeconfig、Credential 或任意流。
- Kube 协议路由、Credential 四个管理操作和集群网关 GET/PUT 全部进入 Agent 集中 deny map。
- Agent 需要诊断或执行平台操作时继续使用现有 OpenAPI 业务工具。

## 17. 接口与文件/模块落地清单

本节把第 3～16 节的设计映射到当前 monorepo。这里的文件名是实现边界，不表示拆成独立服务；同步
Kubernetes 请求仍全部由现有 API 进程处理，Worker 只处理异步协调、清理、观察和计费。

### 17.1 后端新增领域

#### 凭据、Scope 与数据访问

| 文件 | 职责 |
| --- | --- |
| `internal/credential/token.go` | 从现有 API Handler 抽出明文 Token 生成、哈希和常量时间校验，供普通 PAT、OAuth 与 Kube Credential 复用；只返回哈希或短生命周期明文，不记录参数 |
| `internal/authz/kube_scope.go` | 定义 `kube:read/write/connect`、包含关系和 Kube verb 到传输 Scope 的映射；不把这些 Scope 加入普通 PAT/OAuth 可选目录 |
| `internal/model/kube_access.go` | 定义 `KubeAccessBinding`、AccessToken source 常量、关联和唯一索引语义 |
| `internal/kubeaccess/types.go` | 管理 API 输入输出、Context、Credential 摘要和稳定领域错误 |
| `internal/kubeaccess/repository.go` | 按用户分页查询 Credential/Binding，事务创建、撤销和删除终态清理；所有方法接收 `context.Context` |
| `internal/kubeaccess/service.go` | 校验用户、项目成员、项目/集群/应用组合、Scope 与有效期，编排 Credential 和 Binding 的原子生命周期 |
| `internal/kubeaccess/kubeconfig.go` | 只从可信 `PUBLIC_BASE_URL` 和创建结果渲染 kubeconfig，生成稳定且无冲突的 cluster/user/context 名 |
| `internal/runtimeaccess/web_console.go` | 提取平台来源与 kubectl 来源对象共用的 Web Console 有效策略，供现有终端、Kube connect 和 `ephemeralcontainers` 写入调用 |
| `internal/runtimecluster/state.go` | 定义 RuntimeCluster 删除状态、`IsActive` 和统一 GORM `ActiveScope`；非管理/清理路径禁止直接猜测可用状态 |

`internal/kubeaccess.Service` 是四个 Credential Handler 和 Kube Authenticator 的共同入口。管理 Handler
不得直接拼 GORM 查询或自行生成 Token；Kube Authenticator 也不得复用会按 Gin Route Scope 判断的
现有平台中间件。

#### 资源目录与统一策略

| 文件 | 职责 |
| --- | --- |
| `internal/kubecatalog/catalog.go` | 唯一定义允许的 GVR、subresource、verb、平台 Action、资源类别、`bindingScope`、集合隔离策略和固定拒绝项 |
| `internal/kubecatalog/rbac.go` | 从同一目录生成上游 ClusterRole 规则，保证平台 Authorizer 与上游 RBAC 不维护两份白名单 |
| `internal/kubepolicy/workload.go` | 校验 Pod、PodTemplate、Job、CronJob、ephemeral container、hostPort 和控制器工作负载安全字段 |
| `internal/kubepolicy/reference.go` | 定义 `PolicyContext`/`ReferenceResolver`，实时校验 Secret、ConfigMap、PVC、ServiceAccount 与受限投射引用 |
| `internal/kubepolicy/service.go` | 校验 Service 类型和 externalIPs；不承载 Pod/Container 字段判断 |
| `internal/kubepolicy/gateway.go` | 校验 Ingress/HTTPRoute/GRPCRoute 的域名、class/parent、backend、TLS Secret 和跨 namespace 边界 |

`kubecatalog` 的额外 GVR 规则只在构造 Catalog 时合并，必须先通过 Discovery 证明为 namespaced，并继续
受固定拒绝项约束。`kubepolicy` 返回 Kubernetes `field.ErrorList` 或等价结构，使平台 API 和 Kube
Gateway 可以分别生成稳定平台错误和原生 422 Status，而不复制判断。

#### Kubernetes 协议代理

新增 `internal/kubeproxy`，按下表拆分；禁止把所有逻辑堆入一个 Gin Handler：

| 文件 | 职责 |
| --- | --- |
| `types.go` | `AccessContext`、`RequestInfo`、`Decision`、`Upstream`、传输类型和稳定错误类型 |
| `handler.go` | 组合完整流水线，统一恢复 panic 并返回 Kubernetes Status |
| `request_info.go` | 基于原始 `EscapedPath` 解析非资源路径、GVR、namespace、name、subresource、verb 与 Upgrade |
| `authorization.go` | 组合 Kube Scope、项目角色、业务 Action、Binding、Catalog 与资源策略求值，并在访问上游前执行 `bindingScope` 的项目/应用适用性判断 |
| `ownership.go` | 固定 namespace、按 Catalog 集合策略合并 selector、执行单对象归属预读和范围外 NotFound 语义；校验 ReplicaSet/Pod/Job/PVC/Endpoints/EndpointSlice 等控制器派生对象的归属标签 |
| `mutation.go` | 请求体上限、保留标签注入/保护和统一 WorkloadPolicy；覆盖 Create、Update 与四类 Patch/Apply，并递归处理 PodTemplate、CronJob jobTemplate 和 StatefulSet volumeClaimTemplates 的归属 metadata |
| `metrics.go` | 处理 PodMetrics 特例：项目 Binding 直通；应用 Binding 强制 selector/identity/canonical JSON、16 MiB 整份归属验证、Table/PartialObjectMetadata 转换与 Kubernetes serializer 内容协商；HEAD 通过内部 GET 完成同等校验后丢弃实体；区分客户端 406、上游非 2xx 原样返回和不可验证 2xx 的稳定 503 |
| `upstream.go` | 构造目标 URL、清洗请求头、重建短期上游身份并禁止用户控制目标 |
| `proxy.go` | 代理普通 HTTP、Watch 和 Logs，保留 Content Negotiation、Status、Warning 与取消语义 |
| `discovery.go` | 代理 Discovery/OpenAPI，并重写 OpenAPI v3 `serverRelativeURL` 与安全 Location |
| `upgrade.go` | 使用官方 Upgrade 代理兼容 WebSocket GET 与 SPDY POST，捕获并可主动关闭两端连接 |
| `upgrade_transport.go` | 为绕过 RoundTripper 的 Upgrade 建立/注入唯一 `kubernetes.proxy` 子 Span、安装安全 klog Context 并清洗协议 Header；不创建 Server Span |
| `stream.go` | Watch、Logs、Exec、Attach、Port-forward 的并发、最长时长、空闲超时和持续复核 |
| `limiter.go` | 认证前 IP、认证后用户/项目请求频率及三类流并发的窄接口和进程内 lease |
| `local_review.go` | 本地合成 SelfSubjectAccessReview、SelfSubjectRulesReview 和 SelfSubjectReview |
| `local_resource.go` | 本地合成当前 Namespace 与项目可用 StorageClass 的 Get/List、Table、Protobuf 和 CBOR 响应 |
| `status.go` | 构造 400/401/403/404/405/406/409/413/422/429/503/504 `metav1.Status` |
| `audit.go` | 只接受白名单审计字段，记录请求与流 open/close 终态 |
| `telemetry.go` | 在 Kube 入口唯一建立脱敏 `kube.gateway.request` Server Span，并提供内部 Span、结构化日志和低基数 Metric |

核心依赖使用窄接口，便于在协议测试中替换数据库、Secret Store 和上游 API Server：

```go
type Authenticator interface {
    Authenticate(context.Context, credential, bindingID string) (AccessContext, error)
    Revalidate(context.Context, AccessContext) (AccessContext, error)
}

type Authorizer interface {
    Authorize(context.Context, AccessContext, RequestInfo) (Decision, error)
}

type UpstreamFactory interface {
    ForBinding(context.Context, AccessContext) (Upstream, error)
}

type AuditRecorder interface {
    Begin(context.Context, AuditEvent) (AuditAttempt, error)
    Finish(context.Context, AuditAttempt, AuditResult) error
    RecordDenial(context.Context, AuditEvent) error
}

type Limiter interface {
    AllowPreAuth(context.Context, ClientKey, RequestClass) error
    AllowRequest(context.Context, AccessContext, RequestInfo) error
    AcquireStream(context.Context, AccessContext, StreamClass) (release func(), err error)
}
```

`Revalidate` 用于长连接复核，必须重新查询 Token、用户、Binding、项目成员、应用归属和集群状态，不能
只检查创建连接时的 `AccessContext`。`AuditRecorder` 的输入类型不包含 body、header、query 或 command
字段，从类型层减少误记录风险；写入与连接必须在 `Begin` 成功后才能继续。

#### Kubernetes Provider、任务和 API 适配

| 文件 | 职责 |
| --- | --- |
| `internal/provider/kubernetes/kubectl_gateway_namespace.go` | 用稳定 system ownership 标签幂等 Ensure `luna-system`，拒绝接管非 Luna 同名 namespace；不提供 Delete |
| `internal/provider/kubernetes/kubectl_gateway_access.go` | 独立 `KubectlGatewayManager`，只消费完整 `GatewayAccessSpec`，先 Ensure system namespace，再幂等安装、更新、观察和回收 ServiceAccount、ClusterRole、ClusterRoleBinding，以及当前有权使用集群的活动项目 RoleBinding |
| `internal/provider/kubernetes/kubectl_gateway_token.go` | 使用 TokenRequest 获取短期 ServiceAccount Token；从匿名 `rest.Config` 重建代理身份并重新插桩，确保管理 kubeconfig 凭据不残留 |
| `internal/provider/kubernetes/kubectl_gateway_cleanup.go` | 按中央 Catalog 与不可变标签清理 kubectl 来源资源；应用清理不触碰项目共享 RoleBinding |
| `internal/provider/kubernetes/kubectl_runtime_observation.go` | 枚举并归一化 kubectl 来源工作负载，提供观察和计费输入，不缓存实时状态 |
| `internal/api/kube_credential_handlers.go` | 适配第 5.2 节四个 JSON 接口，负责绑定、分页和响应，不承载业务规则 |
| `internal/api/runtime_cluster_kube_gateway_handlers.go` | 适配第 5.3 节 GET/PUT，实时观察状态并投递协调任务 |
| `internal/api/kube_proxy_handler.go` | 把两个特殊协议路由接入 `internal/kubeproxy.Handler`，并向 `router.go/static_ui.go` 提供 Kube NoMethod/NoRoute Status responder；自身不重复注册全局 fallback |
| `internal/api/kube_proxy_limiter.go` | 把现有 Redis-backed `rateLimiter` 适配为 Kube `Limiter`，并从可信代理配置解析认证前客户端 key |
| `internal/tasks/kubectl_gateway.go` | 定义只含 `clusterId` 的幂等协调任务与 Trace Context 传播；明确禁用 `asynq.Unique` |
| `internal/worker/kubectl_gateway_runner.go` | 以 PostgreSQL session advisory lock 按集群串行，回读完整最新期望并调用 `KubectlGatewayManager`，处理 active 集群的启用/禁用；遇到任何非 active 状态立即退出，删除只由 `resource:cleanup` 负责 |
| `internal/worker/kubectl_gateway_scheduler.go` | 每 5 分钟为活动集群补投协调任务，修复瞬时漏投；不扫描删除中集群，不保存实时状态 |
| `internal/worker/kubectl_runtime_billing.go` | 把没有 DeploymentTarget 的 kubectl 工作负载纳入 RuntimeObservation 与现有运行资源计费 |

不要扩张现有 `NamespaceManager` 或复用 `runtime_logs_exec.go` 实现原始 kubectl：前者已有大量 Worker
调用和 fake，后者会缓冲日志、只支持固定参数，并把 Exec 包装为 shell 命令，均不符合本方案协议边界。

### 17.2 现有后端文件修改

#### 路由、认证与契约

| 文件 | 修改内容 |
| --- | --- |
| `internal/api/router.go` | 注册六个管理接口和两个显式方法集合的协议路由；在通用 CORS `OPTIONS` 短路前处理 `/kube/`，开启并接线 Kube NoMethod |
| `internal/api/static_ui.go` | 接收 Kube responder 并只注册一个组合 NoRoute：先委派 `/kube/`，非 Kube 请求才继续静态文件与 SPA fallback；`staticFS=nil` 时仍保留 Kube 分支 |
| `internal/api/handlers.go` | 注入 `kubeaccess.Service`、`kubeproxy.Handler`、`KubectlGatewayManager` 和 `EnqueueKubectlGateway` 窄接口 |
| `internal/api/meta_handlers.go` | 增加 `features.kubectlGateway` |
| `internal/api/response.go`、`internal/api/access_token_handlers.go` | 改用 `internal/credential`；普通 PAT 继续固定 `source=personal`，且不展示 `kube:*` |
| `internal/authz/action.go` | 仅在确有缺失时补业务 Action；优先复用现有 `deployment/secret/volume/gateway/cluster` Action |
| `internal/authz/scope.go` | 保持普通 AccessToken/OAuth Scope Catalog 不包含 Kube 专用 Scope |
| `openapi/openapi.yaml` | 增加六个管理接口、Schema、分页、Scope、敏感字段、`operationId` 和 Agent 排除；同步 RuntimeCluster 删除状态投影、DELETE 204/409/503 契约，并删除 namespace 与不再允许的高风险部署字段 |
| `internal/aitool/openapi_catalog.go` | 在集中 deny map 登记六个操作的稳定原因；协议通配路由本身不进入 OpenAPI |
| `go.mod`、`go.sum` | 增加与现有 Kubernetes 依赖同 minor 的 `k8s.io/metrics v0.36.2`，只用于 PodMetrics 类型注册、Protobuf/CBOR 编码和本地表示转换 |

Kube Credential 明文响应和 kubeconfig 必须使用专用 response writer 设置 `Cache-Control: no-store`，且不
经过会记录响应体的通用调试路径。`GET /meta` 只反映服务版本是否包含该能力，不替代集群实时状态。

#### 模型、迁移、namespace 与审计

| 文件 | 修改内容 |
| --- | --- |
| `migrations/000095_kubectl_gateway.up.sql`、`migrations/000095_kubectl_gateway.down.sql` | 实现第 4.1 节全部 Schema、RuntimeCluster 删除状态/阶段字段、索引、外键和回滚；删除/恢复 `deployment_targets.namespace` |
| `internal/model/deployment.go` | RuntimeCluster 增加网关期望配置、现有风格的删除状态字段和上游清理阶段时间；删除 DeploymentTarget namespace override |
| `internal/model/audit.go` | 增加受控 metadata JSONB |
| `internal/model/runtime_observation.go` | 支持 management source、资源 Kind/UID、可空 Application/DeploymentTarget 身份 |
| `internal/model/models.go` | 注册新增模型；不使用 GORM AutoMigrate 代替 SQL migration |
| `internal/api/admission.go` | 认证只负责身份和基础 Token 有效性；Kube 路由不再按 Gin `FullPath` 落入 `system:unmapped` |
| `internal/api/runtime_cluster_access.go` | 复用项目、集群、成员和权威 namespace 解析，对“可使用/可管理”回读应用 `runtimecluster.ActiveScope`，并去除 DeploymentTarget namespace 回退 |
| `internal/api/runtime_cluster_handlers.go`、`internal/api/runtime_cluster_input.go` | 暴露开关摘要和删除状态投影；网关开关、规则或项目 Scope 变化时投递协调任务；DELETE 实现第 5.3 节事务、排空时间和 `resource:cleanup`，不在列表中批量实时探测 |
| `internal/api/runtime_cluster_resource_handlers.go`、`internal/api/runtime_cluster_pressure_handlers.go`、`internal/api/release_runtime_handlers.go`、`internal/api/system_component_handlers.go`、`internal/api/project_volume_cluster.go` | 回读运行集群时统一使用 `runtimecluster.ActiveScope`，非 active 不产生观察、终端、部署、系统组件或卷副作用 |
| `internal/api/deployment_target_metrics_handlers.go`、`internal/api/deployment_target_observation.go`、`internal/api/runtime_cluster_observation.go` | 指标与实时观察的 RuntimeCluster 回读必须使用 `ActiveScope`，管理列表遇到非 active 仅投影删除状态，不再并发请求上游 |
| `internal/api/app_template_handlers.go`、`internal/api/deployment_bundle_candidates.go` | 默认集群解析、模板验证和 Bundle 候选集仅包含 `ActiveScope` 集群，避免新配置继续绑定 delete_failed 资源 |
| `internal/aitool/service.go` | Agent 的 RuntimeCluster 列表与项目关联查询也使用 `ActiveScope`；对原始 `Table("runtime_clusters")` 查询显式套用同一 Where Scope，不依赖 GORM 模型默认 |
| `internal/api/project_handlers.go` | 项目创建、namespace 变化和删除终态触发相关已启用集群的 RoleBinding 协调与回收 |
| `internal/api/deployment_target_input.go`、`internal/api/deployment_target_types.go`、`internal/api/deployment_target_observation.go` | 删除 namespace 输入/输出 override；实际观测到的 namespace 字段继续保留 |
| `internal/api/release_runtime_handlers.go`、`internal/api/runtime_terminal_authorization.go`、`internal/api/runtime_web_console_policy_test.go` | 改用 `internal/runtimeaccess`，保持现有终端与 kubectl 长连接使用同一 Web Console 开关语义；`runtimeClusterForDeploymentTargetDB` 和长连接复核同时强制 `ActiveScope` |
| `internal/api/deployment_bundle_*.go` | 删除 Bundle 导入、导出、预览和校验中的目标 namespace override |
| `internal/api/app_template_handlers.go`、`internal/model/app_template.go` | 删除模板中的目标 namespace override |
| `internal/volume/service.go` | 卷操作只使用项目权威 namespace，删除 DeploymentTarget namespace 回退 |

项目、应用、集群软删除不会触发数据库级级联，因此它们的业务删除终态必须显式撤销对应 Binding 并
取消相关流；只有项目/集群边界变化才回收共享 RoleBinding。硬删除外键仅是最终兜底。

#### Kubernetes 资源、清理、统一策略与计费

| 文件 | 修改内容 |
| --- | --- |
| `internal/provider/kubernetes/labels.go` | 增加 `luna.devops/management-source` 及 `platform/kubectl` 常量 |
| `internal/provider/kubernetes/ownership.go` | 把 management source、项目和应用标签纳入保护标签、归属判断与删除选择器 |
| `internal/provider/kubernetes/namespaces.go`、`internal/provider/kubernetes/namespaces_test.go` | 让 `ProjectNamespaceLabels` 和既有 namespace 协调加入固定 PSA enforce/warn/audit 标签与版本 |
| `internal/provider/kubernetes/runtime_resource_list.go` | 纳入 Job、CronJob、ReplicaSet、Ingress、PDB、NetworkPolicy、GRPCRoute 等 Catalog 资源 |
| `internal/provider/kubernetes/resources.go` | 同步资源详情、YAML、Event 和删除目录 |
| `internal/provider/kubernetes/application_topology.go` | 纳入带应用标签的 kubectl 来源对象 |
| `internal/api/runtime_cluster_resource_contract.go` | 同步受支持 Kind 与前端可见契约 |
| `internal/provider/kubernetes/application_workload.go`、`internal/provider/kubernetes/application_service.go`、`internal/provider/kubernetes/deploy_resources.go` | 平台部署也调用 `kubepolicy`，移除第 10 节列出的绕过路径 |
| `internal/api/deployment_target_input_kubernetes.go` | 在 API 输入阶段调用同一策略，并删除不再允许的 capability、Service 类型和提权选项 |
| `internal/worker/kube_specs.go` | Worker 在最终渲染后再次调用同一策略，禁止只信任入库时校验 |
| `internal/worker/kube_specs.go`、`internal/worker/build_capacity.go`、`internal/worker/gateway_runner.go`、`internal/worker/runtime_billing_runner.go`、`internal/dashboard/service.go` | 部署、构建、网关、计费和看板的 RuntimeCluster 查询统一复用 `ActiveScope`；管理列表例外单独写明 |
| `internal/api/system_component_handlers.go`、`internal/worker/deploy_runner.go` | 仅由可信 SystemComponent 计划授予专用 ServiceAccount/RBAC，删除任意 DeploymentTarget 名称触发授权的通用路径 |
| `internal/api/gateway_route_domain.go`、`internal/api/gateway_route_validation.go` | 提取并复用 `kubepolicy/gateway.go` 的域名、parent 和 backend 约束，防止平台表单与 kubectl 路由双标准 |
| `internal/worker/application_delete_runner.go` | 清理应用 Binding、流和 `management-source=kubectl` 的应用标签资源，不删除项目共享 RoleBinding |
| `internal/worker/resource_cleanup.go` | 作为唯一删除 owner 扩展 `runtime_cluster` 分支、失败标记和超时恢复扫描；以 `Unscoped` 回读、drain deadline、上游清理阶段持久化和 Secret 最后删除实现第 5.3 节状态机 |
| `internal/worker/runtime_billing_runner.go` | 合并 kubectl RuntimeObservation，继续写入现有运行资源计费链路 |

平台发布新建的对象统一补 `management-source=platform`；现有无标签对象在同一事项内由部署协调时补齐，
不写一次性全集群扫描器。删除与孤儿清理必须兼容补齐前的既有 `managed-by/project-id` 对象，但不得因此
放宽 kubectl 新对象的标签要求。

#### 任务与遥测接线

| 文件 | 修改内容 |
| --- | --- |
| `internal/tasks/client.go` | 注册任务类型、`KubectlGatewayPayload`、投递方法和有限重试策略；该任务不得使用 Unique 去重 |
| `internal/api/handlers.go` | `taskEnqueuer` 增加 `EnqueueKubectlGateway`，测试 fake 同步更新 |
| `internal/worker/worker.go` | 注册协调 Runner 和定时补投，并保持父 Trace Context |
| `internal/telemetry/gin.go` | 从通用 `otelgin` 排除 `/kube/v1/bindings/`，由 Kube Proxy 建立脱敏 Server Span |
| `internal/telemetry/gin_test.go` | 断言排除规则和普通平台路由插桩均保持正确 |
| `internal/provider/kubernetes/telemetry_transport.go` | 复用现有脱敏 Transport覆盖普通、Watch 和 Logs；Upgrade 改由 `kubeproxy/upgrade_transport.go` 手工插桩 |

不新增全局动态配置中心。第 14.3 节的大小、时长和并发限制先作为集中、可测试的固定安全默认值；公开
地址复用已有 `PUBLIC_BASE_URL`。只有实际部署需要声明反向代理超时时，才同步现有部署模板，不引入新的
微服务、监听端口或 Redis 状态缓存。

### 17.3 Web 文件

Web 不增加顶级 Route，也不实现第二套 Kubernetes 控制台，只提供三个入口：项目工作区生成访问、账号
设置管理 Credential、集群列表配置网关。

新增文件：

```text
web/src/api/domains/kubectl.ts
web/src/api/domains/kubectl.test.ts
web/src/lib/kubeconfig-file.ts
web/src/lib/kubeconfig-file.test.ts

web/src/pages/projects/kubectl/project-kubectl-access-panel.tsx
web/src/pages/projects/kubectl/project-kubectl-access-panel.test.tsx
web/src/pages/projects/kubectl/kubeconfig-create-dialog.tsx
web/src/pages/projects/kubectl/kubeconfig-create-dialog.test.tsx

web/src/pages/settings/account/kube-credentials-panel.tsx
web/src/pages/settings/account/kube-credentials-panel.test.tsx

web/src/pages/clusters/management/cluster-kube-gateway-dialog.tsx
web/src/pages/clusters/management/cluster-kube-gateway-rules-editor.tsx
web/src/pages/clusters/management/cluster-kube-gateway-dialog.test.tsx

web/src/i18n/locales/{zh-CN,zh-TW,en-US,ja-JP,ko-KR}/kubectlAccess.ts
```

修改文件：

| 文件 | 修改内容 |
| --- | --- |
| `web/src/App.tsx` | 注册 `kubectlAccess` 五语言懒加载资源，不新增页面 Route |
| `web/src/api/client.ts`、`web/src/api/index.ts` | 合并 `kubectlApi` |
| `web/src/api/types.ts` | 按现有集中类型约定增加 Credential、Binding、Gateway Config/Rule/Observation 和 RuntimeCluster 删除状态投影；不另建重复的 kubectl types 文件 |
| `web/src/pages/projects/overview/ProjectWorkspacePage.tsx` | 增加 `tab=kubectl`，复用现有项目、集群和应用查询 |
| `web/src/pages/settings/account/AccountPage.tsx` | 增加“kubectl 凭据”Tab；普通 Access Token 面板保持隔离 |
| `web/src/pages/clusters/ClustersPage.tsx`、`web/src/pages/clusters/management/runtime-cluster-table.tsx` | 增加管理员行操作和网关配置 Dialog；展示 deleting/delete_failed 语义状态，只在 delete_failed 提供重试删除，非 active 不进入任何集群候选器，且不在列表逐集群拉实时状态 |
| `web/src/i18n/locales/{zh-CN,zh-TW,en-US,ja-JP,ko-KR}/errors.ts` | 映射 Credential、Binding、网关未启用、协调中、投递失败、集群删除中/删除失败/不可用和撤销失败的稳定错误码 |

`kubeconfig-file.ts` 只负责 Blob 下载、文件名清洗和 URL 回收。明文只存在于创建 Dialog 的局部 state，
关闭即清空；不得进入 TanStack Query/Mutation Cache、localStorage、埋点、toast 详情或错误日志。创建
动作使用直接 API 调用并在下载/关闭后主动清空响应引用。

namespace 和工作负载策略收口还要同步：

```text
web/src/pages/applications/deployments/application-deployments-panel-utils.ts
web/src/pages/applications/deployments/editor/runtime/application-deployment-kubernetes-advanced-fields.tsx
web/src/pages/applications/deployments/application-deployment-bundle-import-dialog.tsx
web/src/pages/applications/deployments/application-deployment-bundle-import-dialog.test.tsx
web/src/pages/applications/deployments/operations/deployment-target-detail-sheet.tsx
web/src/pages/applications/deployments/operations/application-deployment-targets-list.test.tsx
web/src/pages/app-templates/AppTemplatesPage.tsx
web/src/pages/app-templates/AppTemplatesPage.test.tsx
web/src/i18n/locales/{zh-CN,zh-TW,en-US,ja-JP,ko-KR}/deploymentsPage.ts
```

这里只删除 DeploymentTarget、Bundle 和 AppTemplate 的可写 namespace override 以及不再允许的高风险工作
负载选项；拓扑、资源列表、内部地址和观察响应中的实际 namespace 是只读事实，必须保留。

### 17.4 Luna CLI 独立仓库

Luna CLI 的事实源是独立仓库 `github.com/LiteyukiStudio/luna-cli`；开发时使用对应仓库，不把当前
monorepo 中可能存在的忽略副本当成事实源。本轮新增：

```text
src/commands/kubeconfig.ts
src/kubeconfig/document.ts
src/kubeconfig/store.ts
tests/commands/kubeconfig.test.ts
tests/kubeconfig/document.test.ts
tests/kubeconfig/store.test.ts
```

并修改：

```text
src/commands/registry.ts
src/commands/index.ts
src/i18n/resources.ts
tests/output/security.test.ts
scripts/cli/verify-platform-cli-coverage.mjs
openapi/openapi.yaml
packages/api-contract/src/generated/schema.ts
packages/api-contract/src/generated/operations.ts
README.md
README_EN.md
docs/cli-spec.md
```

`luna kubeconfig write` 创建 Credential 并以 `0600` 原子写入新文件；`luna kubeconfig merge` 解析已有
kubeconfig，检测 cluster/user/context 同名但内容不同的冲突，只有显式 `replaceConflicts=true` 才覆盖。两者在命令
注册中标记为人类专用，并以 `consumedOperations` 认领 `createKubeCredential`；CLI 覆盖率脚本把该
operation 标记为精确 `protocol-adapter`，禁止生成会把敏感响应写到 stdout 的通用命令。

两个命令的 metadata 明确要求普通平台 Scope `token:manage`。当前 OAuth 会话缺少该 Scope 时，复用 CLI
现有 OAuth Scope remediation 提示用户重新授权并重试；不要为了 kubeconfig 命令把 `token:manage`
无条件加入所有 CLI 默认 Scope。Credential 列出/撤销命令执行时遵循同一修复流程，并覆盖缺少 Scope、
重新授权成功和用户取消三条测试。

命令接口统一为：

```text
luna kubeconfig write \
  credentialName=<name> \
  context=<projectId>:<clusterId>[:<applicationId>] [context=...] \
  scope=read|write|connect [scope=...] \
  expiresInDays=7 \
  destination=<path>

luna kubeconfig merge \
  credentialName=<name> \
  context=<projectId>:<clusterId>[:<applicationId>] [context=...] \
  scope=read|write|connect [scope=...] \
  expiresInDays=7 \
  [destination=<path>] [replaceConflicts=true]
```

`context` 使用不可歧义的资源 ID 而非 display name，可重复以生成多 Context kubeconfig；CLI 把
`read|write|connect` 映射到 `kube:*`，服务端仍做最终规范化与授权。`merge` 未显式指定 `destination` 时，
`KUBECONFIG` 未设置则使用 `~/.kube/config`，只包含一个路径则使用该路径；包含多个路径时拒绝并要求
`destination` 明确目标，不尝试原子修改多个文件。

CLI 必须在调用创建接口前完成目标路径、文件权限、YAML 可解析性和可预知的 Context 冲突检查，避免先
签发后发现本地无法保存；如果 API 成功后原子写入仍失败，则立即尽力调用撤销接口。撤销也失败时，错误
只输出 Credential ID 和人工撤销提示，不输出明文内容。

Credential 列出与撤销仍可由生成命令调用。Kube Credential 不写进 Luna CLI 配置，因此不修改
`src/config/schema.ts`；命令成功输出仅包含文件路径、Context 名和 Credential ID。

### 17.5 测试、文档与生成边界

后端新增测试至少包括：

```text
internal/database/kubectl_gateway_migration_test.go
internal/api/kube_credential_integration_test.go
internal/api/kube_gateway_openapi_contract_test.go
internal/api/kube_protocol_route_contract_test.go
internal/api/kube_gateway_audit_test.go

internal/authz/kube_scope_test.go
internal/kubeaccess/{repository,service,kubeconfig}_test.go
internal/runtimeaccess/web_console_test.go
internal/runtimecluster/state_test.go
internal/kubecatalog/{catalog,rbac}_test.go
internal/kubepolicy/{workload,reference,service,gateway}_test.go
internal/kubeproxy/{request_info,authorization,ownership,mutation,metrics,proxy,discovery,upgrade,upgrade_transport,stream,limiter,local_review,local_resource,status,audit,telemetry}_test.go
internal/provider/kubernetes/{kubectl_gateway_namespace,kubectl_gateway_access,kubectl_gateway_token,kubectl_gateway_cleanup,kubectl_runtime_observation}_test.go
internal/worker/{kubectl_gateway_runner,kubectl_gateway_scheduler,kubectl_runtime_billing}_test.go
```

同时扩展现有 `internal/provider/kubernetes/namespaces_test.go`、`telemetry_transport_test.go`、项目/应用/
集群删除测试、`internal/api/static_ui_test.go`、OpenAPI 路由契约测试和任务 Trace 测试，覆盖
PSA、管理凭据剥离、相同配置重投、执行中二次变更、应用删除不删共享 RoleBinding、
`/kube/` 先于 OPTIONS/SPA fallback 返回原生 Status，以及集群删除的 `deleting/delete_failed`、
超时重投、Binding 撤销后 35 秒排空、gateway runner 非 active 退出、上游不可达保留 Secret、
上游已清理后无 Secret 重试和最终软删除顺序。还要使用一组 deleting/delete_failed 集群回归 API、
Worker 和 Dashboard 消费者，确认只有管理投影和清理可回读，其他入口全部按非 active 拒绝。
该回归必须显式包含 DeploymentTarget 指标/观察、AppTemplate 默认集群、Bundle 候选和 AITool
`Table("runtime_clusters")` 原始查询，防止只改 GORM model 路径而漏掉手写 Table 消费者。

`kubepolicy` 契约额外覆盖 Create 原始空 ServiceAccount 在 DryRun 后变为隐式 `default`、
Update/Patch/Apply 保留该隐式值、用户主动指定未授权 ServiceAccount 被拒绝和 automount 始终为
`false`，并单独验证隐式 default SA 可以无 Luna 标签、其 Admission 注入的无标签/越界
`imagePullSecrets` 仍被拒绝。Debug 契约覆盖 `ephemeralcontainers` 在 Web Console 开关关闭时无上游副作用，
`--copy-to` 仍走普通 Pod Create。Upgrade Trace 测试要明确断言每个请求只有一个
`kube.gateway.request` Server Span，`kubernetes.proxy` 为其子 Span。
所有权契约还要用真实 controller 验证 Deployment→ReplicaSet/Pod、CronJob→Job/Pod、
StatefulSet→Pod/PVC 和 Service→Endpoints/EndpointSlice 的标签传播；应用 Binding 的 List/Watch/清理必须实际命中
这些派生对象，失配时 fail closed，不用 fake client 的理想化对象代替。

资源作用域契约必须分别使用项目 Binding 和应用 Binding：项目 Binding 可读取 Event、ServiceAccount、
ResourceQuota 和 LimitRange，应用 Binding 对四类资源的 Get/List/Watch 均在访问上游前返回 403；带
application-id 的 ResourceQuota 也不得绕过。PodMetrics 覆盖项目 Binding 直通、应用 Binding selector
合并、合法响应成功、provider 忽略 selector、条目缺失/伪造标签、`Accept-Encoding: identity`、未压缩
响应超限和上游 2xx 未知 Content-Type/解码失败整份 503，且断言失败响应不包含任何上游 PodMetrics
条目。还要覆盖 JSON/YAML/Protobuf/CBOR/Table/PartialObjectMetadata 的本地转换、只接受不支持表示时
返回 406、重编码实体 Header 重新生成/ETag 删除，以及上游非 2xx Status、Warning、Retry-After、Audit-ID
完全透传；应用 Binding PodMetrics HEAD 必须通过内部 GET 执行同样的成功/失败归属校验、返回与 GET
一致的状态和实体 Header 但零响应体，单对象 Get/HEAD 对外应用 Pod 固定返回 NotFound。

此外增加 `internal/kubeproxy/proxy_e2e_test.go`，用可销毁 API Server 和真实 kubectl 验证纯协议；再增加
`internal/e2e/kubectl_gateway_test.go` 编排 API、PostgreSQL、Redis、Worker、临时 OTel 栈和真实 API
Server，覆盖第 19 节完整调用链。fake client 只用于 RBAC 生成、幂等协调和失败注入，不能替代两类 E2E。

本轮新增或更新以下公开用户文档并同步导航：

```text
docs/docs/zh/use/kubectl.md
docs/docs/en/use/kubectl.md
docs/docs/zh/use/_meta.json
docs/docs/en/use/_meta.json
docs/docs/zh/reference/compatibility.md
docs/docs/en/reference/compatibility.md
docs/docs/zh/start/install/kubernetes.md
docs/docs/en/start/install/kubernetes.md
charts/luna-devops/README.md
```

操作页说明获取/合并 kubeconfig、Context、权限、覆盖语义和故障处理；兼容页只维护验证过的 kubectl 与
Kubernetes minor 范围。安装文档复用现有 `ingress.annotations` 说明所选 Ingress Controller 的 WebSocket、
请求体和至少 2 小时流超时配置，不向 Chart 注入控制器专属默认 annotation；真实 E2E 必须穿过部署时
实际使用的反向代理。实现完成时同步本内部文档、`TODO.md`、OpenAPI 和中英文 CLI 文档。

`openapi/spec.go` 与 `migrations/embed.go` 由现有生成/嵌入机制消费，不手工编辑。除增加与当前
`k8s.io/api`、`k8s.io/apimachinery`、`k8s.io/client-go` 同为 v0.36.2 的 `k8s.io/metrics` 外，不引入
`k8s.io/apiserver`、`k8s.io/kubectl` 或新的代理框架。`luna-agent` 不新增文件，只更新 API 侧的集中
deny map。

## 18. 同一交付事项的落地清单

下面是依赖顺序，不是分批发布。所有清单和验收同时完成后才能启用功能：

1. 收口项目 namespace，删除 DeploymentTarget namespace override 的全链路残留。
2. 增加 `kube:*` Scope、AccessToken 来源、KubeAccessBinding、Audit 元数据和 RuntimeCluster 网关配置。
3. 完成上游 ServiceAccount、ClusterRole、RoleBinding、TokenRequest 的安装、刷新、回收和实时观察。
4. 完成管理 OpenAPI、模型、Repository、Service、Handler、前端类型和 API Client。
5. 实现中央 GVR 目录、RequestInfo、业务 Action 映射和 Kube Authorizer。
6. 实现 selector 合并、归属标签、四类 Patch/Apply 标签保护和共享 WorkloadPolicy。
7. 实现普通 HTTP、Discovery、OpenAPI path-prefix 重写、Content Negotiation 和 Kubernetes Status。
8. 实现 CRUD、DeleteCollection、DryRun、Diff、Apply、Scale、Rollout、Debug 和删除语义。
9. 实现 Watch、Logs、WebSocket、SPDY、Exec、Attach、Port-forward 和 cp。
10. 实现 SelfSubjectAccessReview、RulesReview 和 WhoAmI 本地求值。
11. 完成 Web 页面、五语言、Luna CLI、Credential 生命周期和集群配置。
12. 同步项目/应用删除、孤儿清理、资源观察、计费、审计和全链路 OTel。
13. 同步中英文公开操作文档、内部索引、TODO、OpenAPI 和 Agent deny map。
14. 完成以下全部自动化与真实环境验收后，才允许把集群级开关对用户启用。

## 19. 完整验收矩阵

| 类别 | 必须验证 |
| --- | --- |
| Credential | 生成、一次性下载、多 Context、撤销、过期、用户禁用、成员移除、集群解绑 |
| Discovery | `version`、`api-versions`、`api-resources`、`explain`、OpenAPI v2/v3 |
| Read | `get/list -o yaml/json/wide`、`describe`、`wait`、`top pod`；应用 Binding 的 Event 限制和 PodMetrics fail-closed 语义明确可见 |
| Watch | 正常事件、bookmark、重连、410、撤权后断开、客户端取消无泄漏 |
| Logs | 普通、previous、多容器、tail/since/timestamps、`-f`、撤权后断开 |
| Write | create、client/server apply、diff、edit、replace、四类 Patch、dry-run、delete、deletecollection |
| Workload | run、expose、scale、autoscale、set image/env/resources、rollout status/history/restart/undo |
| Debug/Connect | 非 TTY/TTY exec、attach、非 Node debug、port-forward、cp 上传/下载、WebSocket/SPDY；ephemeral debug 受 Web Console 开关约束且 `--copy-to` 仍按 Pod Create |
| Auth | `can-i` 允许/拒绝、`can-i --list`、`whoami`、拒绝查询他人 |
| Ownership | 跨项目 namespace、应用 Binding、伪造/删除标签、对象名单独访问、selector AND 合并、项目共享资源拒绝，以及 Deployment/CronJob/StatefulSet/Service 派生对象的标签传播 |
| Protected resources | Secret Scope、RBAC、Node、PV、CRD/Webhook、CSR、SA token、proxy 全部符合矩阵 |
| Pod security | PSA 标签、privileged、host namespace、hostPath、hostPort、capability、automount token、越权 SA、隐式 default SA 的 DryRun 出处、Projected Token、CSI Secret provider |
| References | Secret/ConfigMap/PVC/imagePullSecret/ServiceAccount 的同项目、同应用与跨应用引用矩阵 |
| Protocol | JSON/YAML/Protobuf/Table/PartialObjectMetadata/CBOR、Warning、Status、gzip、HTTP/2、HEAD、未知 path=404、method=405、Accept=406、Upgrade=400、Kube 路由先于 OPTIONS/SPA fallback、流式不缓冲 |
| Upstream safety | 用户 Authorization、Cookie、Impersonate、Forwarded 不到上游，响应不泄露上游地址 |
| Limits | 413、429、普通超时、流并发上限、Token 到期主动断开 |
| Lifecycle | 项目/应用删除清理两类资源，应用删除不删共享 RoleBinding；集群删除可重试、Binding 撤销后排空长连接、非 active 消费者全部拒绝、上游失败保留 Secret、清理阶段持久化后最后删 Secret；kubectl 资源纳入实时观察和计费 |
| Audit | 成功、拒绝、失败、流 open/close 可关联，且无正文、Secret、命令和流数据 |
| OTel | Web/API/DB/Secret/TokenRequest/Kubernetes 父子 Trace、Upgrade 单一 Server Span、失败状态和低基数 Metric |
| Reconcile | `luna-system` 创建/幂等/非 Luna 冲突与不随禁用删除；相同 PUT 重投、enqueue 失败、任务执行中二次变更、跨 Worker 串行、周期补投与 Spec hash Ready 判断 |
| CLI | `0600` 原子写、冲突预检、失败撤销、缺少 `token:manage` 的 OAuth 修复、stdout 无凭据 |
| Real E2E | 生成 kubeconfig → 真实 kubectl → kube-apiserver 副作用 → 权威回读 |
| Regression | Go 全量测试、OpenAPI 契约、Web test/lint/build/singletons、Docs build 和浏览器验收 |

真实 E2E 使用可销毁 PostgreSQL、Redis、Kubernetes 集群和临时外部 OTel 栈，至少覆盖成功链、
授权拒绝链、上游失败链、写入副作用链和长连接撤权链。临时集群、凭据、日志和遥测数据不得写入仓库。

兼容版本按仓库使用的 `client-go` minor 与相邻两个 Kubernetes minor 验证；网关原样返回实际上游
`/version`，不得伪装固定 Kubernetes 版本。

## 20. 完成门禁

仅当以下条件同时满足时，本事项才算完成：

- 第 1 节列出的全部命令与协议能力已经实现，没有“暂不支持”或隐藏的功能子集。
- 前端、API、Worker、Kubernetes Provider、Luna CLI、OpenAPI、Agent 排除和文档调用链一致。
- 所有资源动作都经过 Token Scope、项目角色、Binding、namespace、GVR 和对象归属的最终判断。
- 成功、失败、取消、冲突、撤权和上游不可达均有 Kubernetes 原生终态与权威回读。
- 完整验收矩阵通过，TODO 对应事项才能标记完成并允许启用 RuntimeCluster 网关开关。

## 21. 明确不做

- 不让 kubectl 直连 kube-apiserver，不下发任何真实集群 kubeconfig 或上游身份。
- 不建设 Kubernetes Aggregation Layer 或第二套 kube-apiserver。
- 不同步每个 Luna 用户到 Kubernetes User、ServiceAccount、RoleBinding 或 Impersonation。
- 不通过项目 Binding 开放 Node、集群 RBAC、CRD 定义、Webhook、CSR 等集群管理员能力。
- 不把平台资源的临时 kubectl 修改反向猜测为平台表单、Release 或数据库期望配置。
- 不把 Kubernetes 当前状态、resourceVersion、Watch Event 或流内容保存到数据库、Redis 或进程缓存。
- 不向 Agent 暴露通用原始 Kubernetes API、kubeconfig 或流式连接。

## 22. 参考与事实源

内部事实源：

- 产品边界：[`产品概要.md`](产品概要.md)
- 代码与完整验证要求：[`代码检查流程.md`](代码检查流程.md)
- 可观测与遥测安全：[`可观测和插桩规范.md`](可观测和插桩规范.md)
- Agent 特殊协议排除：[`11-AI助手与Agent规格.md`](11-AI助手与Agent规格.md)
- 计划与验收状态：[`../TODO.md`](../TODO.md)
- 项目级强制约束：[`../AGENTS.md`](../AGENTS.md)

Kubernetes 协议事实源：

- [Kubernetes API](https://kubernetes.io/docs/concepts/overview/kubernetes-api/)
- [Kubernetes API Concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/)
- [Server-Side Apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/)
- [Authorization](https://kubernetes.io/docs/reference/access-authn-authz/authorization/)
- [RBAC Good Practices](https://kubernetes.io/docs/concepts/security/rbac-good-practices/)
- [Impersonation](https://kubernetes.io/docs/reference/access-authn-authz/user-impersonation/)
- [WebSocket Streaming Transition](https://kubernetes.io/blog/2024/08/20/websockets-transition/)
- [Auditing](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
- [Version Skew Policy](https://kubernetes.io/releases/version-skew-policy/)
- [Kubernetes v1.36.2 Endpoints controller 的 Service 标签传播](https://github.com/kubernetes/kubernetes/blob/v1.36.2/pkg/controller/endpoint/endpoints_controller.go)
- [Kubernetes v1.36.2 EndpointSlice 的 Service 标签传播](https://github.com/kubernetes/kubernetes/blob/v1.36.2/staging/src/k8s.io/endpointslice/utils.go)
- [Metrics Server v0.8.0 PodMetrics List 的 label selector 执行](https://github.com/kubernetes-sigs/metrics-server/blob/v0.8.0/pkg/api/pod.go)
- [Metrics Server v0.8.0 PodMetrics 的 Pod 标签传播](https://github.com/kubernetes-sigs/metrics-server/blob/v0.8.0/pkg/storage/pod.go)
