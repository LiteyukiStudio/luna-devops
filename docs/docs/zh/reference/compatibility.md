# 兼容范围

更新时间：2026-07-24。

以下版本是当前优先支持和测试的范围。新部署优先选择仍在维护的稳定版本；升级后应完成页面末尾的冒烟测试。

## 代码源与镜像站

| 组件 | 支持范围 | 备注 |
| --- | --- | --- |
| GitHub | GitHub.com；GHES `3.17 ~ 3.21` | 验证 OAuth、Webhook 和仓库读取 |
| Gitea | `1.20.x ~ 1.25.x` | 新部署建议当前稳定版 |
| GitLab | 暂不支持 | 当前使用 GitHub 或 Gitea |
| Docker Hub | 当前公开 API v2 | 注意限流和网络可达性 |
| Harbor | `>= 2.0`，重点验证 `2.10.x ~ 2.14.x` | 新部署建议当前维护版 |
| 通用 OCI Registry | Distribution API V2；OCI Distribution `1.0 ~ 1.1` | 无法列目录时仍可填写完整镜像地址 |

## 运行与构建

| 组件 | 支持范围 | 备注 |
| --- | --- | --- |
| Kubernetes / K3s | Kubernetes `1.34 ~ 1.36` | K3s 按内置 Kubernetes 版本判断 |
| Metrics Server | 与集群版本兼容 | 缺失时只影响实时资源指标 |
| Gateway API | `1.0.0 ~ 1.6.x` | 需要 CRD 和兼容控制器 |
| Traefik | `3.x` | 需要启用 Kubernetes Gateway Provider |
| cert-manager | `>= 1.0` | 版本需同时兼容当前 Kubernetes |
| PostgreSQL | `14 ~ 18` | 推荐 `17`；不支持 SQLite |
| Redis | `7.x ~ 8.x` | 当前不支持 Cluster 或 Sentinel |
| BuildKit | `v0.20+ rootless` | 默认并重点验证 `v0.24.0-rootless` |

## 登录、通知与可观测

| 组件 | 支持范围 | 备注 |
| --- | --- | --- |
| OIDC Provider | OIDC Core 1.0 + Discovery 1.0 | `issuer` 和回调地址必须可达且一致 |
| Prometheus | `2.40+` 或 `3.x` | API 可抓取，完整指标也可通过 OTLP 导出 |
| Grafana | `9.x ~ 12.x` | iframe 嵌入需由 Grafana 允许并处理认证 |
| SMTP | 标准 SMTP / STARTTLS | 凭据必须按 Secret 管理 |
| Webhook 通知 | HTTP / HTTPS endpoint | 目标平台鉴权和限流由对应配置负责 |

## 升级后验证

1. Git Provider：完成 OAuth、仓库与分支读取，并创建一次 Webhook。
2. 镜像站：搜索镜像、读取 Tag、推送构建产物并从运行集群拉取。
3. Kubernetes：创建构建 Job 和 Deployment，读取状态、日志与终端。
4. Gateway：确认 Gateway 和 HTTPRoute 均进入已接受、已生效状态。
5. OIDC：完成一次真实登录并检查回调地址。
6. 可观测：确认 API、Worker 和 Agent 的遥测可以查询。

更精确的兼容依据以各组件官方支持矩阵及 Luna DevOps 当前 Release 说明为准。
