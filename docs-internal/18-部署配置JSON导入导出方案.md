# 部署配置 JSON 导入导出方案

## 1. 目标与非目标

第一版提供单个 `DeploymentTarget` 的版本化 JSON 导出、目标应用导入预检和原子导入。配置包用于在应用之间复用期望配置，不是数据库备份，也不是应用模板安装器。

本方案必须保证：

- 导出文件只包含可移植的期望配置、非密钥配置值和依赖描述，不包含源数据库 ID、凭据引用、Secret 值或运行态。
- 导入到另一项目空间或空应用时，仓库、集群、镜像站、配置集、Hook 和项目数据卷必须在目标作用域重新解析或显式映射。
- 任一必需依赖缺失、歧义、无权限或不兼容时，预检状态不是 `ready`，确认导入不可用，也不会写入部分部署配置。
- 导入只创建部署配置及其绑定关系，不创建应用、代码仓库账号、镜像凭据、运行配置集、变量集、Hook、数据卷、构建、Release 或 Kubernetes 资源。
- 导入成功不自动触发构建或发布；新部署配置使用现有创建语义启用，后续构建和发布仍由用户显式触发或由未来事件触发。

第一版不支持覆盖或合并已有部署配置，不支持整应用批量导入，也不承诺把源项目资源克隆到目标项目。

## 2. 配置包契约

响应媒体类型为 `application/json`，文件扩展名为 `.json`。顶层结构固定为：

```json
{
  "schemaVersion": 1,
  "kind": "luna-devops.deployment-target",
  "exportedAt": "2026-08-16T12:00:00Z",
  "configuration": {},
  "references": [],
  "secretRequirements": [],
  "omissions": []
}
```

`configuration` 使用部署配置创建输入的非运行态子集。以下字段必须清空且不得从导入文件接受：

- `environmentId`、`buildEnvironmentId`；
- `clusterId`、`repositoryBindingId`、`targetRegistryId`；
- `buildVariableSetIds`、`runtimeConfigSetIds`、`runtimeConfigRefs`、`buildHookBindings` 中的资源 ID；
- `dataVolumes[].projectVolumeId`；
- `buildSecrets`、`secretRefs`、`secretFiles`；
- 任何项目、应用、部署配置、Release、BuildRun、Kubernetes 资源或审计标识。

可直接携带的配置包括名称、阶段、命名空间、工作负载与资源规格、探针、安全上下文、Service、自动扩缩容、镜像或构建定义、Dockerfile/上下文/构建参数、触发规则、普通环境变量、普通配置文件、临时 `emptyDir`、审批与 Web Console 策略。`enabled` 不从文件取值，导入端使用平台创建语义。

如果目标镜像仓库路径是平台按源项目/应用/阶段模板生成的默认值，导出时写为空，导入时由目标镜像站凭据模板重新生成；显式自定义仓库路径才保留。这样不会把源应用标识误带入目标应用。

### 2.1 依赖描述

`references` 每项包含稳定 `key`、`kind`、是否必需、源资源的非密钥描述和使用语义。支持：

| kind | 导出描述 | 目标约束 |
| --- | --- | --- |
| `repositoryBinding` | Provider 类型、owner、repo | 必须绑定到目标应用；不能复用其他应用的绑定 |
| `runtimeCluster` | 名称、类型 | 必须对目标项目可用；源配置为空集群时保留 `projectDefault` 策略，不导出源默认集群 |
| `artifactRegistry` | 名称、Provider、namespace | 必须对目标项目可用；源码构建还必须存在目标项目可用的 push/push-pull 凭据 |
| `buildVariableSet` | 名称、scope | 必须对当前用户和目标项目可用 |
| `runtimeConfigSet` | 名称、原引用模式 | 必须属于目标项目；`snapshot` 在导入时从目标配置集重新生成快照 |
| `hookConfig` | 名称、phase、顺序 | 必须属于目标项目；不内联脚本 |
| `projectVolume` | 展示名、卷模式、访问模式、源集群描述、挂载参数 | 必须属于目标项目、目标集群且与挂载模式兼容 |

候选匹配只用于建议，不授予权限。仅当目标作用域中存在唯一、可访问且属性精确匹配的候选时才自动解析；没有候选、多候选、权限不足或属性不兼容都要求显式处理。

### 2.2 密钥需求

`secretRequirements` 只描述需要重新填写的键名或文件挂载路径，不包含源 Secret 引用和值。支持部署级构建密钥、运行时环境变量密钥和运行时密钥文件。导入确认请求按 requirement key 提交新值，服务端通过 Secret Store 写入并只持久化引用。

目标变量集和运行配置集自带的 Secret 不复制；映射后使用目标资源当前的 Secret。

## 3. API

```text
GET  /api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/export
POST /api/v1/projects/{projectId}/applications/{applicationId}/deployment-target-imports/preview
POST /api/v1/projects/{projectId}/applications/{applicationId}/deployment-target-imports
```

导出要求 `deployment:read`。预检和确认导入要求 `deployment:update` 以及目标项目 Owner/Admin/Developer；涉及部署级构建变量或密钥时继续复用现有 Owner/Admin 与 `secret_update` Step-up 边界。三个接口都重新校验项目和应用归属。

预检请求包含 `bundle`、`mappings` 和可选 `overrides`。响应包含：

- `digest`：规范化配置包 SHA-256；
- `status`：`ready`、`requires_mapping` 或 `invalid`；
- `references`：每项状态、当前解析值和可选候选；
- `secretRequirements`；
- `warnings`；
- 最终名称、阶段和命名空间摘要。

确认导入必须回传相同 bundle、预检 digest、mappings、overrides 和密钥值。服务端重新解析、重新授权、重新解析全部依赖并比较 digest，防止预检后文件被替换；随后在一个数据库事务内创建 DeploymentTarget、数据卷挂载、Hook 绑定和部署级构建环境。任一环节失败时全部回滚。

导入请求不创建构建或发布任务。相同应用/阶段的唯一约束作为重复提交的最终保护；第一版不把冲突解释为覆盖。

## 4. 跨项目和空应用行为

### 4.1 源码来源

源码来源必须解析目标应用自己的 RepositoryBinding。目标应用没有仓库绑定时，预检返回 `requires_mapping` 和 `deployment_bundle.repository_binding_missing`，界面引导先绑定仓库再重新预检。即使目标项目存在同仓库但绑定属于另一应用，也不能导入。

源码来源还必须解析可用目标镜像站和推送凭据。缺少凭据时返回 `deployment_bundle.registry_push_credential_required`，不创建以后必然无法构建的配置。

### 4.2 镜像来源

镜像来源只要 `imageRef` 合法且其余依赖已解析，就能导入到没有仓库、构建变量或历史构建的空应用。它不会凭空创建 Release。

### 4.3 阶段与命名空间

源阶段在目标应用中已存在时返回 `deployment_bundle.stage_conflict`，用户必须选择新的可用阶段。第一版不覆盖已有配置。

显式命名空间作为期望配置保留，但预检返回 `deployment_bundle.namespace_review_required` 警告，提醒跨项目导入时核对。空命名空间继续使用目标项目命名空间。

### 4.4 数据卷

`emptyDir` 可直接导入。项目数据卷必须映射到目标项目的现有卷，且目标卷集群、Filesystem/Block 模式、访问模式和当前绑定能力通过现有 Volume Service 校验。平台不复制 PVC 或数据；需要迁移数据时先使用数据卷中心的导入/导出能力。

## 5. 输入、错误与安全边界

- 请求体上限 1 MiB；只接受 UTF-8 JSON。
- 解析器使用 `DisallowUnknownFields`，拒绝重复键、多个顶层值、超过 32 层嵌套和不支持的 kind/schemaVersion。
- 服务端不记录配置包正文、环境变量、配置文件内容或密钥值；Trace/Metric 只记录稳定结果、引用种类和数量。
- 导出响应使用 `Cache-Control: no-store`、`X-Content-Type-Options: nosniff` 和安全附件文件名。
- 生产错误只返回稳定 code、公共 error 和 requestId；开发模式可保留 detail。
- 导出、预检失败、导入成功/失败写入 AuditLog；审计只记录部署配置 ID、目标应用 ID、稳定错误码和依赖数量，不记录正文或资源密钥。

建议稳定错误码：

- `deployment_bundle.invalid_json`
- `deployment_bundle.unsupported_kind`
- `deployment_bundle.unsupported_version`
- `deployment_bundle.digest_mismatch`
- `deployment_bundle.reference_missing`
- `deployment_bundle.reference_ambiguous`
- `deployment_bundle.reference_forbidden`
- `deployment_bundle.reference_incompatible`
- `deployment_bundle.repository_binding_missing`
- `deployment_bundle.registry_push_credential_required`
- `deployment_bundle.secret_required`
- `deployment_bundle.stage_conflict`
- `deployment_bundle.not_ready`

## 6. 前端流程

部署配置行操作增加“导出 JSON”。部署页标题工具增加“导入 JSON”。导入 Dialog 延迟加载并分为三个阶段：

1. 选择 `.json` 文件并在浏览器侧检查大小、kind 和 version；
2. 调用预检，展示目标名称/阶段、已自动匹配项、缺失/歧义/不兼容项、候选选择器、密钥输入和警告；
3. 只有所有必需引用已解析、阶段可用且必需密钥已填写时才允许确认。成功后刷新部署配置列表并关闭 Dialog。

界面不让用户手填资源 ID。RepositoryBinding、集群、镜像站、变量集、运行配置集、Hook 和数据卷都使用服务端返回的受控候选。无候选时提供对应管理入口说明；关闭或切换文件要清空旧预检、候选和密钥草稿，避免展示上一文件状态。

## 7. 端到端与验收

至少覆盖：

- 同应用导出后预检和导入到新阶段；
- 跨项目唯一匹配、缺失、歧义、无权限和数据卷集群/模式不兼容；
- 空应用的 image 成功与 repository 阻断；
- Secret 只导出 requirement、缺值阻断、提交后 Secret Store 有引用且响应/审计无明文；
- 阶段冲突、digest 变化、重复键、未知字段、超限和不支持版本；
- 预检后资源删除或权限撤销时提交重新校验并零写入；
- 事务中 Hook/Volume/构建环境任一失败时 DeploymentTarget 不落库；
- 导入后不产生 BuildRun、Release 或任务投递；
- 前端加载、文件错误、无权限、需要映射、成功和 API 错误状态；
- 中英文文档、OpenAPI、TypeScript 类型/API Client、CLI/Agent 可见性保持一致。

前端浏览器验收至少覆盖空应用、跨项目缺仓库、密钥重填、窄屏 Dialog 和成功导入后的列表权威回读。
