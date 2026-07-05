# 应用市场

应用市场用于把常见基础设施按模板安装到项目空间。第一版内置 Redis、Valkey、Memcached、PostgreSQL、MySQL、MariaDB、MongoDB、RabbitMQ、Garage、Prometheus、Grafana、Uptime Kuma、Memos、IT-Tools、Excalidraw、Verdaccio、Docker Registry、pgAdmin4、Meilisearch 和 Bytebase，适合快速准备缓存、数据库、消息队列、对象存储、监控和小团队工具。

应用市场也可以承载少量平台组件模板。平台组件只允许平台管理员安装到运行集群，不会创建普通项目空间应用，也不会参与用户项目账单。当前内置的 `Liteyuki Gateway Traffic Probe` 用于可选开启访问流量计费采集。

模板安装会创建：

- 一个应用，并默认使用模板声明的图标；应用图标支持内置图标名、站内资源路径或 `http(s)` 图片地址。
- 一个镜像来源的部署配置。
- 按模板声明的环境变量、密钥变量和运行数据卷。
- 按模板声明的配置文件和密钥文件；敏感文件会写入 Kubernetes Secret。
- 可选的首个 Release；默认会立即投递部署任务。

模板卡片会把镜像、官方网站和官方仓库放在一起，方便安装前快速确认来源。没有独立官网的应用会把官方仓库作为官网入口。

密钥类参数只写入平台密钥存储，部署配置中保存的是密钥引用，不会在前端回显明文。

## 安装步骤

1. 进入“应用市场”。
2. 选择模板并点击“安装”。
3. 选择项目空间，确认应用名称、应用标识、运行集群、镜像地址、CPU、内存、副本数和数据容量。镜像地址默认填充模板镜像，也可以替换为 Harbor、DockerHub 代理或私有镜像的完整地址。
4. 填写模板参数。自动生成的密码可以留空，由后端生成。
5. 保持“安装后立即部署”开启，或关闭后稍后在应用部署页手动发布。

安装成功后，页面会跳转到新应用的部署页。

## 平台组件

平台组件的安装入口仍在“应用市场”，但流程和普通应用不同：

1. 平台管理员选择带有“平台组件”标识的模板。
2. 选择目标运行集群。
3. 填写组件需要的少量参数，例如 DevOps API 地址。
4. 平台创建系统组件安装记录，生成独立上报 Token，并由 Worker 在目标集群下发 `liteyuki-system` 命名空间、只读 RBAC、Secret、ConfigMap 和组件 Deployment。

未安装 Gateway Traffic Probe 时，账单页会把访问流量显示为不可用，并引导平台管理员从应用市场安装。组件安装后，探针首次成功上报一个时间窗口，账单页才会认为访问流量可用。

Gateway Traffic Probe 作为独立镜像 `liteyukistudio/devops-gateway-traffic-probe` 发布。安装时平台会注入 `API_BASE_URL`、`REPORT_TOKEN`、`RUNTIME_CLUSTER_ID`、`TRAEFIK_METRICS_URL` 等环境变量；其中 `REPORT_TOKEN` 只保存哈希到平台数据库，Pod 内 Secret 保存明文 token 用于上报。

模板列表支持按分类筛选、按模板名称、镜像、官网或仓库搜索，也可以按热度权重或名称排序。当前内置模板暂不收录 PHP 应用，例如 Adminer 和 phpMyAdmin。

## 当前限制

MVP 模板只支持镜像默认启动即可运行的应用。Prometheus 当前提供抓取自身 `/metrics` 的最小配置，Grafana 和 Prometheus 仍是独立单应用模板，不会自动创建 Grafana 数据源或发现业务工作负载。Garage 当前作为单节点轻量对象存储模板提供，平台会生成基础配置文件；多节点 layout 初始化、bucket/key 输出和更完整的连接信息会在后续模板 outputs 能力中补齐。

应用市场模板来自后端内置 JSON。后续第三方模板市场可以继续复用同一份 schema，由后端负责拉取、校验和缓存。
