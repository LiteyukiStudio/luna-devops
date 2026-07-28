# Kubernetes (Helm) 部署

如果准备把 Luna DevOps 长期运行在 Kubernetes 或 K3s 中，推荐使用 Helm。Chart 会一起部署 API、Worker、PostgreSQL 和 Redis，也可以改为连接已有的外部数据库与 Redis。

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

这会启动：

```text
liteyukistudio/devops-api:nightly
liteyukistudio/devops-worker:nightly
postgres:17-alpine
redis:8-alpine
```

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
  --set worker.image.tag=v0.1.0-rc.1
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

内置 Redis 会在首次安装时生成密码，并在 Kubernetes Secret 中分别保存 `redis-password` 和供 API/Worker 使用的 `redis-url`，后续升级会复用已有 Secret。为内置 Redis 指定 `redis.auth.existingSecret` 时，该 Secret 需要同时提供这两个 key。接入外部 Redis 时，可以直接填写 `externalRedis.url`，也可以使用只包含 `redis-url` 的 `externalRedis.existingSecret`。外部 Redis URI 格式为 `redis://用户名:密码@域名:端口/数据库`，TLS 连接使用 `rediss://`。

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
| `app.publicBaseUrl` | `http://localhost:8088` | 控制台对外访问地址。启用 Ingress 后必须改成公网地址。 |
| `app.secretEncryptionKey` | 首次安装自动生成 | 用于加密 Git、镜像站和 OIDC 密钥。生产环境要保持稳定。 |
| `api.image.tag` / `worker.image.tag` | `nightly` | API 和 worker 镜像版本。 |
| `postgresql.enabled` | `true` | 是否安装内置 PostgreSQL。 |
| `redis.enabled` | `true` | 是否安装内置 Redis。 |
| `externalRedis.url` | 空 | 外部 Redis 完整连接 URI；关闭内置 Redis 时配置。 |
| `worker.buildEgressMode` | `permissive` | 构建 Job 出站网络模式。需要强隔离时改为 `restricted`。 |

## 卸载

```bash
helm uninstall luna-devops -n luna-devops
```

PVC 默认会保留，避免误删数据。确认不再需要这些数据后，再手动清理：

```bash
kubectl -n luna-devops delete pvc -l app.kubernetes.io/instance=luna-devops
```
