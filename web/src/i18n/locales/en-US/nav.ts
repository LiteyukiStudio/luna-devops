const nav = {
  workbench: 'Workbench',
  integrations: 'Integrations',
  systemManagement: 'System management',
  openSidebar: 'Open sidebar',
  toggleSidebar: 'Toggle sidebar',
  breadcrumb: 'Breadcrumb',
  projectNavigation: 'Project space navigation',
  switchProject: 'Switch project space',
  allProjectSpaces: 'All project spaces',
  projectSections: {
    apps: 'Applications',
    overview: 'Overview',
    buildVariables: 'Build variables',
    runtimeConfigs: 'Config',
    hooks: 'Hooks',
    volumes: 'Volumes',
    members: 'Members',
    topology: 'Service relations',
  },
  commandPalette: {
    trigger: 'Search…',
    shortcutMac: '⌘K',
    shortcutOther: 'Ctrl K',
    title: 'Command palette',
    description: 'Search pages, project spaces, and applications, or run a common action.',
    placeholder: 'Search pages, project spaces, applications, or actions…',
    empty: 'No matching results',
    groups: {
      pages: 'Pages',
      projects: 'Project spaces',
      applications: 'Applications in {{project}}',
      actions: 'Actions',
    },
    actions: {
      createProject: 'Create project space',
      toggleTheme: 'Toggle light/dark theme',
    },
  },
}

export default nav
