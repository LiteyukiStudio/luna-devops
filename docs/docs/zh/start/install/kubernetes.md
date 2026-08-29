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

数据库全新时，先把首个管理员配置保存到 Kubernetes Secret。下面的文件应设置为仅当前用户可读，用完后安全删除；名称和语言键可以省略，API 会分别回退为邮箱和 `zh-CN`：

```dotenv title="initial-admin.env"
initial-admin-email=admin@example.com
initial-admin-password=请替换为8至72字节的强密码
```

然后在仓库根目录执行：

```bash
kubectl create namespace luna-devops
kubectl -n luna-devops create secret generic luna-devops-initial-admin \
  --from-env-file=initial-admin.env
helm install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --set api.initialAdmin.existingSecret=luna-devops-initial-admin
```

默认会同时安装 API、Worker、PostgreSQL 和 Redis。管理员 Secret 只注入 API；API 仅在全新数据库中创建首个管理员，不会在升级或重启时覆盖已有账号。已有有效管理员的数据库可以不设置 `api.initialAdmin`，Chart 仍能正常安装或升级。AI 助手默认关闭；启用时设置 `ai.enabled=true`，并通过 `ai.existingSecret` 提供稳定的 `ai-internal-secret`。

确认可以登录后，可以让 API 脱离初始化 Secret，再删除它：

```bash
helm upgrade luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --reuse-values \
  --set-string api.initialAdmin.existingSecret=
kubectl -n luna-devops delete secret luna-devops-initial-admin
```

四个 Secret 键引用都是可选的；`existingSecret`、`email`、`name`、`password` 和 `language` 均为空时，Chart 不会创建初始管理员 Secret。Chart 仅在显式提供任一管理员字段时创建受管 Secret，密码和语言也只在非空时校验。

## 打开控制台

先把 API Service 转发到本机：

```bash
kubectl -n luna-devops port-forward svc/luna-devops-api 8088:80
```

然后访问：

```text
http://localhost:8088/login
```

## 使用固定版本

```bash
helm upgrade --install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace \
  --set api.initialAdmin.existingSecret=luna-devops-initial-admin \
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
  --set api.initialAdmin.existingSecret=luna-devops-initial-admin \
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

## 配置 Agent 网络策略

Chart 默认启用 API 到 Agent 的入站隔离，但不默认限制 Agent 出站。Agent 需要访问模型 Provider、
OpenTelemetry Collector 和 PostgreSQL；其中动态域名无法由原生 Kubernetes NetworkPolicy 可靠表达。

只有列全实际目的地后才启用出站隔离，例如：

```yaml
ai:
  agent:
    networkPolicy:
      egress:
        enabled: true
        additionalCIDRs:
          - 203.0.113.10/32
        additionalRules:
          - to:
              - namespaceSelector:
                  matchLabels:
                    kubernetes.io/metadata.name: observability
            ports:
              - protocol: TCP
                port: 4318
```

模型 Provider 使用动态地址时，请改用支持 FQDN 策略的 CNI 或稳定出口代理，不要把未覆盖真实目的地
的 deny-all 规则当作可用配置。Agent 的非 root 用户、只读根文件系统和禁用 ServiceAccount Token
不受此开关影响。

## 常用配置

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| `app.publicBaseUrl` | `http://localhost:8088` | 设置 API 回调和 Worker 通知详情链接共用的平台根地址；生产环境填写用户实际访问的绝对 HTTP(S) URL。 |
| `app.secretEncryptionKey` | 自动生成 | 加密平台保存的凭据；填写稳定的非空密钥。 |
| `api.initialAdmin.existingSecret` | 空 | 指定首个管理员配置 Secret；全新数据库需包含 `initial-admin-email/password`，可选 `initial-admin-name/language`。 |
| `api.initialAdmin.email` / `password` | 空 | 让 Chart 在显式提供字段时创建首个管理员 Secret；全新数据库分别填写有效邮箱和 8–72 字节密码，生产环境优先使用 `existingSecret`。 |
| `api.initialAdmin.name` / `language` | 空 / 空 | 设置首个管理员名称和语言；留空时 API 分别使用邮箱和 `zh-CN`，语言非空时可填 `zh-CN` 或 `en-US`。 |
| `api.image.tag` / `worker.image.tag` | `nightly` | 选择 API 与 Worker 镜像版本；填写镜像标签。 |
| `api.database.maxOpenConns` / `maxIdleConns` | `20` / `5` | 限制每个 API 副本的 PostgreSQL 打开与空闲连接数；分别填写正整数和不超过前者的非负整数。 |
| `worker.database.maxOpenConns` / `maxIdleConns` | `20` / `5` | 限制每个 Worker 副本的 PostgreSQL 打开与空闲连接数；分别填写正整数和不超过前者的非负整数。 |
| `ai.enabled` / `ai.existingSecret` | `false` / 空 | 启用 Agent 并指定内部密钥；分别填写布尔值和 Kubernetes Secret 名称。 |
| `ai.agent.observabilityCaptureDatabaseSpans` | `false` | 控制是否临时采集逐条 Agent PostgreSQL Span；填写布尔值，常规运行保持关闭。 |
| `ai.agent.networkPolicy.ingress.enabled` / `egress.enabled` | `true` / `false` | 分别控制 API 到 Agent 的入站隔离和 Agent 出站隔离；填写布尔值，并仅在目的地规则完整时启用出站。 |
| `ai.agent.networkPolicy.egress.additionalCIDRs` / `additionalRules` | `[]` / `[]` | 补充 Agent 可访问目的地；分别填写 CIDR 列表和 Kubernetes NetworkPolicy egress rule 列表。 |
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
