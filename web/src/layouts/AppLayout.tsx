import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Boxes, ChevronDown, Container, FolderKanban, GitBranch, LayoutDashboard, Link2, Pin, Server, Settings, ShieldCheck, Users } from 'lucide-react'
import { AnimatePresence } from 'motion/react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, NavLink, Outlet, useLocation } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api/client'
import { useDocumentTitle } from '@/app/document-title'
import { usePublicConfig } from '@/app/public-config-context'
import { useSession } from '@/app/session-context'
import { useTheme } from '@/app/theme-context'
import { DebugFloatingPanel } from '@/components/common/debug-floating-panel'
import { PageMotion } from '@/components/common/motion'
import { SidebarUserPanel } from '@/components/common/sidebar-user-panel'
import { ThemeModeSegmented } from '@/components/common/theme-mode-segmented'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
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

const navSections = [
  {
    titleKey: 'DevOps',
    items: [
      { to: '/code-repositories', labelKey: 'codeRepositories', icon: GitBranch },
      { to: '/registries', labelKey: 'registries', icon: Container },
      { to: '/clusters', labelKey: 'clusters', icon: Server },
    ],
  },
  {
    titleKey: 'nav.personalWorkspace',
    items: [
      { to: '/settings/account', labelKey: 'account', icon: Link2 },
    ],
  },
  {
    titleKey: 'nav.systemManagement',
    items: [
      { to: '/settings/auth-providers', labelKey: 'authProviders', icon: ShieldCheck, permission: 'user.manage' },
      { to: '/settings/users', labelKey: 'users', icon: Users, permission: 'user.manage' },
      { to: '/settings/site', labelKey: 'siteSettings', icon: Settings },
    ],
  },
]

const pageMetaRules = [
  { match: (pathname: string) => /^\/projects\/[^/]+\/apps\/[^/]+$/.test(pathname), titleKey: 'apps.detailTitle', descriptionKey: 'apps.detailDescription' },
  { match: (pathname: string) => /^\/projects\/[^/]+\/repositories$/.test(pathname), titleKey: 'repositories.title', descriptionKey: 'repositories.description' },
  { match: (pathname: string) => /^\/projects\/[^/]+\/members$/.test(pathname), titleKey: 'projectMembers.title', descriptionKey: 'projectMembers.description' },
  { match: (pathname: string) => /^\/projects\/[^/]+\/apps$/.test(pathname), titleKey: 'apps.title', descriptionKey: 'apps.description' },
  { match: (pathname: string) => /^\/projects\/[^/]+$/.test(pathname), titleKey: 'projectSpaces.workspaceTitle', descriptionKey: 'projectSpaces.workspaceDescription' },
  { match: (pathname: string) => pathname === '/projects', titleKey: 'projectSpaces.title', descriptionKey: 'projectSpaces.description' },
  { match: (pathname: string) => pathname === '/code-repositories', titleKey: 'codeRepositories', descriptionKey: 'codeRepositoriesPage.description' },
  { match: (pathname: string) => pathname === '/registries', titleKey: 'registries', descriptionKey: 'registriesPage.description' },
  { match: (pathname: string) => pathname === '/clusters', titleKey: 'clusters', descriptionKey: 'clustersPage.description' },
  { match: (pathname: string) => pathname === '/settings/account' || pathname === '/settings/security', titleKey: 'account', descriptionKey: 'accountPage.description' },
  { match: (pathname: string) => pathname === '/settings/auth-providers', titleKey: 'authProvidersPage.title', descriptionKey: 'authProvidersPage.description' },
  { match: (pathname: string) => pathname === '/settings/users', titleKey: 'usersPage.title', descriptionKey: 'usersPage.description' },
  { match: (pathname: string) => pathname === '/settings/site', titleKey: 'siteSettings', descriptionKey: 'settings.siteDescription' },
]

function sidebarMenuButtonClassName(active?: boolean) {
  return cn(
    'flex h-10 w-full min-w-0 max-w-full items-center gap-3 overflow-hidden rounded-full px-3 text-sm font-normal leading-none text-muted-foreground transition-all duration-150 hover:bg-muted hover:text-foreground',
    active && 'bg-muted text-foreground',
  )
}

function SidebarProjectItem({
  locationPathname,
  pinned,
  project,
  projectsExpanded,
  togglePending,
  onTogglePin,
}: {
  locationPathname: string
  pinned: boolean
  project: { id: string, name: string }
  projectsExpanded: boolean
  togglePending: boolean
  onTogglePin: (pinned: boolean) => void
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const applications = useQuery({
    queryKey: ['applications', project.id],
    queryFn: () => api.listApplications(project.id),
    enabled: projectsExpanded && expanded,
  })
  const projectActive = locationPathname === `/projects/${project.id}` || locationPathname.startsWith(`/projects/${project.id}/`)

  return (
    <div className="min-w-0">
      <div className="group relative min-w-0">
        <NavLink
          className={({ isActive }) => cn(sidebarMenuButtonClassName(isActive || projectActive), 'h-9 min-w-0 pr-16 pl-2 text-xs')}
          title={project.name}
          to={`/projects/${project.id}`}
        >
          <FolderKanban className="shrink-0" size={15} />
          <span className="min-w-0 flex-1 truncate">{project.name}</span>
        </NavLink>
        <Button
          aria-expanded={expanded}
          aria-label={expanded ? t('projectSpaces.collapseProjectApps') : t('projectSpaces.expandProjectApps')}
          className="absolute right-8 top-1/2 size-7 -translate-y-1/2 rounded-full text-muted-foreground opacity-0 transition-opacity hover:text-foreground group-hover:opacity-100"
          size="icon"
          type="button"
          variant="ghost"
          onClick={(event) => {
            event.preventDefault()
            event.stopPropagation()
            setExpanded(value => !value)
          }}
        >
          <ChevronDown className={cn('size-3.5 transition-transform duration-150', expanded && 'rotate-180')} />
        </Button>
        <Button
          aria-label={pinned ? t('projectSpaces.unpinProject') : t('projectSpaces.pinProject')}
          className={cn(
            'absolute right-1 top-1/2 size-7 -translate-y-1/2 rounded-full text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100',
            pinned && 'text-primary opacity-100',
          )}
          disabled={togglePending}
          size="icon"
          variant="ghost"
          onClick={(event) => {
            event.preventDefault()
            event.stopPropagation()
            onTogglePin(pinned)
          }}
        >
          <Pin className={pinned ? 'fill-current' : undefined} size={13} />
        </Button>
      </div>
      {expanded && (
        <div className="mt-1 grid gap-1">
          {(applications.data ?? []).slice(0, 6).map(application => (
            <NavLink
              key={application.id}
              className={({ isActive }) => cn(sidebarMenuButtonClassName(isActive), 'ml-5 h-8 min-w-0 max-w-[calc(100%-1.25rem)] gap-2 px-2 text-xs')}
              title={application.name}
              to={`/projects/${project.id}/apps/${application.id}`}
            >
              <Boxes className="shrink-0" size={13} />
              <span className="min-w-0 flex-1 truncate">{application.name}</span>
            </NavLink>
          ))}
          {!applications.isLoading && (applications.data ?? []).length === 0 && (
            <p className="ml-5 truncate px-2 py-1 text-xs text-muted-foreground">{t('apps.emptyTitle')}</p>
          )}
        </div>
      )}
    </div>
  )
}

export function AppLayout() {
  const { i18n, t } = useTranslation()
  const { mode, setMode } = useTheme()
  const { isLoading: sessionLoading, isLoggingOut, logout, user } = useSession()
  const configs = usePublicConfig()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [projectsExpanded, setProjectsExpanded] = useState(true)
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects, enabled: Boolean(user) })
  const projectPins = useQuery({ queryKey: ['project-pins'], queryFn: api.listProjectPins, enabled: Boolean(user) })
  const projectRouteMatch = location.pathname.match(/^\/projects\/([^/]+)/)
  const appRouteMatch = location.pathname.match(/^\/projects\/([^/]+)\/apps\/([^/]+)$/)
  const currentApplication = useQuery({
    queryKey: ['application', appRouteMatch?.[1], appRouteMatch?.[2]],
    queryFn: () => api.getApplication(appRouteMatch?.[1] ?? '', appRouteMatch?.[2] ?? ''),
    enabled: Boolean(user && appRouteMatch),
  })
  const pageMeta = useMemo(() => {
    const rule = pageMetaRules.find(item => item.match(location.pathname))
    const projectWorkspaceMatch = location.pathname.match(/^\/projects\/([^/]+)$/)
    const currentProject = projectRouteMatch ? projects.data?.find(project => project.id === projectRouteMatch[1]) : undefined
    const application = appRouteMatch ? currentApplication.data : undefined
    return {
      description: projectWorkspaceMatch && currentProject ? currentProject.description || t('common.noDescription') : rule?.descriptionKey ? t(rule.descriptionKey) : t('common.noDescription'),
      title: application
        ? t('apps.detailTopbarTitle', { name: application.name })
        : projectWorkspaceMatch && currentProject
          ? t('projectSpaces.detailTopbarTitle', { name: currentProject.name })
          : rule?.titleKey ? t(rule.titleKey) : configs['site.title'] || t('appName'),
    }
  }, [appRouteMatch, configs, currentApplication.data, location.pathname, projectRouteMatch, projects.data, t])
  useDocumentTitle(pageMeta.title)
  const pageMotionKey = /^\/projects\/[^/]+$/.test(location.pathname) ? '/projects/:projectId' : location.pathname
  const pinnedProjectIds = useMemo(() => new Set((projectPins.data ?? []).map(project => project.id)), [projectPins.data])
  const sidebarProjects = useMemo(() => {
    const pinned = projectPins.data ?? []
    const unpinned = (projects.data ?? []).filter(project => !pinnedProjectIds.has(project.id))
    return [...pinned, ...unpinned].slice(0, 10)
  }, [pinnedProjectIds, projectPins.data, projects.data])
  const toggleProjectPin = useMutation({
    mutationFn: async ({ projectId, pinned }: { projectId: string, pinned: boolean }) => {
      if (pinned)
        await api.unpinProject(projectId)
      else
        await api.pinProject(projectId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['project-pins'] })
    },
    onError: error => toast.error(error.message),
  })

  const handleLogout = () => {
    logout().catch(error => toast.error(error.message))
  }

  useEffect(() => {
    if (user?.language && i18n.language !== user.language)
      i18n.changeLanguage(user.language)
  }, [i18n, user?.language])

  if (sessionLoading) {
    return (
      <div className="grid min-h-screen place-items-center bg-background text-sm text-muted-foreground">
        {t('common.loading')}
      </div>
    )
  }

  if (!user) {
    const redirect = `${location.pathname}${location.search}`
    return <Navigate to={`/login?redirect=${encodeURIComponent(redirect)}`} replace />
  }

  return (
    <div className="h-dvh overflow-hidden bg-background text-foreground">
      <div className="flex h-full w-full min-w-0 overflow-hidden">
        <Sidebar>
          <SidebarHeader>
            <Link
              aria-label={configs['site.title'] || t('appName')}
              className="flex h-16 w-full min-w-0 max-w-full items-center gap-3 overflow-hidden px-5"
              to="/projects"
            >
              <img
                alt=""
                className="size-10 shrink-0 rounded-xl object-contain"
                src={configs['site.logoUrl'] || '/liteyuki-logo.svg'}
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
                  {index > 0 && <Separator className="mb-4" />}
                  <SidebarGroupLabel>{section.titleKey === 'DevOps' ? section.titleKey : t(section.titleKey)}</SidebarGroupLabel>
                  <SidebarMenu>
                    {section.titleKey === 'DevOps' && (
                      <SidebarMenuItem>
                        <div className={cn(sidebarMenuButtonClassName(location.pathname.startsWith('/projects')), 'gap-0 p-0')}>
                          <NavLink className="flex min-w-0 flex-1 items-center gap-3 overflow-hidden px-3 py-3" to="/projects">
                            <LayoutDashboard className="size-[17px] shrink-0" />
                            <span className="min-w-0 flex-1 truncate text-sm font-normal leading-none">{t('projects')}</span>
                          </NavLink>
                          <button
                            aria-expanded={projectsExpanded}
                            aria-label={projectsExpanded ? t('projectSpaces.collapseProjects') : t('projectSpaces.expandProjects')}
                            className="flex h-full shrink-0 items-center px-3 text-muted-foreground transition hover:text-foreground"
                            type="button"
                            onClick={() => setProjectsExpanded(value => !value)}
                          >
                            <ChevronDown className={cn('size-4 transition-transform duration-150', projectsExpanded && 'rotate-180')} />
                          </button>
                        </div>
                        {projectsExpanded && (
                          <div className="ml-4 mt-2 max-h-[14.75rem] overflow-y-auto border-l border-border/70 pl-2 pr-1">
                            <div className="grid gap-1">
                              {sidebarProjects.map(project => (
                                <SidebarProjectItem
                                  key={project.id}
                                  locationPathname={location.pathname}
                                  pinned={pinnedProjectIds.has(project.id)}
                                  project={project}
                                  projectsExpanded={projectsExpanded}
                                  togglePending={toggleProjectPin.isPending}
                                  onTogglePin={(pinned) => {
                                    toggleProjectPin.mutate({ projectId: project.id, pinned })
                                  }}
                                />
                              ))}
                              {sidebarProjects.length === 0 && (
                                <p className="px-3 py-2 text-xs text-muted-foreground">{t('projectSpaces.emptyCompact')}</p>
                              )}
                            </div>
                          </div>
                        )}
                      </SidebarMenuItem>
                    )}
                    {items.map(item => (
                      <SidebarMenuItem key={item.to}>
                        <NavLink
                          className={({ isActive }) => sidebarMenuButtonClassName(isActive)}
                          title={t(item.labelKey)}
                          to={item.to}
                        >
                          <item.icon className="size-[17px] shrink-0" />
                          <span className="min-w-0 flex-1 truncate text-sm font-normal leading-none">{t(item.labelKey)}</span>
                        </NavLink>
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                </SidebarGroup>
              )
            })}
          </SidebarContent>
          <SidebarFooter>
            <ThemeModeSegmented mode={mode} setMode={setMode} />
            <SidebarUserPanel
              logoutLabel={t('logout')}
              logoutPending={isLoggingOut}
              user={user}
              onLogout={handleLogout}
            />
          </SidebarFooter>
        </Sidebar>

        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <header className="z-10 flex min-h-20 shrink-0 items-center justify-between border-b border-border bg-background/90 px-4 py-3 backdrop-blur lg:px-5 xl:px-6">
            <div className="min-w-0">
              <h1 className="truncate text-xl font-semibold tracking-normal">{pageMeta.title}</h1>
              <p className="mt-1 line-clamp-1 text-sm text-muted-foreground">{pageMeta.description}</p>
            </div>
            <div className="flex items-center gap-2 lg:hidden">
              <Button aria-label={t('logout')} disabled={isLoggingOut} variant="ghost" onClick={handleLogout}>
                {t('logout')}
              </Button>
            </div>
          </header>
          <main className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden px-4 py-5 lg:px-5 xl:px-6">
            <AnimatePresence mode="wait">
              <PageMotion key={pageMotionKey}>
                <Outlet />
              </PageMotion>
            </AnimatePresence>
          </main>
        </div>
      </div>
      <DebugFloatingPanel />
    </div>
  )
}
