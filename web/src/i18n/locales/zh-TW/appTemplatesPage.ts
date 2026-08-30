const appTemplatesPage = {
  description: '從預設模板一鍵安裝資料庫、快取、監控、工具和輕量協作應用。',
  heroEyebrow: 'Luna DevOps 模板庫',
  heroTitle: '發現服務，更快開始部署',
  templateCount: '可用模板',
  categoryCount: '服務分類',
  browseTitle: '瀏覽全部模板',
  resultCount: '找到 {{count}} 個可用模板',
  searchPlaceholder: '搜尋應用、用途或映象',
  categoryFilter: '分類篩選',
  allCategories: '全部分類',
  sortBy: '排序欄位',
  sortByPopularity: '熱度',
  sortByName: '名稱 A-Z',
  sortOrder: '排序順序',
  sortDesc: '倒序',
  sortAsc: '順序',
  loading: '正在載入應用模板...',
  emptyTitle: '沒有找到模板',
  emptyDescription: '換個關鍵詞試試，或等待管理員新增更多模板。',
  install: '安裝',
  installing: '安裝中...',
  installStarted: '應用模板已開始安裝',
  systemInstallStarted: '平臺元件已開始安裝',
  installDialogTitle: '安裝 {{name}}',
  installDialogDescription: '模板會在目標專案空間中建立一個應用和部署配置；金鑰類引數會安全儲存，不會明文回顯。',
  systemInstallDialogDescription: '平臺元件會安裝到指定執行叢集的系統名稱空間，用於增強平臺能力，不會建立專案空間應用。',
  platformComponent: '平臺元件',
  selectRuntimeCluster: '請選擇執行叢集',
  componentNamespace: '元件名稱空間',
  systemInstallAdminOnly: '只有平臺管理員可以安裝平臺元件。',
  runtimeCluster: '執行叢集',
  defaultCluster: '使用預設叢集',
  applicationName: '應用名稱',
  applicationIdentifier: '應用標識',
  deploymentName: '部署配置',
  stage: '階段',
  imageRef: '映象地址',
  imageRefHint: '預設使用模板映象；如果你有 Harbor、DockerHub 代理或私有映象，可以改成自己的完整映象地址。',
  provisionAccess: '由平台建立 ServiceAccount 與 RBAC 權限',
  provisionAccessDescription: '勾選後平台會為該元件建立專用 ServiceAccount 並授予讀取 Gateway API 路由的權限；不勾選則使用命名空間預設賬號，由你自行設定 RBAC。',
  replicas: '副本數',
  cpu: 'CPU',
  memory: '記憶體',
  projectVolume: '專案資料卷',
  selectProjectVolume: '請選擇可掛載的專案資料卷',
  projectVolumeNotRequired: '此模板不需要持久化資料卷',
  templateParameters: '模板引數',
  templateParametersDescription: '留空的自動生成金鑰會由後端生成並寫入金鑰儲存。',
  autoGeneratePlaceholder: '留空自動生成',
  installNow: '安裝後立即部署',
  installNowDescription: '關閉後只建立應用和部署配置，之後可在應用部署頁手動釋出。',
  image: '映象',
  officialWebsite: '官方網站',
  officialRepository: '官方倉庫',
  port: '埠',
  resources: '資源',
  categories: {
    collaboration: '協作與內容',
    database: '資料庫',
    developerTool: '開發工具',
    middleware: '中介軟體',
    observability: '監控觀測',
    security: '安全工具',
    storage: '儲存',
  },
  stageOptions: {
    prod: '生產',
    staging: '預發',
    test: '測試',
    dev: '開發',
  },
  valueLabels: {
    username: '使用者名稱',
    database: '資料庫名',
    password: '密碼',
    rootPassword: 'Root 密碼',
    rpcSecret: 'RPC 金鑰',
    adminToken: '管理 Token',
    metricsToken: '指標 Token',
    masterKey: '主金鑰',
    apiKey: 'API Key',
    mongodbUrl: 'MongoDB 地址',
    dbHost: '資料庫地址',
    email: '郵箱',
    apiBaseUrl: 'Luna DevOps API 基礎地址',
    traefikMetricsUrl: 'Traefik Metrics 地址',
  },
  valueHints: {
    redisPassword: '選填。建議設定較長的隨機密碼；留空時 Redis 不啟用密碼驗證。',
    apiBaseUrl: '填寫平臺對探針可訪問的基礎地址，例如 https://luna-devops.example.com；不要填寫 /api/v1/billing/gateway-traffic 這類具體介面路徑，探針會自動拼接上報介面。',
    traefikMetricsUrl: '填寫探針 Pod 在叢集內可訪問的 Traefik Prometheus metrics 地址。留空時預設使用 http://traefik.<Gateway 名稱空間>.svc.cluster.local:9100/metrics。',
  },
  valuePlaceholders: {
    redisPassword: '建議設定隨機密碼（可留空）',
    apiBaseUrl: 'https://luna-devops.example.com',
    traefikMetricsUrl: 'http://traefik.kube-system.svc.cluster.local:9100/metrics',
  },
  templates: {
    'luna-gateway-traffic-probe': {
      description: '可選平臺元件，用於採集 Gateway API 訪問流量視窗並上報到賬單。',
    },
    'redis': {
      description: '用於快取、佇列和輕量協調的記憶體資料儲存。',
    },
    'postgresql': {
      description: '適合應用資料、後設資料和事務型場景的關係型資料庫。',
    },
    'mysql': {
      description: '經典關係型資料庫，使用單容器快速安裝。',
    },
    'mongodb': {
      description: '面向 JSON 類資料和靈活 schema 的文件資料庫。',
    },
    'mariadb': {
      description: '相容 MySQL 的關係型資料庫，適合輕量單容器執行。',
    },
    'valkey': {
      description: '相容 Redis 的記憶體資料儲存，適合快取和輕量佇列。',
    },
    'memcached': {
      description: '極簡高速記憶體快取服務，適合簡單 key-value 快取。',
    },
    'rabbitmq': {
      description: '用於非同步任務、事件和輕量訊息佇列的訊息中介軟體。',
    },
    'meilisearch': {
      description: '輕量全文搜尋引擎，適合應用內搜尋和索引場景。',
    },
    'grafana': {
      description: '指標儀表盤和視覺化平臺，適合監控資料展示。',
    },
    'prometheus': {
      description: '時序指標資料庫和抓取引擎，適合作為可觀測資料底座。',
    },
    'uptime-kuma': {
      description: '自託管可用性監控，用於站點、API 和內部服務巡檢。',
    },
    'memos': {
      description: '小團隊自託管筆記和知識片段工具。',
    },
    'it-tools': {
      description: '瀏覽器裡的開發和運維小工具集合。',
    },
    'excalidraw': {
      description: '簡單易用的白板工具，適合畫圖、草圖和產品說明。',
    },
    'verdaccio': {
      description: '私有 npm 相容包倉庫，適合內部 JavaScript 包分發。',
    },
    'docker-registry': {
      description: '私有 OCI 映象倉庫，適合內部容器映象分發。',
    },
    'pgadmin4': {
      description: 'PostgreSQL 的 Web 管理控制檯。',
    },
    'bytebase': {
      description: '面向小團隊的資料庫變更、評審和釋出工作流。',
    },
    'garage': {
      description: '輕量 S3 相容物件儲存，適合專案檔案和構建產物存放。',
    },
    'nats': {
      description: '輕量訊息系統，適合事件、釋出訂閱和服務間通訊。',
    },
    'clickhouse': {
      description: '面向日誌、事件和實時分析的列式資料庫。',
    },
    'qdrant': {
      description: '向量資料庫，適合語義搜尋、推薦和 AI 檢索場景。',
    },
    'typesense': {
      description: '支援容錯輸入的快速搜尋引擎，適合商品搜尋和應用內搜尋。',
    },
    'adminer': {
      description: '輕量資料庫 Web 管理工具，支援 MySQL、PostgreSQL、SQLite 等。',
    },
    'mongo-express': {
      description: 'MongoDB 的 Web 管理介面。',
    },
    'caddy': {
      description: '簡單的 Web 服務和反向代理啟動模板，預設返回一段靜態響應。',
    },
    'gitea': {
      description: '輕量自託管 Git 服務，適合倉庫、Issue 和 Release 管理。',
    },
    'vaultwarden': {
      description: '相容 Bitwarden 客戶端的輕量自託管密碼管理器。',
    },
    'nocodb': {
      description: '開源無程式碼資料庫介面，適合小型內部工具和資料應用。',
    },
    'wiki-js': {
      description: '現代自託管 Wiki，適合團隊文件和知識庫。',
    },
    'wordpress': {
      description: '常用自託管 CMS 和部落格平臺，需要連線已有 MySQL 相容資料庫。',
    },
  },
}

export default appTemplatesPage
