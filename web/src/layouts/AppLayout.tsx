import type { AIAssistantView } from '@/components/common/ai-assistant/route-state'
import type { ProjectWorkspaceSection } from '@/pages/projects/overview/project-workspace-sections'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Bell, ChartNoAxesCombined, Check, ChevronsUpDown, Container, Fingerprint, FolderKanban, GitBranch, LayoutDashboard, PanelLeft, ScrollText, Server, Settings, Sparkles, Store, Users } from 'lucide-react'
import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useDocumentTitle } from '@/app/document-title'
import { usePublicConfig } from '@/app/public-config-context'
import { useSession } from '@/app/session-context'
import { AccountMenu } from '@/components/common/account-menu'
import { AIAssistantDesktopHost } from '@/components/common/ai-assistant/desktop-host'
import { useAIAssistantPresentationMode } from '@/components/common/ai-assistant/presentation-mode'
import {
  AI_ASSISTANT_ROUTE_PATH,
  createAIAssistantRouteState,
  isAIAssistantRoutePath,
} from '@/components/common/ai-assistant/route-state'
import { useAIAssistantRuntime } from '@/components/common/ai-assistant/runtime-context'
import { AIAssistantRuntimeProvider } from '@/components/common/ai-assistant/runtime-provider'
import { CommandPalette } from '@/components/common/command-palette'
import { InboxTrigger } from '@/components/common/inbox/inbox-trigger'
import { AppLoadingState } from '@/components/common/loading-states'
import { PageMotion } from '@/components/common/motion'
import { WorkspaceChromeTargetsProvider } from '@/components/common/workspace-chrome-context'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'
import { AIAssistantPage } from '@/pages/ai-assistant/AIAssistantPage'
import { normalizeProjectWorkspaceSection, projectWorkspaceSectionPath, projectWorkspaceSections } from '@/pages/projects/overview/project-workspace-sections'

interface Breadcrumb {
  label: string
  to?: string
}

interface NavItem {
  to: string
  labelKey: string
  icon: typeof LayoutDashboard
  permission?: string
  activeMatch?: (pathname: string) => boolean
}

interface NavSection {
  titleKey: string
  items: NavItem[]
}

// 全局导航按工作流分组：日常工作、接入的外部资源（代码源、镜像站、运行集群）、平台管理。
// 账号与账单收在头像菜单。
const navSections: NavSection[] = [
  {
    titleKey: 'nav.workbench',
    items: [
      { to: '/dashboard', labelKey: 'dashboard', icon: LayoutDashboard },
      { to: '/projects', labelKey: 'projects', icon: FolderKanban, activeMatch: pathname => pathname === '/projects' || pathname.startsWith('/projects/') },
      { to: '/app-templates', labelKey: 'appTemplates', icon: Store },
      { to: '/events', labelKey: 'events', icon: ScrollText },
    ],
  },
  {
    titleKey: 'nav.integrations',
    items: [
      { to: '/code-repositories', labelKey: 'codeRepositories', icon: GitBranch },
      { to: '/registries', labelKey: 'registries', icon: Container },
      { to: '/clusters', labelKey: 'clusters', icon: Server },
    ],
  },
  {
    titleKey: 'nav.systemManagement',
    items: [
      { to: '/settings/auth-providers', labelKey: 'authProviders', icon: Fingerprint, permission: 'user.manage' },
      { to: '/settings/users', labelKey: 'users', icon: Users, permission: 'user.manage' },
      { to: '/settings/notifications', labelKey: 'notifications', icon: Bell, permission: 'user.manage' },
      { to: '/settings/operations', labelKey: 'operationsDashboard', icon: ChartNoAxesCombined, permission: 'user.manage' },
      { to: '/settings/site', labelKey: 'siteSettings', icon: Settings, permission: 'user.manage' },
    ],
  },
]

const pageMetaRules = [
  { match: (pathname: string) => pathname === '/dashboard', titleKey: 'dashboard' },
  { match: (pathname: string) => isAIAssistantRoutePath(pathname), titleKey: 'aiAssistant.title' },
  { match: (pathname: string) => /^\/projects\/[^/]+\/apps\/[^/]+(?:\/deployments\/(?:new|[^/]+\/edit))?$/.test(pathname), titleKey: 'apps.detailTitle' },
  { match: (pathname: string) => /^\/projects\/[^/]+$/.test(pathname), titleKey: 'projectSpaces.workspaceTitle' },
  { match: (pathname: string) => pathname === '/projects', titleKey: 'projectSpaces.title' },
  { match: (pathname: string) => pathname === '/events', titleKey: 'eventsPage.title' },
  { match: (pathname: string) => pathname === '/inbox', titleKey: 'inbox.title' },
  { match: (pathname: string) => pathname === '/app-templates', titleKey: 'appTemplates' },
  { match: (pathname: string) => pathname === '/code-repositories', titleKey: 'codeRepositories' },
  { match: (pathname: string) => pathname === '/registries', titleKey: 'registries' },
  { match: (pathname: string) => pathname === '/clusters', titleKey: 'clusters' },
  { match: (pathname: string) => pathname === '/billing', titleKey: 'billing' },
  { match: (pathname: string) => pathname === '/settings/account', titleKey: 'account' },
  { match: (pathname: string) => pathname === '/settings/auth-providers', titleKey: 'authProvidersPage.title' },
  { match: (pathname: string) => pathname === '/settings/notifications', titleKey: 'notificationsPage.title' },
  { match: (pathname: string) => pathname === '/settings/operations', titleKey: 'operationsDashboard' },
  { match: (pathname: string) => pathname === '/settings/users', titleKey: 'usersPage.title' },
  { match: (pathname: string) => pathname === '/settings/site', titleKey: 'siteSettings' },
]

function sidebarMenuButtonClassName(active?: boolean) {
  return cn(
    'flex h-10 w-full min-w-0 max-w-full items-center gap-3 overflow-hidden rounded-control px-3 text-sm font-normal leading-none text-foreground transition-colors duration-fast hover:bg-sidebar-nav-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&>svg]:size-[18px]',
    active && 'font-medium [background:var(--sidebar-nav-active)] text-sidebar-nav-active-text hover:[background:var(--sidebar-nav-active)] hover:text-sidebar-nav-active-text',
  )
}

function useChromeSlotPresence() {
  const [registrations, setRegistrations] = useState(0)
  const register = useCallback(() => {
    let active = true
    setRegistrations(current => current + 1)

    return () => {
      if (!active)
        return
      active = false
      setRegistrations(current => Math.max(0, current - 1))
    }
  }, [])

  return [registrations > 0, register] as const
}

export function AppLayout() {
  const { i18n, t } = useTranslation()
  const { isLoading: sessionLoading, isLoggingOut, logout, user } = useSession()
  const configs = usePublicConfig()
  const location = useLocation()
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)
  const [desktopSidebarCollapsed, setDesktopSidebarCollapsed] = useState(false)
  const [headingTarget, setHeadingTarget] = useState<HTMLDivElement | null>(null)
  const [descriptionTarget, setDescriptionTarget] = useState<HTMLDivElement | null>(null)
  const [tabsTarget, setTabsTarget] = useState<HTMLDivElement | null>(null)
  const [toolsTarget, setToolsTarget] = useState<HTMLDivElement | null>(null)
  const [hasCustomHeading, registerHeading] = useChromeSlotPresence()
  const [, registerTabs] = useChromeSlotPresence()
  const [, registerTools] = useChromeSlotPresence()
  const projects = useQuery({ queryKey: ['projects'], queryFn: () => api.listProjects(), enabled: Boolean(user) })
  // 布局层是 AI 访问状态的唯一查询者。Agent 短暂不可达时仍保留入口和已打开窗口，
  // 只有平台关闭助手或当前用户不在允许范围时才不挂载。
  const aiCapabilities = useQuery({
    queryKey: ['ai', 'capabilities'],
    queryFn: api.getAICapabilities,
    retry: 2,
    staleTime: 30_000,
    refetchInterval: 30_000,
    enabled: Boolean(user),
  })
  const enabledAICapabilities = aiCapabilities.data?.enabled ? aiCapabilities.data : undefined
  const assistantRoute = isAIAssistantRoutePath(location.pathname)
  const projectRouteMatch = location.pathname.match(/^\/projects\/([^/]+)/)
  const appRouteMatch = location.pathname.match(/^\/projects\/([^/]+)\/apps\/([^/]+)(\/deployments\/(?:new|[^/]+\/edit))?$/)
  const currentProject = useQuery({
    queryKey: ['project', projectRouteMatch?.[1]],
    queryFn: () => api.getProject(projectRouteMatch?.[1] ?? ''),
    enabled: Boolean(user && projectRouteMatch),
  })
  const currentApplication = useQuery({
    queryKey: ['application', appRouteMatch?.[1], appRouteMatch?.[2]],
    queryFn: () => api.getApplication(appRouteMatch?.[1] ?? '', appRouteMatch?.[2] ?? ''),
    enabled: Boolean(user && appRouteMatch),
  })
  const project = currentProject.data ?? (projectRouteMatch ? projects.data?.find(item => item.id === projectRouteMatch[1]) : undefined)
  const activeProjectSection: ProjectWorkspaceSection = appRouteMatch
    ? 'apps'
    : normalizeProjectWorkspaceSection(new URLSearchParams(location.search).get('tab'))
  const pageMeta = useMemo(() => {
    const rule = pageMetaRules.find(item => item.match(location.pathname))
    const projectWorkspaceMatch = /^\/projects\/[^/]+$/.test(location.pathname)
    const application = appRouteMatch ? currentApplication.data : undefined
    let documentTitle = rule?.titleKey ? t(rule.titleKey) : configs['site.title'] || t('appName')
    let heading = documentTitle
    const breadcrumbs: Breadcrumb[] = []
    if (projectRouteMatch) {
      breadcrumbs.push({ label: t('projectSpaces.title'), to: '/projects' })
      if (project)
        breadcrumbs.push({ label: project.name, to: `/projects/${project.id}` })
    }
    if (projectWorkspaceMatch && project) {
      documentTitle = t('projectSpaces.detailTopbarTitle', { name: project.name })
      heading = project.name
    }
    if (application) {
      documentTitle = t('apps.detailTopbarTitle', { name: application.name, projectName: project?.name ?? t('projectSpaces.title') })
      heading = application.name
      breadcrumbs.push({ label: application.name, to: `/projects/${application.projectId}/apps/${application.id}` })
    }
    if (breadcrumbs.length === 0)
      breadcrumbs.push({ label: heading })
    return { breadcrumbs, documentTitle, heading }
  }, [appRouteMatch, configs, currentApplication.data, location.pathname, project, projectRouteMatch, t])
  useDocumentTitle(pageMeta.documentTitle)
  const workspaceChromeTargets = useMemo(() => ({
    description: descriptionTarget,
    heading: headingTarget,
    registerHeading,
    registerTabs,
    registerTools,
    tabs: tabsTarget,
    tools: toolsTarget,
  }), [descriptionTarget, headingTarget, registerHeading, registerTabs, registerTools, tabsTarget, toolsTarget])
  const pageMotionKey = /^\/projects\/[^/]+$/.test(location.pathname) ? '/projects/:projectId' : location.pathname
  const handleLogout = async () => {
    try {
      await logout()
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('errors.request.failed'))
    }
  }

  const renderGlobalNavigation = (onNavigate?: () => void) => {
    if (!user)
      return null

    return navSections.map((section, index) => {
      const items = section.items.filter(item => !item.permission || user.permissions.includes(item.permission))
      if (items.length === 0)
        return null

      return (
        <SidebarGroup key={section.titleKey} className={index > 0 ? 'mt-4' : undefined}>
          <SidebarGroupLabel>{t(section.titleKey)}</SidebarGroupLabel>
          <SidebarMenu>
            {items.map(item => (
              <SidebarMenuItem key={item.to}>
                <NavLink
                  className={({ isActive }) => sidebarMenuButtonClassName(isActive || item.activeMatch?.(location.pathname))}
                  title={t(item.labelKey)}
                  to={item.to}
                  onClick={onNavigate}
                >
                  <item.icon className="shrink-0" />
                  <span className="min-w-0 flex-1 truncate text-sm leading-none">{t(item.labelKey)}</span>
                </NavLink>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroup>
      )
    })
  }

  const renderProjectNavigation = (projectId: string, onNavigate?: () => void) => (
    <>
      <Link
        className="mb-2 inline-flex h-8 w-full items-center gap-2 rounded-control px-3 text-sm text-muted-foreground transition-colors duration-fast hover:bg-sidebar-nav-hover hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        to="/projects"
        onClick={onNavigate}
      >
        <ArrowLeft aria-hidden="true" className="size-4 shrink-0" />
        <span className="truncate">{t('nav.allProjectSpaces')}</span>
      </Link>
      <ProjectSwitcher
        currentProjectId={projectId}
        currentProjectName={project?.name}
        projects={projects.data ?? []}
        onNavigate={onNavigate}
      />
      <SidebarGroup className="mt-3">
        <SidebarMenu aria-label={t('nav.projectNavigation')}>
          {projectWorkspaceSections.map(section => (
            <SidebarMenuItem key={section.value}>
              <Link
                aria-current={activeProjectSection === section.value ? 'page' : undefined}
                className={sidebarMenuButtonClassName(activeProjectSection === section.value)}
                title={t(section.labelKey)}
                to={projectWorkspaceSectionPath(projectId, section.value)}
                onClick={onNavigate}
              >
                <section.icon className="shrink-0" />
                <span className="min-w-0 flex-1 truncate text-sm leading-none">{t(section.labelKey)}</span>
              </Link>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroup>
    </>
  )

  const renderSidebarContent = (onNavigate?: () => void) => {
    if (!user)
      return null

    return (
      <>
        <SidebarHeader className="p-2">
          <Link
            aria-label={configs['site.title'] || t('appName')}
            className="flex h-12 min-w-0 items-center gap-2 rounded-control px-2 outline-none transition-colors hover:bg-sidebar-nav-hover focus-visible:ring-2 focus-visible:ring-ring"
            to="/dashboard"
            onClick={onNavigate}
          >
            <img
              alt=""
              className="size-6 shrink-0 rounded-control object-contain"
              src={configs['site.logoUrl'] || '/luna-devops-logo.svg'}
            />
            <span className="min-w-0 truncate text-base font-semibold">{configs['site.title'] || t('appName')}</span>
          </Link>
        </SidebarHeader>
        <SidebarContent className="px-2 py-2">
          {projectRouteMatch
            ? renderProjectNavigation(projectRouteMatch[1], onNavigate)
            : renderGlobalNavigation(onNavigate)}
        </SidebarContent>
        <SidebarFooter className="border-t-0 p-2">
          <AccountMenu
            logoutPending={isLoggingOut}
            user={user}
            variant="sidebar"
            onLogout={handleLogout}
          />
        </SidebarFooter>
      </>
    )
  }

  useEffect(() => {
    if (user?.language && i18n.language !== user.language)
      i18n.changeLanguage(user.language)
  }, [i18n, user?.language])

  if (sessionLoading) {
    return <AppLoadingState logoUrl={configs['site.logoUrl'] || '/luna-devops-logo.svg'} title={configs['site.title'] || t('appName')} />
  }

  if (!user) {
    const redirect = `${location.pathname}${location.search}`
    return <Navigate to={`/login?redirect=${encodeURIComponent(redirect)}`} replace />
  }

  return (
    <AIAssistantRuntimeProvider capabilities={enabledAICapabilities}>
      {assistantRoute
        ? (
            <div className="h-dvh min-h-0 overflow-hidden bg-surface text-foreground">
              <main className="size-full min-h-0 overflow-hidden">
                {aiCapabilities.isPending
                  ? <AppLoadingState logoUrl={configs['site.logoUrl'] || '/luna-devops-logo.svg'} title={t('aiAssistant.title')} />
                  : <AIAssistantPage />}
              </main>
            </div>
          )
        : (
            <div className="workspace-canvas flex h-dvh overflow-hidden text-foreground">
              {!desktopSidebarCollapsed && (
                <Sidebar className="h-full w-72 min-w-72 max-w-72 bg-transparent">
                  {renderSidebarContent()}
                </Sidebar>
              )}
              <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
                <SheetContent className="workspace-canvas flex h-full w-72 max-w-[86vw] flex-col gap-0 overflow-hidden p-0 sm:max-w-80" side="left">
                  <SheetTitle className="sr-only">{configs['site.title'] || t('appName')}</SheetTitle>
                  {renderSidebarContent(() => setMobileSidebarOpen(false))}
                </SheetContent>
              </Sheet>

              <WorkspaceChromeTargetsProvider value={workspaceChromeTargets}>
                <div className={cn('flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-background lg:my-2 lg:mr-2 lg:rounded-container lg:shadow-sm', desktopSidebarCollapsed && 'lg:ml-2')}>
                  <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4">
                    <Button
                      aria-label={t('nav.openSidebar')}
                      className="-ml-1 size-8 shrink-0 lg:hidden"
                      size="icon"
                      variant="ghost"
                      onClick={() => setMobileSidebarOpen(true)}
                    >
                      <PanelLeft className="size-4" />
                    </Button>
                    <Button
                      aria-label={t('nav.toggleSidebar')}
                      aria-pressed={!desktopSidebarCollapsed}
                      className="-ml-1 hidden size-8 shrink-0 lg:inline-flex"
                      size="icon"
                      variant="ghost"
                      onClick={() => setDesktopSidebarCollapsed(collapsed => !collapsed)}
                    >
                      <PanelLeft className="size-4" />
                    </Button>
                    <span aria-hidden="true" className="mx-1 h-4 w-px shrink-0 bg-border" />
                    <Breadcrumbs items={pageMeta.breadcrumbs} label={t('nav.breadcrumb')} />
                    <div ref={setHeadingTarget} className="min-w-0 flex-1" data-slot="workspace-page-heading">
                      {!hasCustomHeading && (
                        <h1 className="truncate text-base font-medium text-foreground">{pageMeta.heading}</h1>
                      )}
                    </div>
                    <CommandPalette />
                    <InboxTrigger />
                    <AIAssistantTopbarButton />
                  </header>
                  <main className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden px-page-inline py-page-block">
                    <div className="flex min-h-full min-w-0 flex-col gap-group">
                      {/* 左侧依次是 Tabs 与页面描述，右侧是页面级操作；三者都为空时整行隐藏 */}
                      <div className="flex min-w-0 flex-wrap items-center justify-between gap-x-4 gap-y-3 has-[[data-slot=workspace-page-tabs]:empty]:has-[[data-slot=workspace-page-description]:empty]:has-[[data-slot=workspace-page-tools]:empty]:hidden" data-slot="workspace-page-header">
                        <div className="grid min-w-0 flex-1 gap-2" data-slot="workspace-page-intro">
                          <div ref={setTabsTarget} className="min-w-0 empty:hidden" data-slot="workspace-page-tabs" />
                          <div ref={setDescriptionTarget} className="min-w-0 empty:hidden" data-slot="workspace-page-description" />
                        </div>
                        <div ref={setToolsTarget} className="ml-auto flex min-w-0 max-w-full items-center justify-end empty:hidden" data-slot="workspace-page-tools" />
                      </div>
                      <PageMotion key={pageMotionKey} className="w-full min-w-0 max-w-full">
                        <Outlet />
                      </PageMotion>
                    </div>
                  </main>
                </div>
              </WorkspaceChromeTargetsProvider>
              <AIAssistantEntryHost />
            </div>
          )}
    </AIAssistantRuntimeProvider>
  )
}

function AIAssistantEntryHost() {
  const location = useLocation()
  const navigate = useNavigate()
  const runtime = useAIAssistantRuntime()
  const { enabled, open, rememberWorkspaceLocation, transitionAssistantToPage } = runtime
  const presentationMode = useAIAssistantPresentationMode(location.pathname)
  const [assistantView, setAssistantView] = useState<AIAssistantView>('chat')

  useLayoutEffect(() => {
    if (!enabled || presentationMode !== 'page' || !open)
      return

    rememberWorkspaceLocation(location)
    transitionAssistantToPage()
    navigate(AI_ASSISTANT_ROUTE_PATH, {
      state: createAIAssistantRouteState(location, assistantView),
    })
  }, [assistantView, enabled, location, navigate, open, presentationMode, rememberWorkspaceLocation, transitionAssistantToPage])

  if (!runtime.enabled)
    return null

  if (presentationMode === 'window') {
    return (
      <AIAssistantDesktopHost
        view={assistantView}
        onExpand={() => {
          rememberWorkspaceLocation(location)
          transitionAssistantToPage()
          navigate(AI_ASSISTANT_ROUTE_PATH, {
            state: createAIAssistantRouteState(location),
          })
        }}
        onViewChange={setAssistantView}
      />
    )
  }

  if (runtime.open)
    return <div aria-hidden="true" className="fixed inset-0 z-50 bg-surface" data-ai-assistant-transition />

  return null
}

/** 顶栏右上角的助手入口：桌面窗口模式切换右侧停靠面板，页面模式进入全页助手。 */
function AIAssistantTopbarButton() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const runtime = useAIAssistantRuntime()
  const presentationMode = useAIAssistantPresentationMode(location.pathname)
  if (!runtime.enabled)
    return null

  const open = presentationMode === 'window' && runtime.open
  return (
    <Button
      aria-pressed={open}
      className={cn('h-8 shrink-0 gap-1.5 px-2.5', open && 'border-primary-border bg-primary-subtle text-primary-text hover:bg-primary-subtle-hover')}
      data-ai-assistant-trigger
      size="sm"
      title={t('aiAssistant.open')}
      variant="outline"
      onClick={() => {
        if (presentationMode === 'window') {
          if (runtime.open)
            runtime.closeAssistant()
          else
            runtime.openAssistant()
          return
        }
        runtime.rememberWorkspaceLocation(location)
        navigate(AI_ASSISTANT_ROUTE_PATH, {
          state: createAIAssistantRouteState(location),
        })
      }}
    >
      <Sparkles aria-hidden="true" className="size-4 text-primary-text" />
      <span className="max-sm:sr-only">{t('aiAssistantNav')}</span>
    </Button>
  )
}

/** 面板顶栏里标题前的上级层级（如 项目空间 / xxx /），当前页面由 h1 表达；窄屏隐藏。 */
function Breadcrumbs({ items, label }: { items: Breadcrumb[], label: string }) {
  const ancestors = items.slice(0, -1).filter(item => item.to)
  if (ancestors.length === 0)
    return null

  return (
    <nav aria-label={label} className="hidden min-w-0 shrink md:block">
      <ol className="flex min-w-0 items-center gap-1.5 text-sm text-muted-foreground">
        {ancestors.map(item => (
          <li key={`${item.label}-${item.to}`} className="flex min-w-0 items-center gap-1.5">
            <Link className="min-w-0 max-w-48 truncate rounded-sm outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring" title={item.label} to={item.to ?? '/'}>{item.label}</Link>
            <span aria-hidden="true" className="text-muted-foreground/60">/</span>
          </li>
        ))}
      </ol>
    </nav>
  )
}

function ProjectSwitcher({ currentProjectId, currentProjectName, onNavigate, projects }: {
  currentProjectId: string
  currentProjectName?: string
  onNavigate?: () => void
  projects: { id: string, name: string }[]
}) {
  const { t } = useTranslation()
  const name = currentProjectName ?? projects.find(item => item.id === currentProjectId)?.name ?? ''

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          aria-label={t('nav.switchProject')}
          className="flex h-10 w-full min-w-0 items-center gap-2 rounded-control border border-border bg-surface px-3 text-left text-sm outline-none transition-colors duration-fast hover:bg-sidebar-nav-hover focus-visible:ring-2 focus-visible:ring-ring"
          type="button"
        >
          <span aria-hidden="true" className="grid size-6 shrink-0 place-items-center rounded-control bg-muted text-xs font-semibold text-foreground">
            {name.slice(0, 1).toUpperCase()}
          </span>
          <span className="min-w-0 flex-1 truncate font-medium">{name}</span>
          <ChevronsUpDown aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-80 w-(--radix-dropdown-menu-trigger-width) min-w-56 overflow-y-auto">
        <DropdownMenuLabel>{t('projectSpaces.title')}</DropdownMenuLabel>
        {projects.map(item => (
          <DropdownMenuItem key={item.id} asChild>
            <Link to={`/projects/${item.id}`} onClick={onNavigate}>
              <span className="min-w-0 flex-1 truncate">{item.name}</span>
              {item.id === currentProjectId && <Check aria-hidden="true" />}
            </Link>
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link to="/projects" onClick={onNavigate}>{t('nav.allProjectSpaces')}</Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
