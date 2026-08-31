# 使用 kubectl 管理项目资源

Luna DevOps 可以生成项目空间或应用范围的 kubeconfig。`kubectl` 连接的是 Luna DevOps 的 Kubernetes API 兼容网关，而不是运行集群的 kube-apiserver；真实集群地址、管理 kubeconfig 和上游 ServiceAccount Token 不会下发到本机。

## 前置条件

- 账号仍是目标项目空间的有效成员。
- 目标运行集群处于“可用”状态，平台管理员已经启用 kubectl 网关，并且实时状态为“已就绪”。
- 管理员已为平台公开地址配置 HTTPS，以及支持长连接的反向代理。部署要求见 [Kubernetes (Helm) 部署](/start/install/kubernetes#配置-kubectl-网关反向代理)。
- 本机安装了当前平台版本验证范围内的 `kubectl`；准确版本以[兼容范围](/reference/compatibility)和当前 Release 说明为准。

## 从控制台获取 kubeconfig

1. 打开目标项目空间，进入“kubectl 访问”。
2. 点击“创建 kubeconfig”，选择一个可用运行集群，并按需把范围限制到单个应用。
3. 选择最小权限和 1、7 或 30 天有效期，然后创建凭据。
4. 立即保存自动下载的 kubeconfig。明文只返回这一次，关闭弹窗后无法再次查看或下载。
5. 确认文件只有当前用户可读：

```bash
chmod 600 "$HOME/.kube/luna-project.yaml"
```

控制台一次创建一个 Context。需要撤销时，打开“账号设置 → kubectl 凭据”；可以查看 Context 元数据，但不能找回原 Token 或 kubeconfig。撤销后，已有文件也会失效。

## 使用 Luna CLI 写入或合并

Luna CLI 的专用命令不会把 kubeconfig 或 Token 输出到普通 stdout。写入新文件：

```bash
luna kubeconfig write \
  credentialName=dev-read \
  context=prj_example:clu_example \
  scope=read \
  expiresInDays=7 \
  destination="$HOME/.kube/luna-dev-read.yaml"
```

合并到现有 kubeconfig：

```bash
luna kubeconfig merge \
  credentialName=dev-tools \
  context=prj_example:clu_example:app_example \
  scope=read \
  scope=connect \
  expiresInDays=1 \
  destination="$HOME/.kube/config"
```

重复 `context=...` 可以一次生成多个 Context。格式为 `projectId:runtimeClusterId[:applicationId]`；`scope=read|write|connect` 分别映射到 `kube:read|kube:write|kube:connect`。CLI 会先检查 YAML、目标权限和同名冲突，再以 `0600` 原子写入；只有确认要覆盖内容不同的同名项时才增加 `replaceConflicts=true`。

创建和管理凭据需要当前 Luna CLI OAuth 会话具有 `token:manage`。缺少时按 CLI 提示重新授权，不要改用普通 Access Token 代替 Kube Credential。

## 选择 Context 并验证权限

先单独使用下载文件，避免误改默认 kubeconfig：

```bash
export KUBECONFIG="$HOME/.kube/luna-dev-read.yaml"
kubectl config get-contexts
kubectl config current-context
kubectl auth whoami
kubectl auth can-i get deployments
kubectl get deployments
```

生成的 Context 名使用稳定资源 ID，并固定目标项目的 Kubernetes Namespace。更换 `-n` 不能越过这个边界，`-A` 会被拒绝；应用范围的 Context 还会强制限制到该应用的归属标签。

## 权限如何生效

| Scope | 主要用途 | 说明 |
| --- | --- | --- |
| `kube:read` | Discovery、OpenAPI、`get/list/watch`、`describe`、`wait`、`top`、日志和授权自检 | 仍需当前项目角色允许相应读取动作；Secret 原值另有更高权限要求。 |
| `kube:write` | `create/apply/edit/replace/patch/delete`、Scale 和 Rollout 等写操作 | 自动包含 `kube:read`，但不会绕过工作负载安全策略或平台业务 Action。 |
| `kube:connect` | `exec`、`attach`、`port-forward`、`cp` 和受控 Debug | 自动包含 `kube:read`；连接仍要求 Developer 及以上角色、`deployment:exec` 和项目 Web Console 开关。 |

每次请求都会重新判断 Credential、Binding、项目成员、当前角色、集群状态、Namespace、资源类型和对象归属。签发 kubeconfig 不会冻结权限；角色降低、成员移除、应用或集群停用后，旧文件不会保留原权限。已建立的 Watch、日志或连接会定期复核，撤权后最长约 30 秒收口。

## 支持范围与固定限制

网关在授权范围内支持标准 Discovery/OpenAPI、读取与输出格式、CRUD/Apply/Patch、Watch、日志、Exec、Attach、Port-forward、`cp`、授权自检，以及 Kubernetes 原生 Status、Table、分页和流式语义。`kubectl config`、`completion`、`kustomize` 和插件等纯客户端功能仍由本机 `kubectl` 处理。

以下边界不能通过增加 Scope 绕过：

- 不能管理 Node、PV、集群 RBAC、CRD、Webhook、CSR、APIService 或集群级 Gateway，也不支持 `kubectl --as`。
- 不能写 Namespace、ServiceAccount 或其 Token 子资源，不能使用 Node Debug、资源 `proxy` 或跨 Namespace 引用。
- 工作负载不能使用 privileged、host namespace、hostPath、hostPort、提权、额外 Linux capability、任意 ServiceAccount、Projected ServiceAccount Token 或 CSI Secret provider。
- Service 只能使用 ClusterIP；外部入口继续通过 Luna DevOps 的访问入口管理。
- Secret 原值只对当前角色同时具有 `secret:view_value` 的 Owner 或 Admin 开放。具有 Exec 权限的用户仍可能读取容器内已经注入的 Secret，因此不要把 Exec 凭据当成普通只读凭据。

平台创建资源时，数据库中的部署配置仍是期望状态。通过 kubectl 对这些对象做的临时修改可能在下一次发布、回滚或协调时被覆盖。kubectl 新建的资源由 Kubernetes 保存期望状态，并按项目归属参与清理、观察和运行资源计费。

## 常见问题

| 现象 | 处理方式 |
| --- | --- |
| 创建时显示网关已关闭、协调中或不可用 | 请管理员在“运行集群”中启用 kubectl 网关，等待实时状态变为“已就绪”；不可用时检查运行集群连接和网关协调任务。 |
| `401 Unauthorized` | Credential 已过期、撤销、用户被禁用，或使用了普通 Access Token；重新创建 Kube Credential。 |
| `403 Forbidden` | 当前 Scope、项目角色、业务 Action、资源目录或应用范围不允许该操作；用 `kubectl auth can-i` 检查，不要尝试更换 Namespace 绕过。 |
| `404 NotFound` | 对象不存在，或对象不属于当前项目/应用 Binding；网关会隐藏范围外对象的存在性。 |
| `422 Unprocessable Entity` | Namespace、保留归属标签、引用关系或工作负载安全策略不合法；按返回的 Kubernetes Status 修正清单。 |
| `429 TooManyRequests` | 请求频率或流连接数超过限制；关闭闲置 Watch、日志跟随、Exec 或 Port-forward 后重试。 |
| `503/504` | 运行集群、Secret Store、TokenRequest 或上游 API 暂时不可用；稍后重试并让管理员检查集群状态。应用范围的 `kubectl top` 在 Metrics Provider 无法证明标签隔离时也会失败关闭。 |
| Watch、日志或连接定期断开 | Watch 最长 30 分钟，日志和连接最长 2 小时；这是重新鉴权边界，客户端可以按标准行为重连。 |

不再需要访问时，先在“账号设置 → kubectl 凭据”撤销，再安全删除本地文件。不要把 kubeconfig 提交到代码仓库、聊天记录或工单附件。
