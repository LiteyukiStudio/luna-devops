# Kubernetes (Helm) 部署

如果准备把 Luna DevOps 长期运行在 Kubernetes 中，推荐使用 Helm。Chart 会一起部署 API、Worker、PostgreSQL 和 Redis，也可以改为连接已有的外部数据库与 Redis。

本文命令以标准 Kubernetes 为基准，也兼容满足[兼容范围](/reference/compatibility)的 K3s 和其他 Kubernetes 发行版；发行版特有的存储、Ingress 或安全策略需要由集群管理员适配。

## 开始前准备

你需要：

- 一个可用的 Kubernetes 或 K3s 集群。
- 本机已经配置好 `kubectl` 和 `helm`。
- 集群能拉取 DockerHub 镜像。
- 默认 StorageClass 可用，用来保存 PostgreSQL 和 Redis 数据。

## 安装

在仓库根目录执行：

```bash
helm install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace
```

默认会同时安装 API、Worker、PostgreSQL 和 Redis。AI 助手默认关闭；启用时设置 `ai.enabled=true`，并通过 `ai.existingSecret` 提供稳定的 `ai-internal-secret`。

## 打开控制台

先把 API Service 转发到本机：

```bash
kubectl -n luna-devops port-forward svc/luna-devops-api 8088:80
```

然后访问：

```text
http://localhost:8088
```

## 使用固定版本

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  --set api.image.tag=v0.1.0-rc.1 \
  --set worker.image.tag=v0.1.0-rc.1 \
  --set ai.agent.image.tag=v0.1.0-rc.1
```

## 通过公网域名访问

如果通过 Ingress 暴露控制台，把 `app.publicBaseUrl` 改成用户真实访问的地址：

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  --set app.publicBaseUrl=https://devops.example.com \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=devops.example.com
```

`app.publicBaseUrl` 会影响 OIDC 回调、Webhook 回调和浏览器跨域校验，不要写成集群内 Service 地址。

## 使用外部 PostgreSQL 或 Redis

内置数据库适合快速启动。生产环境已经有托管 PostgreSQL 或 Redis 时，可以关闭对应内置组件：

```yaml
postgresql:
  enabled: false
externalDatabase:
  url: postgres://devops:password@postgres.example.com:5432/devops?sslmode=disable

redis:
  enabled: false
externalRedis:
  url: redis://default:replace-with-a-strong-password@redis.example.com:6379/0
```

外部 Redis 可以使用连接 URI 或现有 Secret；TLS 连接使用 `rediss://`。生产环境请通过 Kubernetes Secret 提供凭据，不要把密码直接提交到 values 文件。

然后安装：

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  -f values-prod.yaml
```

## 常用配置

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| `app.publicBaseUrl` | `http://localhost:8088` | 设置用户访问平台的根地址；填写 HTTP(S) URL。 |
| `app.secretEncryptionKey` | 自动生成 | 加密平台保存的凭据；填写稳定的非空密钥。 |
| `api.image.tag` / `worker.image.tag` | `nightly` | 选择 API 与 Worker 镜像版本；填写镜像标签。 |
| `ai.enabled` / `ai.existingSecret` | `false` / 空 | 启用 Agent 并指定内部密钥；分别填写布尔值和 Kubernetes Secret 名称。 |
| `postgresql.enabled` / `externalDatabase.url` | `true` / 空 | 选择内置或外部 PostgreSQL；分别填写布尔值和 PostgreSQL 连接 URI。 |
| `redis.enabled` / `externalRedis.url` | `true` / 空 | 选择内置或外部 Redis；分别填写布尔值和 `redis://` 或 `rediss://` URI。 |
| `worker.buildEgressMode` | `restricted` | 设置构建网络出口策略；可填 `restricted` 或 `permissive`。 |

## 卸载

```bash
helm uninstall luna-devops -n luna-devops
```

PVC 默认会保留，避免误删数据。确认不再需要这些数据后，再手动清理：

```bash
kubectl -n luna-devops delete pvc -l app.kubernetes.io/instance=luna-devops
```
