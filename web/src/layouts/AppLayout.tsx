import { useQuery } from '@tanstack/react-query'
import { Bell, ChartNoAxesCombined, CircleUserRound, Container, CreditCard, Fingerprint, FolderKanban, GitBranch, LayoutDashboard, Menu, ScrollText, Server, Settings, Store, Users } from 'lucide-react'
import { AnimatePresence } from 'motion/react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, NavLink, Outlet, useLocation } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useDocumentTitle } from '@/app/document-title'
import { usePublicConfig } from '@/app/public-config-context'
import { useSession } from '@/app/session-context'
import { DebugFloatingPanel } from '@/components/common/debug-floating-panel'
import { AppLoadingState } from '@/components/common/loading-states'
import { PageMotion } from '@/components/common/motion'
import { PageChrome } from '@/components/common/page-chrome'
import { PageChromeTargetsProvider } from '@/components/common/page-chrome-context'
import { SidebarUserPanel } from '@/components/common/sidebar-user-panel'
import { Button } from '@/components/ui/button'
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

export function AppLayout() {
  const { i18n, t } = useTranslation()
  const { isLoading: sessionLoading, isLoggingOut, logout, user } = useSession()
  const configs = usePublicConfig()
  const location = useLocation()
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)
  const [pageTabsTarget, setPageTabsTarget] = useState<HTMLDivElement | null>(null)
  const [pageToolsTarget, setPageToolsTarget] = useState<HTMLDivElement | null>(null)
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects, enabled: Boolean(user) })
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
  const pageMotionKey = /^\/projects\/[^/]+$/.test(location.pathname) ? '/projects/:projectId' : location.pathname
  const handleLogout = () => {
    logout().catch(error => toast.error(error.message))
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
                        <item.icon className="size-4 shrink-0" />
                        <span className="min-w-0 flex-1 truncate text-sm leading-none">{t(item.labelKey)}</span>
                      </NavLink>
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              </SidebarGroup>
            )
          })}
        </SidebarContent>
        <SidebarFooter>
          <SidebarUserPanel
            logoutLabel={t('logout')}
            logoutPending={isLoggingOut}
            user={user}
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
          <header
            className="z-10 flex h-14 shrink-0 items-center justify-between bg-surface-base/80 px-page-inline py-inline backdrop-blur lg:hidden"
          >
            <Button
              aria-label={t('nav.openSidebar')}
              className="mr-2 shrink-0 lg:hidden"
              size="icon"
              variant="ghost"
              onClick={() => setMobileSidebarOpen(true)}
            >
              <Menu className="size-5" />
            </Button>
            <div className="min-w-0 flex-1">
              <TopbarTitle crumbs={pageMeta.titleCrumbs} prefix={pageMeta.titlePrefix} title={pageMeta.title} />
            </div>
          </header>
          <main
            className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden bg-transparent px-page-inline py-page-block transition-colors"
          >
            <div className="flex min-h-full min-w-0 flex-col gap-group py-0">
              <PageChrome
                backNavigation={pageMeta.backNavigation}
                tabsTargetRef={setPageTabsTarget}
                title={(
                  <TopbarTitle crumbs={pageMeta.titleCrumbs} prefix={pageMeta.titlePrefix} title={pageMeta.title} />
                )}
                toolsTargetRef={setPageToolsTarget}
              />
              <PageChromeTargetsProvider value={{ tabs: pageTabsTarget, tools: pageToolsTarget }}>
                <AnimatePresence mode="wait">
                  <PageMotion key={pageMotionKey}>
                    <Outlet />
                  </PageMotion>
                </AnimatePresence>
              </PageChromeTargetsProvider>
            </div>
          </main>
        </div>
      </div>
      <DebugFloatingPanel />
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
