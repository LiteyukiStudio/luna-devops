import { useQuery } from '@tanstack/react-query'
import { Bell, ChartNoAxesCombined, CircleUserRound, Container, CreditCard, Fingerprint, FolderKanban, GitBranch, LayoutDashboard, Menu, ScrollText, Server, Settings, Sparkles, Store, Users } from 'lucide-react'
import { lazy, useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, NavLink, Outlet, useLocation } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useDocumentTitle } from '@/app/document-title'
import { usePublicConfig } from '@/app/public-config-context'
import { useSession } from '@/app/session-context'
import { AccountMenu } from '@/components/common/account-menu'
import { DeferredAIAssistantLauncher } from '@/components/common/ai-assistant/deferred-launcher'
import { AI_ASSISTANT_OPEN_EVENT } from '@/components/common/ai-assistant/events'
import { LAUNCHER_STORAGE_KEY, readLauncherPosition } from '@/components/common/ai-assistant/layout'
import { DebugFloatingPanel } from '@/components/common/debug-floating-panel'
import { InboxTrigger } from '@/components/common/inbox/inbox-trigger'
import { LazyLoadBoundary } from '@/components/common/lazy-load-boundary'
import { AppLoadingState } from '@/components/common/loading-states'
import { PageMotion } from '@/components/common/motion'
import { PageBackNavigation } from '@/components/common/page-chrome'
import { WorkspaceChromeTargetsProvider } from '@/components/common/workspace-chrome-context'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'

const AiAssistant = lazy(async () => ({ default: (await import('@/components/common/ai-assistant/assistant')).AiAssistant }))

interface TopbarCrumb {
  label: string
  to: string
}

interface NavItem {
  to: string
  labelKey: string
  icon: typeof LayoutDashboard
  permission?: string
  activeMatch?: (pathname: string) => boolean
  /** 动作型菜单项：不跳转路由、无选中态，点击后派发对应动作 */
  action?: 'ai-assistant'
}

interface NavSection {
  titleKey: string
  items: NavItem[]
}

const navSections: NavSection[] = [
  {
    titleKey: 'nav.workbench',
    items: [
      { to: '/dashboard', labelKey: 'dashboard', icon: LayoutDashboard },
      { to: '/projects', labelKey: 'projects', icon: FolderKanban, activeMatch: pathname => pathname === '/projects' || pathname.startsWith('/projects/') },
      { to: '/events', labelKey: 'events', icon: ScrollText },
      { to: '/ai-assistant', labelKey: 'aiAssistantNav', icon: Sparkles, action: 'ai-assistant' },
    ],
  },
  {
    titleKey: 'nav.resources',
    items: [
      { to: '/code-repositories', labelKey: 'codeRepositories', icon: GitBranch },
      { to: '/registries', labelKey: 'registries', icon: Container },
      { to: '/clusters', labelKey: 'clusters', icon: Server },
      { to: '/app-templates', labelKey: 'appTemplates', icon: Store },
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
  {
    titleKey: 'nav.personal',
    items: [
      { to: '/settings/account', labelKey: 'account', icon: CircleUserRound },
      { to: '/billing', labelKey: 'billing', icon: CreditCard },
    ],
  },
]

const pageMetaRules = [
  { match: (pathname: string) => pathname === '/dashboard', titleKey: 'dashboard' },
  { match: (pathname: string) => /^\/projects\/[^/]+\/apps\/[^/]+$/.test(pathname), titleKey: 'apps.detailTitle' },
  { match: (pathname: string) => /^\/projects\/[^/]+\/members$/.test(pathname), titleKey: 'projectMembers.title' },
  { match: (pathname: string) => /^\/projects\/[^/]+\/apps$/.test(pathname), titleKey: 'apps.title' },
  { match: (pathname: string) => /^\/projects\/[^/]+$/.test(pathname), titleKey: 'projectSpaces.workspaceTitle' },
  { match: (pathname: string) => pathname === '/projects', titleKey: 'projectSpaces.title' },
  { match: (pathname: string) => pathname === '/events', titleKey: 'eventsPage.title' },
  { match: (pathname: string) => pathname === '/inbox', titleKey: 'inbox.title' },
  { match: (pathname: string) => pathname === '/app-templates', titleKey: 'appTemplates' },
  { match: (pathname: string) => pathname === '/code-repositories', titleKey: 'codeRepositories' },
  { match: (pathname: string) => pathname === '/registries', titleKey: 'registries' },
  { match: (pathname: string) => pathname === '/clusters', titleKey: 'clusters' },
  { match: (pathname: string) => pathname === '/billing', titleKey: 'billing' },
  { match: (pathname: string) => pathname === '/settings/account' || pathname === '/settings/security', titleKey: 'account' },
  { match: (pathname: string) => pathname === '/settings/auth-providers', titleKey: 'authProvidersPage.title' },
  { match: (pathname: string) => pathname === '/settings/notifications', titleKey: 'notificationsPage.title' },
  { match: (pathname: string) => pathname === '/settings/operations', titleKey: 'operationsDashboard' },
  { match: (pathname: string) => pathname === '/settings/users', titleKey: 'usersPage.title' },
  { match: (pathname: string) => pathname === '/settings/site', titleKey: 'siteSettings' },
]

function sidebarMenuButtonClassName(active?: boolean) {
  return cn(
    'flex h-10 w-full min-w-0 max-w-full items-center gap-3 overflow-hidden rounded-lg px-3 text-sm font-normal leading-none text-muted-foreground transition-all duration-150 hover:bg-sidebar-nav-hover hover:text-primary-text-strong',
    active && 'font-semibold [background:var(--sidebar-nav-active)] text-sidebar-nav-active-text hover:font-semibold hover:[background:var(--sidebar-nav-active)] hover:text-sidebar-nav-active-text',
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
  const [assistantMounted, setAssistantMounted] = useState(false)
  const [deferredAssistantPosition, setDeferredAssistantPosition] = useState(readLauncherPosition)
  const [topbarTabsTarget, setTopbarTabsTarget] = useState<HTMLDivElement | null>(null)
  const [topbarToolsTarget, setTopbarToolsTarget] = useState<HTMLDivElement | null>(null)
  const [hasTopbarTabs, registerTopbarTabs] = useChromeSlotPresence()
  const [hasTopbarTools, registerTopbarTools] = useChromeSlotPresence()
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects, enabled: Boolean(user) })
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
  useEffect(() => {
    const mountAssistant = () => setAssistantMounted(true)
    window.addEventListener(AI_ASSISTANT_OPEN_EVENT, mountAssistant)
    return () => window.removeEventListener(AI_ASSISTANT_OPEN_EVENT, mountAssistant)
  }, [])
  useEffect(() => {
    localStorage.setItem(LAUNCHER_STORAGE_KEY, JSON.stringify(deferredAssistantPosition))
  }, [deferredAssistantPosition])
  const projectRouteMatch = location.pathname.match(/^\/projects\/([^/]+)/)
  const appRouteMatch = location.pathname.match(/^\/projects\/([^/]+)\/apps\/([^/]+)$/)
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
  const pageMeta = useMemo(() => {
    const rule = pageMetaRules.find(item => item.match(location.pathname))
    const projectWorkspaceMatch = location.pathname.match(/^\/projects\/([^/]+)$/)
    const projectMembersMatch = location.pathname.match(/^\/projects\/([^/]+)\/members$/)
    const projectApplicationsMatch = location.pathname.match(/^\/projects\/([^/]+)\/apps$/)
    const project = currentProject.data ?? (projectRouteMatch ? projects.data?.find(project => project.id === projectRouteMatch[1]) : undefined)
    const application = appRouteMatch ? currentApplication.data : undefined
    let title = rule?.titleKey ? t(rule.titleKey) : configs['site.title'] || t('appName')
    let titlePrefix = ''
    const titleCrumbs: TopbarCrumb[] = []
    let backNavigation
    if (projectWorkspaceMatch && project) {
      title = t('projectSpaces.detailTopbarTitle', { name: project.name })
      titlePrefix = t('projectSpaces.topbarPrefix')
      titleCrumbs.push({ label: project.name, to: `/projects/${project.id}` })
      backNavigation = { label: t('backToProjectSpaces'), to: '/projects' }
    }
    if ((projectMembersMatch || projectApplicationsMatch) && project) {
      backNavigation = {
        label: t('backToProjectWorkspace'),
        to: `/projects/${project.id}`,
      }
    }
    if (application) {
      title = t('apps.detailTopbarTitle', { name: application.name, projectName: project?.name ?? t('projectSpaces.title') })
      titlePrefix = t('apps.applicationTopbarPrefix')
      if (project)
        titleCrumbs.push({ label: project.name, to: `/projects/${project.id}` })
      titleCrumbs.push({ label: application.name, to: `/projects/${application.projectId}/apps/${application.id}` })
      backNavigation = {
        label: t('backToApps'),
        to: `/projects/${application.projectId}?tab=apps`,
      }
    }
    return {
      backNavigation,
      title,
      titleCrumbs,
      titlePrefix,
    }
  }, [appRouteMatch, configs, currentApplication.data, currentProject.data, location.pathname, projectRouteMatch, projects.data, t])
  useDocumentTitle(pageMeta.title)
  const workspaceChromeTargets = useMemo(() => ({
    registerTabs: registerTopbarTabs,
    registerTools: registerTopbarTools,
    tabs: topbarTabsTarget,
    tools: topbarToolsTarget,
  }), [registerTopbarTabs, registerTopbarTools, topbarTabsTarget, topbarToolsTarget])
  const hasTopbarSecondaryRow = hasTopbarTabs || hasTopbarTools
  const pageMotionKey = /^\/projects\/[^/]+$/.test(location.pathname) ? '/projects/:projectId' : location.pathname
  const handleLogout = async () => {
    try {
      await logout()
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('errors.request.failed'))
    }
  }

  const renderSidebarContent = (onNavigate?: () => void) => {
    if (!user)
      return null

    return (
      <>
        <SidebarHeader className="h-18">
          <Link
            aria-label={configs['site.title'] || t('appName')}
            className="flex h-full w-full min-w-0 max-w-full items-center gap-3 overflow-hidden px-4"
            to="/projects"
          >
            <img
              alt=""
              className="size-10 shrink-0 rounded-xl object-contain"
              src={configs['site.logoUrl'] || '/luna-devops-logo.svg'}
            />
            <span className="min-w-0 flex-1 truncate font-semibold">{configs['site.title'] || t('appName')}</span>
          </Link>
        </SidebarHeader>
        <SidebarContent>
          {navSections.map((section, index) => {
            const items = section.items.filter(item => (!item.permission || user.permissions.includes(item.permission)) && (item.action !== 'ai-assistant' || enabledAICapabilities))
            if (items.length === 0)
              return null

            return (
              <SidebarGroup key={section.titleKey} className={index > 0 ? 'mt-4' : undefined}>
                <SidebarGroupLabel>{t(section.titleKey)}</SidebarGroupLabel>
                <SidebarMenu>
                  {items.map(item => (
                    <SidebarMenuItem key={item.to}>
                      {item.action
                        ? (
                            <button
                              className={sidebarMenuButtonClassName(false)}
                              title={t(item.labelKey)}
                              type="button"
                              onClick={() => {
                                onNavigate?.()
                                window.dispatchEvent(new CustomEvent(AI_ASSISTANT_OPEN_EVENT))
                              }}
                            >
                              <item.icon className="size-4 shrink-0" />
                              <span className="min-w-0 flex-1 truncate text-left text-sm leading-none">{t(item.labelKey)}</span>
                            </button>
                          )
                        : (
                            <NavLink
                              className={({ isActive }) => sidebarMenuButtonClassName(isActive || item.activeMatch?.(location.pathname))}
                              title={t(item.labelKey)}
                              to={item.to}
                              onClick={onNavigate}
                            >
                              <item.icon className="size-4 shrink-0" />
                              <span className="min-w-0 flex-1 truncate text-sm leading-none">{t(item.labelKey)}</span>
                            </NavLink>
                          )}
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              </SidebarGroup>
            )
          })}
        </SidebarContent>
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
    <div className="workspace-canvas h-dvh overflow-hidden text-foreground">
      <div className="flex h-full w-full min-w-0 overflow-hidden">
        <Sidebar>
          {renderSidebarContent()}
        </Sidebar>
        <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
          <SheetContent className="workspace-canvas flex h-full w-72 max-w-[86vw] flex-col gap-0 overflow-hidden p-0 sm:max-w-80" side="left">
            <SheetTitle className="sr-only">{configs['site.title'] || t('appName')}</SheetTitle>
            {renderSidebarContent(() => setMobileSidebarOpen(false))}
          </SheetContent>
        </Sheet>

        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <WorkspaceChromeTargetsProvider value={workspaceChromeTargets}>
            <header
              className={cn(
                'relative z-20 flex shrink-0 flex-col bg-transparent',
                hasTopbarSecondaryRow ? 'h-26 lg:h-30' : 'h-14 lg:h-18',
              )}
            >
              <div className="flex h-14 min-w-0 shrink-0 items-center gap-2 px-page-inline lg:h-18">
                <Button
                  aria-label={t('nav.openSidebar')}
                  className="shrink-0 lg:hidden"
                  size="icon"
                  variant="ghost"
                  onClick={() => setMobileSidebarOpen(true)}
                >
                  <Menu className="size-5" />
                </Button>
                <div className="min-w-0 flex-1">
                  <TopbarTitle crumbs={pageMeta.titleCrumbs} prefix={pageMeta.titlePrefix} title={pageMeta.title} />
                </div>
                <InboxTrigger />
                <AccountMenu
                  logoutPending={isLoggingOut}
                  user={user}
                  onLogout={handleLogout}
                />
              </div>
              <div className={cn('flex min-w-0 shrink-0 items-center gap-3 overflow-hidden px-page-inline', hasTopbarSecondaryRow ? 'h-12' : 'h-0')}>
                <div ref={setTopbarTabsTarget} className="min-w-0 flex-1 overflow-hidden" data-slot="workspace-topbar-tabs" />
                <div ref={setTopbarToolsTarget} className="hidden min-w-0 shrink-0 items-center justify-end empty:hidden lg:flex" data-slot="workspace-topbar-tools" />
              </div>
            </header>
            <main
              className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden bg-transparent px-page-inline py-page-block transition-colors"
            >
              <div className="flex min-h-full min-w-0 flex-col gap-group py-0">
                {pageMeta.backNavigation && <PageBackNavigation {...pageMeta.backNavigation} />}
                <PageMotion key={pageMotionKey} className="w-full min-w-0 max-w-full">
                  <Outlet />
                </PageMotion>
              </div>
            </main>
          </WorkspaceChromeTargetsProvider>
        </div>
      </div>
      <DebugFloatingPanel />
      {enabledAICapabilities && !assistantMounted && (
        <DeferredAIAssistantLauncher
          label={t('aiAssistant.open')}
          position={deferredAssistantPosition}
          onOpen={() => setAssistantMounted(true)}
          onPositionChange={setDeferredAssistantPosition}
        />
      )}
      {enabledAICapabilities && assistantMounted && (
        <LazyLoadBoundary fallback={null} resetKey="ai-assistant">
          <AiAssistant capabilities={enabledAICapabilities} initiallyOpen />
        </LazyLoadBoundary>
      )}
    </div>
  )
}

function TopbarTitle({ crumbs, prefix, title }: { crumbs: TopbarCrumb[], prefix: string, title: string }) {
  if (crumbs.length === 0)
    return <h1 className="truncate text-lg font-semibold tracking-normal md:text-xl">{title}</h1>

  return (
    <h1 className="flex min-w-0 items-center gap-1.5 text-lg font-semibold tracking-normal md:text-xl">
      <span className="shrink-0">{prefix}</span>
      {crumbs.map((crumb, index) => (
        <span key={crumb.to} className="flex min-w-0 items-center gap-1.5">
          {index > 0 && <span className="shrink-0 text-muted-foreground">/</span>}
          <Link className="min-w-0 truncate rounded-sm outline-none transition-colors hover:text-primary-text focus-visible:text-primary-text focus-visible:ring-2 focus-visible:ring-ring" title={crumb.label} to={crumb.to}>
            {crumb.label}
          </Link>
        </span>
      ))}
    </h1>
  )
}
