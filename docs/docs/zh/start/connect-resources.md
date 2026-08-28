# 添加基础资源

平台管理员应先准备以下资源，让普通用户创建应用后可以直接构建或部署：

```text
运行集群 → 镜像站 → Git Provider OAuth
```

## 选择全局资源范围

运行集群、镜像站、Git Provider、Git 账号、镜像与凭据、构建变量集等跨项目资源列表默认使用“与我相关”：保留全局资源、当前账号资源，以及绑定到本人所在项目空间的资源。平台管理员只有在明确进行全局盘点时才切换到“全部”。已知目标项目空间时继续选择该项目空间；项目空间条件会在当前可见范围内进一步缩小结果。

## 1. 添加运行集群

在“运行集群”中添加 Kubernetes 集群，粘贴平台 API 和 Worker 都能访问的 kubeconfig，然后执行连接测试。

- kubeconfig 的 API Server 必须使用 HTTPS。
- 平台运行在容器中时，不要使用仅宿主机可访问的 `127.0.0.1`。
- 按需配置默认 Gateway、域名后缀和 TLS；平台不会替你创建 ACME 账号或 DNS Provider 凭据。

在“资源分配策略”中设置 CPU request、内存 request、CPU limit、内存 limit 占应用配额的百分比。默认值依次为 `10% / 25% / 100% / 100%`，范围均为 0–100；`0%` 表示不向 Kubernetes 写入对应字段。启用 limit 时，request 百分比不能大于同类 limit 百分比；request-only 和 limit-only 都允许。策略修改只影响之后重新部署的工作负载，不会主动改写正在运行的资源。

Kubernetes 调度器仍按 requests 预留和调度容量。limits 是容器运行上限：CPU 超限通常触发节流，内存超限可能导致 OOMKill。

集群列表每约 10 秒读取一次 Kubernetes 当前状态。CPU、内存圆环展示“已调度且未终止 Pod 的有效 requests / 节点 allocatable 总量”；悬停时还会在 Metrics API 可用时展示实际节点用量。压力等级综合 CPU、内存的 request 分配率与实际用量，内存权重略高，并防止单项接近饱和时被平均值掩盖：低于 20 为“空闲”，20–44.9 为“轻度”，45–69.9 为“中度”，70–89.9 为“重度”，90 及以上为“满载”。这是容量概览，不替代 Kubernetes 对污点、亲和性、拓扑和单节点余量的最终调度判断。

删除集群前，先迁移或删除引用它的部署配置。

## 2. 添加镜像站

在“镜像站”中添加 Harbor、Gitea Registry、DockerHub 或通用 OCI Registry，并测试连接。

- **源码构建**需要当前用户或项目空间可用的“推送”或“拉取与推送”凭据。
- **已有镜像部署**需要确认目标集群拥有镜像拉取权限。
- 凭据应限制到必要的用户或项目空间；删除镜像站会一并删除其凭据。

## 3. 配置 Git Provider OAuth

在“代码仓库 → Git Provider”中创建 GitHub 或 Gitea Provider，选择 OAuth，并按表单显示的 Callback URL 在代码托管平台创建 OAuth App。

1. 填写 Provider 类型和服务地址。
2. 把 Luna DevOps 显示的 Callback URL 填入 OAuth App。
3. 将 Client ID 和 Client Secret 保存到 Provider。
4. 测试连接，并用普通用户完成一次 OAuth 授权。

GitLab 当前优先使用 PAT 凭据。若团队暂时只部署已有镜像，可以稍后再配置 Git Provider。

## 验证结果

使用一个非管理员测试账号确认：能看到可用集群、能选择镜像站、能通过 OAuth 连接代码账号并搜索仓库。完成后即可进入[日常交付](/use/workflow)。
