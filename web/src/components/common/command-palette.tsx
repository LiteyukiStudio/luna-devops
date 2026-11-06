import type { TFunction } from 'i18next'
import type { LucideIcon } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { AppWindow, Bell, ChartNoAxesCombined, CircleUserRound, Container, CreditCard, Fingerprint, FolderKanban, GitBranch, LayoutDashboard, Plus, ScrollText, Search, Server, Settings, Store, SunMoon, Users } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { useTheme } from '@/app/theme-context'
import { Button } from '@/components/ui/button'
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandShortcut } from '@/components/ui/command'

interface PageCommand {
  to: string
  label: string
  icon: LucideIcon
  permission?: string
}

/** 当前项目空间的分区入口；分区定义由布局从项目空间工作区目录传入。 */
export interface ProjectSectionCommand {
  to: string
  label: string
  icon: LucideIcon
}

// 与侧栏和头像菜单的页面入口保持一致；系统管理页仍由后端最终鉴权，这里只按权限隐藏入口。
function pageCommands(t: TFunction): PageCommand[] {
  return [
    { to: '/dashboard', label: t('dashboard'), icon: LayoutDashboard },
    { to: '/projects', label: t('projects'), icon: FolderKanban },
    { to: '/app-templates', label: t('appTemplates'), icon: Store },
    { to: '/events', label: t('events'), icon: ScrollText },
    { to: '/code-repositories', label: t('codeRepositories'), icon: GitBranch },
    { to: '/registries', label: t('registries'), icon: Container },
    { to: '/clusters', label: t('clusters'), icon: Server },
    { to: '/billing', label: t('billing'), icon: CreditCard },
    { to: '/settings/account', label: t('account'), icon: CircleUserRound },
    { to: '/settings/auth-providers', label: t('authProviders'), icon: Fingerprint, permission: 'user.manage' },
    { to: '/settings/users', label: t('users'), icon: Users, permission: 'user.manage' },
    { to: '/settings/notifications', label: t('notifications'), icon: Bell, permission: 'user.manage' },
    { to: '/settings/operations', label: t('operationsDashboard'), icon: ChartNoAxesCombined, permission: 'user.manage' },
    { to: '/settings/site', label: t('siteSettings'), icon: Settings, permission: 'user.manage' },
  ]
}

// 编辑器和终端有自己的快捷键语义，焦点在其中时不抢占 ⌘K / Ctrl+K。
const SHORTCUT_EXCLUDED_SELECTOR = '.cm-editor, .xterm'

function isCommandPaletteShortcut(event: KeyboardEvent) {
  if (event.defaultPrevented || event.isComposing || event.altKey || event.shiftKey)
    return false
  if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== 'k')
    return false
  return !(event.target instanceof Element && event.target.closest(SHORTCUT_EXCLUDED_SELECTOR))
}

function isApplePlatform() {
  return typeof navigator !== 'undefined' && /Mac|iPhone|iPad|iPod/.test(navigator.userAgent)
}

/** 全局命令面板：顶栏搜索按钮或 ⌘K / Ctrl+K 打开，用于跳转页面、项目空间、当前项目空间的分区与应用，以及执行常用操作。 */
export function CommandPalette({ projectSections = [] }: { projectSections?: ProjectSectionCommand[] }) {
  const { t } = useTranslation()
  const { user } = useSession()
  const { setMode } = useTheme()
  const navigate = useNavigate()
  const location = useLocation()
  const [open, setOpen] = useState(false)
  const currentProjectId = location.pathname.match(/^\/projects\/([^/]+)/)?.[1]
  const projects = useQuery({ queryKey: ['projects'], queryFn: () => api.listProjects(), enabled: open && Boolean(user) })
  const applications = useQuery({
    queryKey: ['command-palette-applications', currentProjectId],
    queryFn: () => api.listApplications(currentProjectId ?? ''),
    enabled: open && Boolean(user && currentProjectId),
  })
  const currentProject = projects.data?.find(project => project.id === currentProjectId)
  const pages = pageCommands(t).filter(page => !page.permission || user?.permissions.includes(page.permission))
  const shortcut = isApplePlatform() ? t('nav.commandPalette.shortcutMac') : t('nav.commandPalette.shortcutOther')

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (!isCommandPaletteShortcut(event))
        return
      event.preventDefault()
      setOpen(current => !current)
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [])

  function run(action: () => void) {
    setOpen(false)
    action()
  }

  function toggleTheme() {
    setMode(document.documentElement.classList.contains('dark') ? 'light' : 'dark')
  }

  return (
    <>
      <Button
        aria-keyshortcuts="Meta+K Control+K"
        className="h-8 shrink-0 gap-2 px-2 text-muted-foreground sm:w-48 sm:justify-start sm:px-3"
        size="sm"
        type="button"
        variant="outline"
        onClick={() => setOpen(true)}
      >
        <Search aria-hidden="true" className="size-4" />
        <span className="sr-only sm:not-sr-only sm:flex-1 sm:text-left">{t('nav.commandPalette.trigger')}</span>
        <kbd aria-hidden="true" className="hidden rounded-sm border border-border bg-muted px-1.5 font-mono text-[0.6875rem] leading-5 text-muted-foreground sm:inline-block">{shortcut}</kbd>
      </Button>
      <CommandDialog
        description={t('nav.commandPalette.description')}
        open={open}
        showCloseButton={false}
        title={t('nav.commandPalette.title')}
        onOpenChange={setOpen}
      >
        <CommandInput placeholder={t('nav.commandPalette.placeholder')} />
        <CommandList className="max-h-[min(24rem,60vh)]">
          <CommandEmpty>{t('nav.commandPalette.empty')}</CommandEmpty>
          <CommandGroup heading={t('nav.commandPalette.groups.pages')}>
            {pages.map(page => (
              <CommandItem key={page.to} value={`${page.label} ${page.to}`} onSelect={() => run(() => navigate(page.to))}>
                <page.icon aria-hidden="true" />
                <span>{page.label}</span>
              </CommandItem>
            ))}
          </CommandGroup>
          {currentProjectId && projectSections.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.projectSections', { project: currentProject?.name ?? t('projects') })}>
              {projectSections.map(section => (
                <CommandItem key={section.to} value={`${section.label} ${section.to}`} onSelect={() => run(() => navigate(section.to))}>
                  <section.icon aria-hidden="true" />
                  <span>{section.label}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}
          {currentProjectId && (applications.data?.length ?? 0) > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.applications', { project: currentProject?.name ?? t('projects') })}>
              {applications.data?.map(application => (
                <CommandItem
                  key={application.id}
                  value={`${application.name} ${application.identifier} ${application.id}`}
                  onSelect={() => run(() => navigate(`/projects/${application.projectId}/apps/${application.id}`))}
                >
                  <AppWindow aria-hidden="true" />
                  <span className="truncate">{application.name}</span>
                  <CommandShortcut className="truncate font-mono tracking-normal">{application.identifier}</CommandShortcut>
                </CommandItem>
              ))}
            </CommandGroup>
          )}
          {(projects.data?.length ?? 0) > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.projects')}>
              {projects.data?.map(project => (
                <CommandItem key={project.id} value={`${project.name} ${project.identifier} ${project.id}`} onSelect={() => run(() => navigate(`/projects/${project.id}`))}>
                  <FolderKanban aria-hidden="true" />
                  <span className="truncate">{project.name}</span>
                  <CommandShortcut className="truncate font-mono tracking-normal">{project.identifier}</CommandShortcut>
                </CommandItem>
              ))}
            </CommandGroup>
          )}
          <CommandGroup heading={t('nav.commandPalette.groups.actions')}>
            <CommandItem value={t('nav.commandPalette.actions.createProject')} onSelect={() => run(() => navigate('/projects?create=1'))}>
              <Plus aria-hidden="true" />
              <span>{t('nav.commandPalette.actions.createProject')}</span>
            </CommandItem>
            <CommandItem value={t('nav.commandPalette.actions.toggleTheme')} onSelect={() => run(toggleTheme)}>
              <SunMoon aria-hidden="true" />
              <span>{t('nav.commandPalette.actions.toggleTheme')}</span>
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  )
}
