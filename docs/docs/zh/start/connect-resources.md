# 添加基础资源

平台管理员应先准备以下资源，让普通用户创建应用后可以直接构建或部署：

```text
运行集群 → 镜像站 → Git Provider OAuth
```

## 1. 添加运行集群

在“运行集群”中添加 Kubernetes 集群，粘贴平台 API 和 Worker 都能访问的 kubeconfig，然后执行连接测试。

- kubeconfig 的 API Server 必须使用 HTTPS。
- 平台运行在容器中时，不要使用仅宿主机可访问的 `127.0.0.1`。
- 按需配置默认 Gateway、域名后缀和 TLS；平台不会替你创建 ACME 账号或 DNS Provider 凭据。

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
