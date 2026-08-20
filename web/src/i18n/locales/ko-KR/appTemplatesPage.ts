const appTemplatesPage = {
  description: '프리셋 템플릿에서 데이터베이스, 캐시, 모니터링, 도구, 경량 협업 앱을 원클릭으로 설치합니다.',
  heroEyebrow: 'Luna DevOps 템플릿 라이브러리',
  heroTitle: '서비스를 발견하고 더 빠르게 배포 시작',
  templateCount: '사용 가능한 템플릿',
  categoryCount: '서비스 카테고리',
  browseTitle: '모든 템플릿 찾아보기',
  resultCount: '{{count}}개의 사용 가능한 템플릿을 찾았습니다',
  searchPlaceholder: '앱, 용도, 이미지 검색',
  categoryFilter: '카테고리 필터',
  allCategories: '모든 카테고리',
  sortBy: '정렬 필드',
  sortByPopularity: '인기',
  sortByName: '이름 A-Z',
  sortOrder: '정렬 순서',
  sortDesc: '내림차순',
  sortAsc: '오름차순',
  loading: '앱 템플릿 로드 중...',
  emptyTitle: '템플릿을 찾을 수 없습니다',
  emptyDescription: '다른 키워드를 시도하거나 관리자가 더 많은 템플릿을 추가할 때까지 기다리세요.',
  install: '설치',
  installing: '설치 중...',
  installStarted: '앱 템플릿 설치가 시작되었습니다',
  systemInstallStarted: '플랫폼 컴포넌트 설치가 시작되었습니다',
  installDialogTitle: '{{name}} 설치',
  installDialogDescription: '템플릿은 대상 프로젝트 스페이스에 애플리케이션과 배포 구성을 생성합니다. 시크릿 계열 파라미터는 안전하게 저장되며 평문으로 다시 표시되지 않습니다.',
  systemInstallDialogDescription: '플랫폼 컴포넌트는 지정한 실행 클러스터의 시스템 네임스페이스에 설치되어 플랫폼 기능을 향상시킵니다. 프로젝트 스페이스 애플리케이션은 생성되지 않습니다.',
  platformComponent: '플랫폼 컴포넌트',
  selectRuntimeCluster: '실행 클러스터를 선택하세요',
  componentNamespace: '컴포넌트 네임스페이스',
  systemInstallAdminOnly: '플랫폼 관리자만 플랫폼 컴포넌트를 설치할 수 있습니다.',
  runtimeCluster: '실행 클러스터',
  defaultCluster: '기본 클러스터 사용',
  applicationName: '애플리케이션 이름',
  applicationIdentifier: '애플리케이션 식별자',
  deploymentName: '배포 구성',
  stage: '단계',
  imageRef: '이미지 주소',
  imageRefHint: '기본적으로 템플릿 이미지를 사용합니다. Harbor, DockerHub 프록시 또는 프라이빗 이미지가 있는 경우 자신의 전체 이미지 주소로 변경할 수 있습니다.',
  provisionAccess: '플랫폼에서 ServiceAccount 및 RBAC 권한 생성',
  provisionAccessDescription: '활성화하면 플랫폼이 전용 ServiceAccount를 생성하고 Gateway API 경로에 대한 읽기 권한을 부여합니다. 비활성화하면 네임스페이스 기본 계정으로 실행되며 RBAC는 직접 관리합니다.',
  replicas: '레플리카 수',
  cpu: 'CPU',
  memory: '메모리',
  projectVolume: '프로젝트 데이터 볼륨',
  selectProjectVolume: '준비 완료된 프로젝트 데이터 볼륨을 선택하세요',
  projectVolumeNotRequired: '이 템플릿은 영구 데이터 볼륨이 필요하지 않습니다',
  templateParameters: '템플릿 파라미터',
  templateParametersDescription: '비워 둔 자동 생성 시크릿은 백엔드에서 생성되어 시크릿 저장소에 기록됩니다.',
  autoGeneratePlaceholder: '비워 두면 자동 생성',
  installNow: '설치 후 즉시 배포',
  installNowDescription: '비활성화하면 애플리케이션과 배포 구성만 생성되고 나중에 애플리케이션 배포 페이지에서 수동으로 릴리스할 수 있습니다.',
  image: '이미지',
  officialWebsite: '공식 웹사이트',
  officialRepository: '공식 리포지토리',
  port: '포트',
  resources: '리소스',
  categories: {
    cache: '캐시',
    collaboration: '협업',
    cms: '콘텐츠 관리',
    database: '데이터베이스',
    databaseTool: '데이터베이스 도구',
    developerTool: '개발 도구',
    lowCode: '로우코드',
    middleware: '미들웨어',
    objectStorage: '객체 스토리지',
    observability: '모니터링 및 관측',
    passwordManager: '비밀번호 관리',
    registry: '아티팩트 리포지토리',
    search: '검색',
    vectorDatabase: '벡터 데이터베이스',
    webServer: '웹 서비스',
  },
  stageOptions: {
    prod: '프로덕션',
    staging: '스테이징',
    test: '테스트',
    dev: '개발',
  },
  valueLabels: {
    username: '사용자 이름',
    database: '데이터베이스 이름',
    password: '비밀번호',
    rootPassword: 'Root 비밀번호',
    rpcSecret: 'RPC 시크릿',
    adminToken: '관리 Token',
    metricsToken: '메트릭 Token',
    masterKey: '마스터 키',
    apiKey: 'API Key',
    mongodbUrl: 'MongoDB 주소',
    dbHost: '데이터베이스 주소',
    email: '이메일',
    apiBaseUrl: 'Luna DevOps API 기본 주소',
    traefikMetricsUrl: 'Traefik Metrics 주소',
  },
  valueHints: {
    apiBaseUrl: '플랫폼이 프로브에서 접근 가능한 기본 주소를 입력하세요. 예: https://luna-devops.example.com. /api/v1/billing/gateway-traffic과 같은 구체적인 인터페이스 경로를 입력하지 마세요. 프로브가 자동으로 보고 인터페이스를 붙입니다.',
    traefikMetricsUrl: '프로브 Pod가 클러스터 내에서 접근 가능한 Traefik Prometheus metrics 주소를 입력하세요. 비워 두면 기본값 http://traefik.<Gateway 네임스페이스>.svc.cluster.local:9100/metrics를 사용합니다.',
  },
  valuePlaceholders: {
    apiBaseUrl: 'https://luna-devops.example.com',
    traefikMetricsUrl: 'http://traefik.kube-system.svc.cluster.local:9100/metrics',
  },
  templates: {
    'luna-gateway-traffic-probe': {
      description: '선택적 플랫폼 컴포넌트. Gateway API 접근 트래픽 윈도우를 수집하여 청구에 보고합니다.',
    },
    'redis': {
      description: '캐시, 대기열, 경량 조정용 인메모리 데이터 저장소.',
    },
    'postgresql': {
      description: '애플리케이션 데이터, 메타데이터, 트랜잭션 시나리오에 적합한 관계형 데이터베이스.',
    },
    'mysql': {
      description: '클식 관계형 데이터베이스. 단일 컨테이너로 빠른 설치.',
    },
    'mongodb': {
      description: 'JSON 유사 데이터와 유연한 스키마를 위한 문서 데이터베이스.',
    },
    'mariadb': {
      description: 'MySQL 호환 관계형 데이터베이스. 경량 단일 컨테이너 실행에 적합합니다.',
    },
    'valkey': {
      description: 'Redis 호환 인메모리 데이터 저장소. 캐시와 경량 대기열에 적합합니다.',
    },
    'memcached': {
      description: '미니멀하고 고속인 인메모리 캐시 서비스. 단순한 key-value 캐시에 적합합니다.',
    },
    'rabbitmq': {
      description: '비동기 작업, 이벤트, 경량 메시지 대기열용 메시지 미들웨어.',
    },
    'meilisearch': {
      description: '경량 전문 검색 엔진. 앱 내 검색과 인덱싱 시나리오에 적합합니다.',
    },
    'grafana': {
      description: '메트릭 대시보드와 시각화 플랫폼. 모니터링 데이터 표시에 적합합니다.',
    },
    'prometheus': {
      description: '시계열 메트릭 데이터베이스와 스크래핑 엔진. 관측 가능성 데이터 기반으로 적합합니다.',
    },
    'uptime-kuma': {
      description: '셀프 호스트 가용성 모니터링. 사이트, API, 남부 서비스 모니터링에 사용합니다.',
    },
    'memos': {
      description: '소규모 팀용 셀프 호스트 노트와 지식 스니펫 도구.',
    },
    'it-tools': {
      description: '브라우저 내 개발 및 운영 도구 모음.',
    },
    'excalidraw': {
      description: '간단하고 사용하기 쉬운 화이트보드 도구. 그림, 스케치, 제품 설명에 적합합니다.',
    },
    'verdaccio': {
      description: '프라이빗 npm 호환 패키지 리포지토리. 남부 JavaScript 패키지 배포에 적합합니다.',
    },
    'docker-registry': {
      description: '프라이빗 OCI 이미지 리포지토리. 남부 컨테이너 이미지 배포에 적합합니다.',
    },
    'pgadmin4': {
      description: 'PostgreSQL의 웹 관리 콘솔.',
    },
    'bytebase': {
      description: '소규모 팀용 데이터베이스 변경, 리뷰, 릴리스 워크플로우.',
    },
    'garage': {
      description: '경량 S3 호환 객체 스토리지. 프로젝트 파일과 빌드 산출물 저장에 적합합니다.',
    },
    'nats': {
      description: '경량 메시지 시스템. 이벤트, 발행/구독, 서비스 간 통신에 적합합니다.',
    },
    'clickhouse': {
      description: '로그, 이벤트, 실시간 분석을 위한 컬럼형 데이터베이스.',
    },
    'qdrant': {
      description: '벡터 데이터베이스. 시맨틱 검색, 추천, AI 검색 시나리오에 적합합니다.',
    },
    'typesense': {
      description: '오타 내성이 있는 고속 검색 엔진. 상품 검색과 앱 내 검색에 적합합니다.',
    },
    'adminer': {
      description: '경량 데이터베이스 웹 관리 도구. MySQL, PostgreSQL, SQLite 등을 지원합니다.',
    },
    'mongo-express': {
      description: 'MongoDB의 웹 관리 인터페이스.',
    },
    'caddy': {
      description: '간단한 웹 서비스와 리버스 프록시 시작 템플릿. 기본적으로 정적 응답을 반환합니다.',
    },
    'gitea': {
      description: '경량 셀프 호스트 Git 서비스. 리포지토리, Issue, Release 관리에 적합합니다.',
    },
    'vaultwarden': {
      description: 'Bitwarden 클라이언트 호환 경량 셀프 호스트 비밀번호 관리자.',
    },
    'nocodb': {
      description: '오픈 소스 노코드 데이터베이스 인터페이스. 소규모 남부 도구와 데이터 앱에 적합합니다.',
    },
    'wiki-js': {
      description: '모던한 셀프 호스트 Wiki. 팀 문서와 지식 베이스에 적합합니다.',
    },
    'wordpress': {
      description: '일반적인 셀프 호스트 CMS와 블로그 플랫폼. 기존 MySQL 호환 데이터베이스 연결이 필요합니다.',
    },
  },
}

export default appTemplatesPage
