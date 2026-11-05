const nav = {
  workbench: '워크벤치',
  integrations: '연동',
  systemManagement: '시스템 관리',
  openSidebar: '사이드바 열기',
  toggleSidebar: '사이드바 접기/펼치기',
  breadcrumb: '탐색 경로',
  projectNavigation: '프로젝트 공간 탐색',
  switchProject: '프로젝트 공간 전환',
  allProjectSpaces: '모든 프로젝트 공간',
  projectSections: {
    apps: '애플리케이션',
    overview: '개요',
    buildVariables: '빌드 변수',
    runtimeConfigs: '설정',
    hooks: '훅',
    volumes: '데이터 볼륨',
    members: '멤버',
    topology: '서비스 관계',
  },
  commandPalette: {
    trigger: '검색…',
    shortcutMac: '⌘K',
    shortcutOther: 'Ctrl K',
    title: '명령 팔레트',
    description: '페이지, 프로젝트 공간, 애플리케이션을 검색하거나 자주 쓰는 작업을 실행합니다.',
    placeholder: '페이지, 프로젝트 공간, 애플리케이션 또는 작업 검색…',
    empty: '일치하는 결과가 없습니다',
    groups: {
      pages: '페이지',
      projects: '프로젝트 공간',
      applications: '{{project}}의 애플리케이션',
      actions: '자주 쓰는 작업',
    },
    actions: {
      createProject: '프로젝트 공간 만들기',
      toggleTheme: '라이트/다크 테마 전환',
    },
  },
}

export default nav
