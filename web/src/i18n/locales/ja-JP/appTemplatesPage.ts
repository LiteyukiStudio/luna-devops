const appTemplatesPage = {
  description: 'プリセットテンプレートからデータベース、キャッシュ、監視、ツール、軽量コラボレーションアプリをワンクリックでインストールします。',
  heroEyebrow: 'Luna DevOps テンプレートライブラリ',
  heroTitle: 'サービスを発見して、より早くデプロイを開始',
  templateCount: '利用可能なテンプレート',
  categoryCount: 'サービスカテゴリ',
  browseTitle: 'すべてのテンプレートを閲覧',
  resultCount: '{{count}} 個の利用可能なテンプレートが見つかりました',
  searchPlaceholder: 'アプリ、用途、イメージを検索',
  categoryFilter: 'カテゴリフィルター',
  allCategories: 'すべてのカテゴリ',
  sortBy: 'ソートフィールド',
  sortByPopularity: '人気',
  sortByName: '名前 A-Z',
  sortOrder: 'ソート順',
  sortDesc: '降順',
  sortAsc: '昇順',
  loading: 'アプリテンプレートを読み込み中...',
  emptyTitle: 'テンプレートが見つかりませんでした',
  emptyDescription: '別のキーワードを試すか、管理者がさらにテンプレートを追加するのをお待ちください。',
  install: 'インストール',
  installing: 'インストール中...',
  installStarted: 'アプリテンプレートのインストールを開始しました',
  systemInstallStarted: 'プラットフォームコンポーネントのインストールを開始しました',
  installDialogTitle: '{{name}} をインストール',
  installDialogDescription: 'テンプレートはターゲットプロジェクトスペースにアプリケーションとデプロイ設定を作成します。シークレット系パラメータは安全に保存され、平文で再表示されません。',
  systemInstallDialogDescription: 'プラットフォームコンポーネントは指定した実行クラスターのシステム名前空間にインストールされ、プラットフォーム機能を強化します。プロジェクトスペースアプリケーションは作成されません。',
  platformComponent: 'プラットフォームコンポーネント',
  selectRuntimeCluster: '実行クラスターを選択してください',
  componentNamespace: 'コンポーネント名前空間',
  systemInstallAdminOnly: 'プラットフォーム管理者のみがプラットフォームコンポーネントをインストールできます。',
  runtimeCluster: '実行クラスター',
  defaultCluster: 'デフォルトクラスターを使用',
  applicationName: 'アプリケーション名',
  applicationIdentifier: 'アプリケーション識別子',
  deploymentName: 'デプロイ設定',
  stage: 'ステージ',
  imageRef: 'イメージアドレス',
  imageRefHint: 'デフォルトではテンプレートイメージを使用します。Harbor、DockerHub プロキシ、またはプライベートイメージがある場合は、独自の完全なイメージアドレスに変更できます。',
  provisionAccess: 'プラットフォームで ServiceAccount と RBAC 権限を作成',
  provisionAccessDescription: '有効にすると、プラットフォームが専用 ServiceAccount を作成し、Gateway API ルートの読み取り権限を付与します。無効の場合は名前空間のデフォルトアカウントで実行され、RBAC は自分で管理します。',
  replicas: 'レプリカ数',
  cpu: 'CPU',
  memory: 'メモリ',
  projectVolume: 'プロジェクトデータボリューム',
  selectProjectVolume: '準備完了のプロジェクトデータボリュームを選択してください',
  projectVolumeNotRequired: 'このテンプレートは永続データボリュームを必要としません',
  templateParameters: 'テンプレートパラメータ',
  templateParametersDescription: '空欄の自動生成シークレットはバックエンドで生成され、シークレットストアに書き込まれます。',
  autoGeneratePlaceholder: '空欄で自動生成',
  installNow: 'インストール後すぐにデプロイ',
  installNowDescription: '無効にするとアプリケーションとデプロイ設定のみ作成され、後でアプリケーションデプロイページで手動リリースできます。',
  image: 'イメージ',
  officialWebsite: '公式サイト',
  officialRepository: '公式リポジトリ',
  port: 'ポート',
  resources: 'リソース',
  categories: {
    cache: 'キャッシュ',
    collaboration: 'コラボレーション',
    cms: 'コンテンツ管理',
    database: 'データベース',
    databaseTool: 'データベースツール',
    developerTool: '開発ツール',
    lowCode: 'ローコード',
    middleware: 'ミドルウェア',
    objectStorage: 'オブジェクトストレージ',
    observability: '監視・観測',
    passwordManager: 'パスワード管理',
    registry: 'アーティファクトリポジトリ',
    search: '検索',
    vectorDatabase: 'ベクトルデータベース',
    webServer: 'Web サービス',
  },
  stageOptions: {
    prod: '本番',
    staging: 'ステージング',
    test: 'テスト',
    dev: '開発',
  },
  valueLabels: {
    username: 'ユーザー名',
    database: 'データベース名',
    password: 'パスワード',
    rootPassword: 'Root パスワード',
    rpcSecret: 'RPC シークレット',
    adminToken: '管理 Token',
    metricsToken: 'メトリクス Token',
    masterKey: 'マスターキー',
    apiKey: 'API Key',
    mongodbUrl: 'MongoDB アドレス',
    dbHost: 'データベースアドレス',
    email: 'メールアドレス',
    apiBaseUrl: 'Luna DevOps API ベースアドレス',
    traefikMetricsUrl: 'Traefik Metrics アドレス',
  },
  valueHints: {
    apiBaseUrl: 'プラットフォームがプローブからアクセス可能なベースアドレスを入力してください。例：https://luna-devops.example.com。/api/v1/billing/gateway-traffic のような具体的なインターフェースパスは入力しないでください。プローブが自動的にレポートインターフェースを付加します。',
    traefikMetricsUrl: 'プローブ Pod がクラスター内でアクセス可能な Traefik Prometheus metrics アドレスを入力してください。空欄の場合、デフォルトで http://traefik.<Gateway 名前空間>.svc.cluster.local:9100/metrics を使用します。',
  },
  valuePlaceholders: {
    apiBaseUrl: 'https://luna-devops.example.com',
    traefikMetricsUrl: 'http://traefik.kube-system.svc.cluster.local:9100/metrics',
  },
  templates: {
    'luna-gateway-traffic-probe': {
      description: 'オプションのプラットフォームコンポーネント。Gateway API アクセス流量ウィンドウを収集して請求にレポートします。',
    },
    'redis': {
      description: 'キャッシュ、キュー、軽量調整用のインメモリデータストア。',
    },
    'postgresql': {
      description: 'アプリケーションデータ、メタデータ、トランザクションシナリオに適したリレーショナルデータベース。',
    },
    'mysql': {
      description: 'クラシックなリレーショナルデータベース。単一コンテナでクイックインストール。',
    },
    'mongodb': {
      description: 'JSON ライクなデータと柔軟なスキーマ向けのドキュメントデータベース。',
    },
    'mariadb': {
      description: 'MySQL 互換のリレーショナルデータベース。軽量単一コンテナ実行に適しています。',
    },
    'valkey': {
      description: 'Redis 互換のインメモリデータストア。キャッシュと軽量キューに適しています。',
    },
    'memcached': {
      description: 'ミニマルで高速なインメモリキャッシュサービス。シンプルな key-value キャッシュに適しています。',
    },
    'rabbitmq': {
      description: '非同期タスク、イベント、軽量メッセージキュー用のメッセージミドルウェア。',
    },
    'meilisearch': {
      description: '軽量全文検索エンジン。アプリ内検索とインデックスシナリオに適しています。',
    },
    'grafana': {
      description: 'メトリクスダッシュボードと可視化プラットフォーム。監視データ表示に適しています。',
    },
    'prometheus': {
      description: '時系列メトリクスデータベースとスクレイピングエンジン。可観測性データ基盤として適しています。',
    },
    'uptime-kuma': {
      description: 'セルフホスト可用性監視。サイト、API、内部サービスの監視に使用します。',
    },
    'memos': {
      description: '小規模チーム向けセルフホストノートと知識スニペットツール。',
    },
    'it-tools': {
      description: 'ブラウザ内の開発・運用ツールコレクション。',
    },
    'excalidraw': {
      description: 'シンプルで使いやすいホワイトボードツール。図、スケッチ、製品説明に適しています。',
    },
    'verdaccio': {
      description: 'プライベート npm 互換パッケージリポジトリ。内部 JavaScript パッケージ配布に適しています。',
    },
    'docker-registry': {
      description: 'プライベート OCI イメージリポジトリ。内部コンテナイメージ配布に適しています。',
    },
    'pgadmin4': {
      description: 'PostgreSQL の Web 管理コンソール。',
    },
    'bytebase': {
      description: '小規模チーム向けデータベース変更、レビュー、リリースワークフロー。',
    },
    'garage': {
      description: '軽量 S3 互換オブジェクトストレージ。プロジェクトファイルとビルド成果物の保存に適しています。',
    },
    'nats': {
      description: '軽量メッセージシステム。イベント、パブリッシュ/サブスクライブ、サービス間通信に適しています。',
    },
    'clickhouse': {
      description: 'ログ、イベント、リアルタイム分析向けのカラムナーデータベース。',
    },
    'qdrant': {
      description: 'ベクトルデータベース。セマンティック検索、レコメンデーション、AI 検索シナリオに適しています。',
    },
    'typesense': {
      description: '誤字耐性のある高速検索エンジン。商品検索とアプリ内検索に適しています。',
    },
    'adminer': {
      description: '軽量データベース Web 管理ツール。MySQL、PostgreSQL、SQLite などをサポート。',
    },
    'mongo-express': {
      description: 'MongoDB の Web 管理インターフェース。',
    },
    'caddy': {
      description: 'シンプルな Web サービスとリバースプロキシ起動テンプレート。デフォルトで静的レスポンスを返します。',
    },
    'gitea': {
      description: '軽量セルフホスト Git サービス。リポジトリ、Issue、Release 管理に適しています。',
    },
    'vaultwarden': {
      description: 'Bitwarden クライアント互換の軽量セルフホストパスワードマネージャー。',
    },
    'nocodb': {
      description: 'オープンソースノーコードデータベースインターフェース。小規模内部ツールとデータアプリに適しています。',
    },
    'wiki-js': {
      description: 'モダンなセルフホスト Wiki。チームドキュメントとナレッジベースに適しています。',
    },
    'wordpress': {
      description: '一般的なセルフホスト CMS とブログプラットフォーム。既存の MySQL 互換データベースへの接続が必要です。',
    },
  },
}

export default appTemplatesPage
