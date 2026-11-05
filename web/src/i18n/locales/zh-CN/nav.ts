const nav = {
  workbench: '工作台',
  integrations: '集成',
  systemManagement: '系统管理',
  openSidebar: '打开侧边栏',
  toggleSidebar: '折叠或展开侧边栏',
  breadcrumb: '面包屑导航',
  projectNavigation: '项目空间导航',
  switchProject: '切换项目空间',
  allProjectSpaces: '全部项目空间',
  projectSections: {
    apps: '应用',
    overview: '概览',
    buildVariables: '构建变量',
    runtimeConfigs: '配置',
    hooks: '钩子',
    volumes: '数据卷',
    members: '成员',
    topology: '服务关系',
  },
  commandPalette: {
    trigger: '搜索…',
    shortcutMac: '⌘K',
    shortcutOther: 'Ctrl K',
    title: '命令面板',
    description: '搜索页面、项目空间和应用，或执行常用操作。',
    placeholder: '搜索页面、项目空间、应用或操作…',
    empty: '没有匹配的结果',
    groups: {
      pages: '页面',
      projects: '项目空间',
      applications: '{{project}} 的应用',
      actions: '常用操作',
    },
    actions: {
      createProject: '新建项目空间',
      toggleTheme: '切换明暗主题',
    },
  },
}

export default nav
