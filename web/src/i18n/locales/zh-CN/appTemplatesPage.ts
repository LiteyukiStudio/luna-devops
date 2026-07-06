const appTemplatesPage = {
  description: '从预设模板一键安装数据库、缓存、监控、工具和轻量协作应用。',
  heroTitle: '应用市场',
  heroDescription: '选择一个内置模板，填写少量参数后安装到项目空间；平台会创建应用、部署配置，并按需立即发布。',
  searchPlaceholder: '搜索模板或镜像',
  categoryFilter: '分类筛选',
  allCategories: '全部分类',
  sortBy: '排序字段',
  sortByPopularity: '热度',
  sortByName: '名称 A-Z',
  sortOrder: '排序顺序',
  sortDesc: '倒序',
  sortAsc: '顺序',
  loading: '正在加载应用模板...',
  emptyTitle: '没有找到模板',
  emptyDescription: '换个关键词试试，或等待管理员添加更多模板。',
  install: '安装',
  installing: '安装中...',
  installStarted: '应用模板已开始安装',
  systemInstallStarted: '平台组件已开始安装',
  installDialogTitle: '安装 {{name}}',
  installDialogDescription: '模板会在目标项目空间中创建一个应用和部署配置；密钥类参数会安全存储，不会明文回显。',
  systemInstallDialogDescription: '平台组件会安装到指定运行集群的系统命名空间，用于增强平台能力，不会创建项目空间应用。',
  platformComponent: '平台组件',
  selectRuntimeCluster: '请选择运行集群',
  componentNamespace: '组件命名空间',
  systemInstallAdminOnly: '只有平台管理员可以安装平台组件。',
  runtimeCluster: '运行集群',
  defaultCluster: '使用默认集群',
  applicationName: '应用名称',
  applicationSlug: '应用标识',
  deploymentName: '部署配置',
  stage: '阶段',
  imageRef: '镜像地址',
  imageRefHint: '默认使用模板镜像；如果你有 Harbor、DockerHub 代理或私有镜像，可以改成自己的完整镜像地址。',
  replicas: '副本数',
  cpu: 'CPU',
  memory: '内存',
  dataCapacity: '数据容量',
  templateParameters: '模板参数',
  templateParametersDescription: '留空的自动生成密钥会由后端生成并写入密钥存储。',
  autoGeneratePlaceholder: '留空自动生成',
  installNow: '安装后立即部署',
  installNowDescription: '关闭后只创建应用和部署配置，之后可在应用部署页手动发布。',
  image: '镜像',
  officialWebsite: '官方网站',
  officialRepository: '官方仓库',
  port: '端口',
  resources: '资源',
  categories: {
    cache: '缓存',
    collaboration: '协作',
    database: '数据库',
    databaseTool: '数据库工具',
    developerTool: '开发工具',
    middleware: '中间件',
    objectStorage: '对象存储',
    observability: '监控观测',
    registry: '制品仓库',
    search: '搜索',
  },
  stageOptions: {
    prod: '生产',
    staging: '预发',
    test: '测试',
    dev: '开发',
  },
  valueLabels: {
    username: '用户名',
    database: '数据库名',
    password: '密码',
    rootPassword: 'Root 密码',
    rpcSecret: 'RPC 密钥',
    adminToken: '管理 Token',
    metricsToken: '指标 Token',
    masterKey: '主密钥',
    email: '邮箱',
    apiBaseUrl: 'DevOps API 基础地址',
    traefikMetricsUrl: 'Traefik Metrics 地址',
  },
  valueHints: {
    apiBaseUrl: '填写平台对探针可访问的基础地址，例如 https://devops.liteyuki.org；不要填写 /api/v1/billing/gateway-traffic 这类具体接口路径，探针会自动拼接上报接口。',
    traefikMetricsUrl: '填写探针 Pod 在集群内可访问的 Traefik Prometheus metrics 地址。留空时默认使用 http://traefik.<Gateway 命名空间>.svc.cluster.local:9100/metrics。',
  },
  valuePlaceholders: {
    apiBaseUrl: 'https://devops.liteyuki.org',
    traefikMetricsUrl: 'http://traefik.kube-system.svc.cluster.local:9100/metrics',
  },
  templates: {
    'liteyuki-gateway-traffic-probe': {
      description: '可选平台组件，用于采集 Gateway API 访问流量窗口并上报到账单。',
    },
    'redis': {
      description: '用于缓存、队列和轻量协调的内存数据存储。',
    },
    'postgresql': {
      description: '适合应用数据、元数据和事务型场景的关系型数据库。',
    },
    'mysql': {
      description: '经典关系型数据库，使用单容器快速安装。',
    },
    'mongodb': {
      description: '面向 JSON 类数据和灵活 schema 的文档数据库。',
    },
    'mariadb': {
      description: '兼容 MySQL 的关系型数据库，适合轻量单容器运行。',
    },
    'valkey': {
      description: '兼容 Redis 的内存数据存储，适合缓存和轻量队列。',
    },
    'memcached': {
      description: '极简高速内存缓存服务，适合简单 key-value 缓存。',
    },
    'rabbitmq': {
      description: '用于异步任务、事件和轻量消息队列的消息中间件。',
    },
    'meilisearch': {
      description: '轻量全文搜索引擎，适合应用内搜索和索引场景。',
    },
    'grafana': {
      description: '指标仪表盘和可视化平台，适合监控数据展示。',
    },
    'prometheus': {
      description: '时序指标数据库和抓取引擎，适合作为可观测数据底座。',
    },
    'uptime-kuma': {
      description: '自托管可用性监控，用于站点、API 和内部服务巡检。',
    },
    'memos': {
      description: '小团队自托管笔记和知识片段工具。',
    },
    'it-tools': {
      description: '浏览器里的开发和运维小工具集合。',
    },
    'excalidraw': {
      description: '简单易用的白板工具，适合画图、草图和产品说明。',
    },
    'verdaccio': {
      description: '私有 npm 兼容包仓库，适合内部 JavaScript 包分发。',
    },
    'docker-registry': {
      description: '私有 OCI 镜像仓库，适合内部容器镜像分发。',
    },
    'pgadmin4': {
      description: 'PostgreSQL 的 Web 管理控制台。',
    },
    'bytebase': {
      description: '面向小团队的数据库变更、评审和发布工作流。',
    },
    'garage': {
      description: '轻量 S3 兼容对象存储，适合项目文件和构建产物存放。',
    },
  },
}

export default appTemplatesPage
