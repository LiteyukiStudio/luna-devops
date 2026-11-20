import type { LucideIcon } from 'lucide-react'
import type { CommandPaletteRouteContext, CommandPaletteScope } from './command-palette-model'
import type { SearchResultItem } from '@/api'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import {
  AlertTriangle,
  AppWindow,
  Bell,
  Boxes,
  Container,
  Database,
  FileSliders,
  FolderKanban,
  GitBranch,
  Globe,
  Globe2,
  Hammer,
  HardDrive,
  History,
  Loader2,
  Plus,
  Rocket,
  RotateCw,
  Search,
  Server,
  Sparkles,
  Store,
  SunMoon,
  UserRound,
  X,
} from 'lucide-react'
import { use, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { useTheme } from '@/app/theme-context'
import { useAIAssistantPresentationMode } from '@/components/common/ai-assistant/presentation-mode'
import { AI_ASSISTANT_ROUTE_PATH, createAIAssistantRouteState } from '@/components/common/ai-assistant/route-state'
import { AIAssistantRuntimeContext } from '@/components/common/ai-assistant/runtime-context'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from '@/components/ui/command'
import { isPlatformAdmin } from '@/lib/roles'
import { cn } from '@/lib/utils'
import {
  buildPageCommands,
  COMMAND_PALETTE_SCOPES,
  formatSearchResultContext,
  hasInlineResourcePrefix,
  matchesLocalSearchQuery,
  parseInlineTemplateQuery,
  readCommandPaletteRecents,
  recordCommandPaletteRecent,
  resolveRoutePaletteContext,
  resolveSearchResultPath,
  scopeForRouteContext,
  searchTypesForPalette,
  selectCommandPaletteTemplates,
} from './command-palette-model'

/** 当前项目空间的分区入口；分区定义由布局从项目空间工作区目录传入。 */
export interface ProjectSectionCommand {
  to: string
  label: string
  icon: LucideIcon
}

// 编辑器和终端有自己的快捷键语义，焦点在其中时不抢占 ⌘K / Ctrl+K。
const SHORTCUT_EXCLUDED_SELECTOR = '.cm-editor, .xterm'
const SEARCH_DEBOUNCE_MS = 160

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

function SearchResultTypeIcon({ type }: { type: SearchResultItem['type'] }) {
  switch (type) {
    case 'project':
      return <FolderKanban aria-hidden="true" />
    case 'application':
      return <AppWindow aria-hidden="true" />
    case 'deployment_target':
      return <Boxes aria-hidden="true" />
    case 'gateway_route':
      return <Globe aria-hidden="true" />
    case 'repository_binding':
      return <GitBranch aria-hidden="true" />
    case 'project_volume':
      return <HardDrive aria-hidden="true" />
    case 'runtime_config_set':
      return <FileSliders aria-hidden="true" />
    case 'build_variable_set':
      return <Database aria-hidden="true" />
    case 'notification_channel':
      return <Bell aria-hidden="true" />
    case 'runtime_cluster':
      return <Server aria-hidden="true" />
    case 'artifact_registry':
      return <Container aria-hidden="true" />
    case 'build_run':
      return <Hammer aria-hidden="true" />
    case 'release':
      return <Rocket aria-hidden="true" />
    case 'user':
      return <UserRound aria-hidden="true" />
  }
}

/** 全局命令面板（Omnibox）：顶栏搜索按钮或 ⌘K / Ctrl+K 打开，支持跨空间资源检索、页面与模板导航、最近访问和常用操作。 */
export function CommandPalette({ projectSections = [] }: { projectSections?: ProjectSectionCommand[] }) {
  const { t } = useTranslation()
  const { user } = useSession()
  const { setMode } = useTheme()
  const aiRuntime = use(AIAssistantRuntimeContext)
  const navigate = useNavigate()
  const location = useLocation()
  const presentationMode = useAIAssistantPresentationMode(location.pathname)

  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [scope, setScope] = useState<CommandPaletteScope>('all')
  const [routeContext, setRouteContext] = useState<CommandPaletteRouteContext | null>(null)
  const [allSpaces, setAllSpaces] = useState(false)
  const [recentsVersion, setRecentsVersion] = useState(0)

  const canViewAllSpaces = isPlatformAdmin(user?.role)
  const effectiveVisibility = canViewAllSpaces && allSpaces ? 'all' : undefined
  const trimmedQuery = query.trim()
  const effectiveServerQuery = trimmedQuery === '' ? '' : debouncedQuery.trim()
  const inlineTemplate = useMemo(() => parseInlineTemplateQuery(trimmedQuery), [trimmedQuery])
  const templateFocused = scope === 'app_template' || inlineTemplate.matched
  const hasServerOnlyRouteContext = routeContext !== null && routeContext !== 'app_template' && routeContext !== 'project'
  const inlineResourceMode = hasInlineResourcePrefix(trimmedQuery) || inlineTemplate.matched
  const allowLocalNavigation = !inlineResourceMode && !hasServerOnlyRouteContext && (scope === 'all' || scope === 'navigation')
  const allowLocalProjectSections = !inlineResourceMode && !hasServerOnlyRouteContext && (scope === 'all' || scope === 'project' || scope === 'navigation')
  const allowLocalApplications = !inlineResourceMode && !hasServerOnlyRouteContext && (scope === 'all' || scope === 'application')
  const allowLocalProjects = !inlineResourceMode && !hasServerOnlyRouteContext && (scope === 'all' || scope === 'project')

  const currentProjectId = location.pathname.match(/^\/projects\/([^/]+)/)?.[1]
  const currentAppId = location.pathname.match(/^\/projects\/[^/]+\/apps\/([^/]+)/)?.[1]

  useEffect(() => {
    if (trimmedQuery === '')
      return
    const timer = window.setTimeout(() => {
      setDebouncedQuery(trimmedQuery)
    }, SEARCH_DEBOUNCE_MS)
    return () => window.clearTimeout(timer)
  }, [trimmedQuery])

  const projects = useQuery({
    queryKey: ['projects', effectiveVisibility ?? 'related'],
    queryFn: () => api.listProjects(effectiveVisibility),
    enabled: open && Boolean(user),
    staleTime: 30_000,
  })

  const applications = useQuery({
    queryKey: ['command-palette-applications', currentProjectId],
    queryFn: () => api.listApplications(currentProjectId ?? ''),
    enabled: open && Boolean(user && currentProjectId),
    staleTime: 30_000,
  })

  const appTemplates = useQuery({
    queryKey: ['app-templates'],
    queryFn: () => api.listAppTemplates(),
    enabled: open && Boolean(user) && (templateFocused || (trimmedQuery.length > 0 && allowLocalNavigation)),
    staleTime: 5 * 60_000,
  })

  const scopeSearchTypes = useMemo(() => searchTypesForPalette(scope, routeContext), [scope, routeContext])
  const serverSearchEnabled = open
    && Boolean(user)
    && scope !== 'navigation'
    && scope !== 'app_template'
    && !inlineTemplate.matched
    && effectiveServerQuery.length > 0

  const serverSearch = useQuery({
    queryKey: ['global-search', effectiveServerQuery, scope, routeContext ?? 'none', effectiveVisibility ?? 'related'],
    queryFn: ({ signal }) => api.searchPlatformResources({
      q: effectiveServerQuery,
      types: scopeSearchTypes,
      limitPerType: 6,
      visibility: effectiveVisibility,
    }, signal),
    enabled: serverSearchEnabled,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

  const currentProject = projects.data?.find(project => project.id === currentProjectId)
  const currentApplication = applications.data?.find(app => app.id === currentAppId)

  // 自动记录当前正在浏览的项目空间或应用，便于下次 ⌘K 零输入一键切换。
  useEffect(() => {
    if (!user?.id || !currentProject)
      return
    if (currentAppId && currentApplication) {
      recordCommandPaletteRecent(user.id, {
        key: `application:${currentApplication.id}`,
        kind: 'application',
        id: currentApplication.id,
        projectId: currentProject.id,
        label: currentApplication.name,
        sublabel: currentProject.name,
        to: `/projects/${currentProject.id}/apps/${currentApplication.id}`,
      })
      return
    }
    if (!currentAppId) {
      recordCommandPaletteRecent(user.id, {
        key: `project:${currentProject.id}`,
        kind: 'project',
        id: currentProject.id,
        label: currentProject.name,
        sublabel: currentProject.identifier,
        to: `/projects/${currentProject.id}`,
      })
    }
  }, [user?.id, currentProject, currentAppId, currentApplication])

  function applyOpenState(nextOpen: boolean, pathname: string) {
    setOpen(nextOpen)
    if (nextOpen) {
      const nextContext = resolveRoutePaletteContext(pathname)
      setRouteContext(nextContext)
      setScope(scopeForRouteContext(nextContext))
      return
    }
    setQuery('')
    setDebouncedQuery('')
    setRouteContext(null)
    setScope('all')
  }

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (!isCommandPaletteShortcut(event))
        return
      event.preventDefault()
      applyOpenState(!open, location.pathname)
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [open, location.pathname])

  function handleOpenChange(nextOpen: boolean) {
    applyOpenState(nextOpen, location.pathname)
  }

  function clearRouteContext() {
    setRouteContext(null)
    setScope('all')
  }

  function run(action: () => void) {
    handleOpenChange(false)
    action()
  }

  function toggleTheme() {
    setMode(document.documentElement.classList.contains('dark') ? 'light' : 'dark')
  }

  function openSearchResult(item: SearchResultItem) {
    const fallbackProjectId = currentProjectId || projects.data?.[0]?.id
    const targetPath = resolveSearchResultPath(item, fallbackProjectId)
    if (user?.id && item.type === 'project') {
      recordCommandPaletteRecent(user.id, {
        key: `project:${item.id}`,
        kind: 'project',
        id: item.id,
        label: item.title,
        sublabel: item.identifier,
        to: targetPath,
      })
      setRecentsVersion(v => v + 1)
    }
    else if (user?.id && item.type === 'application' && item.projectId) {
      recordCommandPaletteRecent(user.id, {
        key: `application:${item.id}`,
        kind: 'application',
        id: item.id,
        projectId: item.projectId,
        label: item.title,
        sublabel: item.projectName,
        to: targetPath,
      })
      setRecentsVersion(v => v + 1)
    }
    run(() => navigate(targetPath))
  }

  function askLunaAssistant() {
    if (!aiRuntime?.enabled || !trimmedQuery)
      return
    const draft = trimmedQuery
    run(() => {
      aiRuntime.composerProps.onDraftChange(draft)
      if (presentationMode === 'window') {
        aiRuntime.openAssistant()
        return
      }
      aiRuntime.rememberWorkspaceLocation(location)
      navigate(AI_ASSISTANT_ROUTE_PATH, {
        state: createAIAssistantRouteState(location),
      })
    })
  }

  const recents = useMemo(() => {
    void recentsVersion
    if (!open)
      return []
    return readCommandPaletteRecents(user?.id)
  }, [open, user?.id, recentsVersion])

  const pages = useMemo(
    () => buildPageCommands(t).filter(page => !page.permission || user?.permissions.includes(page.permission)),
    [t, user?.permissions],
  )

  const shortcut = isApplePlatform() ? t('nav.commandPalette.shortcutMac') : t('nav.commandPalette.shortcutOther')

  const serverItems = useMemo(
    () => (serverSearchEnabled ? (serverSearch.data?.items ?? []) : []),
    [serverSearchEnabled, serverSearch.data?.items],
  )

  const serverItemsByGroup = useMemo(() => {
    const grouped = {
      applications: [] as SearchResultItem[],
      gatewayRoutes: [] as SearchResultItem[],
      deploymentTargets: [] as SearchResultItem[],
      projects: [] as SearchResultItem[],
      repositories: [] as SearchResultItem[],
      volumes: [] as SearchResultItem[],
      configs: [] as SearchResultItem[],
      notificationChannels: [] as SearchResultItem[],
      infrastructure: [] as SearchResultItem[],
      history: [] as SearchResultItem[],
      users: [] as SearchResultItem[],
    }
    for (const item of serverItems) {
      switch (item.type) {
        case 'application':
          grouped.applications.push(item)
          break
        case 'gateway_route':
          grouped.gatewayRoutes.push(item)
          break
        case 'deployment_target':
          grouped.deploymentTargets.push(item)
          break
        case 'project':
          grouped.projects.push(item)
          break
        case 'repository_binding':
          grouped.repositories.push(item)
          break
        case 'project_volume':
          grouped.volumes.push(item)
          break
        case 'runtime_config_set':
        case 'build_variable_set':
          grouped.configs.push(item)
          break
        case 'notification_channel':
          grouped.notificationChannels.push(item)
          break
        case 'runtime_cluster':
        case 'artifact_registry':
          grouped.infrastructure.push(item)
          break
        case 'build_run':
        case 'release':
          grouped.history.push(item)
          break
        case 'user':
          grouped.users.push(item)
          break
      }
    }
    return grouped
  }, [serverItems])

  const serverAppIds = useMemo(
    () => new Set(serverItemsByGroup.applications.map(item => item.id)),
    [serverItemsByGroup.applications],
  )
  const serverProjectIds = useMemo(
    () => new Set(serverItemsByGroup.projects.map(item => item.id)),
    [serverItemsByGroup.projects],
  )

  const filteredPages = useMemo(
    () => (allowLocalNavigation
      ? pages.filter(page => matchesLocalSearchQuery(trimmedQuery, page.label, page.to, page.keywords))
      : []),
    [allowLocalNavigation, pages, trimmedQuery],
  )

  const filteredSections = useMemo(
    () => (currentProjectId && allowLocalProjectSections
      ? projectSections.filter(section => matchesLocalSearchQuery(trimmedQuery, section.label, section.to))
      : []),
    [currentProjectId, allowLocalProjectSections, projectSections, trimmedQuery],
  )

  const filteredCurrentApps = useMemo(
    () => (currentProjectId && allowLocalApplications
      ? (applications.data ?? []).filter(app => (
          !serverAppIds.has(app.id)
          && matchesLocalSearchQuery(trimmedQuery, app.name, app.identifier, app.id)
        ))
      : []),
    [currentProjectId, allowLocalApplications, applications.data, serverAppIds, trimmedQuery],
  )

  const filteredLocalProjects = useMemo(
    () => (allowLocalProjects
      ? (projects.data ?? []).filter(project => (
          !serverProjectIds.has(project.id)
          && matchesLocalSearchQuery(trimmedQuery, project.name, project.identifier, project.kubernetesNamespace, project.id)
        ))
      : []),
    [allowLocalProjects, projects.data, serverProjectIds, trimmedQuery],
  )

  const filteredTemplates = useMemo(
    () => selectCommandPaletteTemplates(appTemplates.data ?? [], {
      query: inlineTemplate.matched ? inlineTemplate.query : trimmedQuery,
      templateFocused,
      allowInGeneralSearch: allowLocalNavigation,
    }),
    [appTemplates.data, inlineTemplate.matched, inlineTemplate.query, trimmedQuery, templateFocused, allowLocalNavigation],
  )

  const showCreateProjectAction = allowLocalNavigation && matchesLocalSearchQuery(
    trimmedQuery,
    t('nav.commandPalette.actions.createProject'),
    'create new project space 新建 创建 项目空间',
  )
  const showToggleThemeAction = allowLocalNavigation && matchesLocalSearchQuery(
    trimmedQuery,
    t('nav.commandPalette.actions.toggleTheme'),
    'theme dark light mode appearance 主题 深色 暗色 亮色 明暗',
  )

  const isSearchingServer = serverSearchEnabled && (serverSearch.isFetching || effectiveServerQuery !== trimmedQuery)
  const activeContextLabel = routeContext ? t(`nav.commandPalette.contextChips.${routeContext}`) : ''

  function renderServerResultItem(item: SearchResultItem) {
    const contextText = formatSearchResultContext(item)
    return (
      <CommandItem
        key={`${item.type}:${item.id}`}
        value={`${item.type}:${item.id}:${item.title}`}
        onSelect={() => openSearchResult(item)}
      >
        <SearchResultTypeIcon type={item.type} />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <div className="flex items-center gap-1.5">
            <span className="truncate font-medium">{item.title}</span>
            {!item.related && (
              <Badge className="h-5 px-1.5 text-xs" variant="outline">
                {t('nav.commandPalette.globalBadge')}
              </Badge>
            )}
          </div>
          {contextText && (
            <span className="truncate text-xs text-muted-foreground">{contextText}</span>
          )}
        </div>
        {item.identifier && (
          <CommandShortcut className="max-w-40 truncate font-mono tracking-normal">
            {item.identifier}
          </CommandShortcut>
        )}
      </CommandItem>
    )
  }

  return (
    <>
      <Button
        aria-keyshortcuts="Meta+K Control+K"
        className="h-8 w-8 shrink-0 gap-2 px-0 text-xs hover:bg-primary-subtle-hover sm:w-48 sm:justify-start sm:px-3 sm:text-muted-foreground"
        size="sm"
        type="button"
        variant="outline"
        onClick={() => handleOpenChange(true)}
      >
        <Search aria-hidden="true" className="size-4" />
        <span className="sr-only sm:not-sr-only sm:flex-1 sm:text-left">{t('nav.commandPalette.trigger')}</span>
        <kbd aria-hidden="true" className="hidden font-sans text-xs leading-5 text-muted-foreground sm:inline-block">{shortcut}</kbd>
      </Button>
      <CommandDialog
        className="sm:max-w-xl"
        description={t('nav.commandPalette.description')}
        open={open}
        shouldFilter={false}
        showCloseButton={false}
        title={t('nav.commandPalette.title')}
        onOpenChange={handleOpenChange}
      >
        <CommandInput
          placeholder={t('nav.commandPalette.placeholder')}
          prefix={routeContext
            ? (
                <span
                  className="inline-flex shrink-0 items-center gap-1 rounded-control border border-primary-border bg-primary-subtle px-2 py-0.5 text-xs font-medium text-primary-text"
                  data-testid="command-palette-context-chip"
                >
                  <span>{activeContextLabel}</span>
                  <button
                    aria-label={t('nav.commandPalette.clearContextChip', { context: activeContextLabel })}
                    className="inline-flex items-center justify-center rounded-sm p-0.5 text-primary-text/80 transition-colors hover:bg-primary/15 hover:text-primary-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    type="button"
                    onClick={clearRouteContext}
                    onMouseDown={event => event.preventDefault()}
                  >
                    <X aria-hidden="true" className="size-3" />
                  </button>
                </span>
              )
            : undefined}
          value={query}
          onKeyDown={(event) => {
            if (event.key === 'Backspace' && query === '' && routeContext !== null) {
              event.preventDefault()
              clearRouteContext()
            }
          }}
          onValueChange={setQuery}
        />
        <div
          aria-label={t('nav.commandPalette.scopes.label')}
          className="flex flex-wrap items-center justify-between gap-1.5 border-b border-border bg-muted/30 px-3 py-1.5"
          role="toolbar"
        >
          <div className="flex flex-wrap items-center gap-1">
            {COMMAND_PALETTE_SCOPES.map((itemScope) => {
              const active = scope === itemScope && (itemScope !== 'all' || !hasServerOnlyRouteContext)
              return (
                <button
                  key={itemScope}
                  aria-pressed={active}
                  className={cn(
                    'rounded-control px-2 py-0.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                    active
                      ? 'bg-primary text-primary-foreground shadow-2xs'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                  )}
                  type="button"
                  onClick={() => {
                    setRouteContext(null)
                    setScope(itemScope)
                  }}
                  onMouseDown={event => event.preventDefault()}
                >
                  {t(`nav.commandPalette.scopes.${itemScope}`)}
                </button>
              )
            })}
          </div>
          <div className="flex items-center gap-1.5">
            {isSearchingServer && (
              <Loader2 aria-label={t('nav.commandPalette.searching')} className="size-3.5 animate-spin text-muted-foreground" />
            )}
            {canViewAllSpaces && (
              <button
                aria-pressed={allSpaces}
                className={cn(
                  'inline-flex items-center gap-1 rounded-control border px-2 py-0.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  allSpaces
                    ? 'border-primary-border bg-primary-subtle text-primary-text'
                    : 'border-border bg-background text-muted-foreground hover:text-foreground',
                )}
                title={t('nav.commandPalette.allSpacesHint')}
                type="button"
                onClick={() => setAllSpaces(current => !current)}
                onMouseDown={event => event.preventDefault()}
              >
                <Globe2 aria-hidden="true" className="size-3" />
                <span>{t('nav.commandPalette.allSpacesToggle')}</span>
              </button>
            )}
          </div>
        </div>
        <CommandList className="max-h-[min(28rem,66dvh)]">
          {serverSearchEnabled && serverSearch.isError && (
            <div className="mx-3 my-2 flex items-center justify-between gap-3 rounded-control border border-danger-border bg-danger-subtle px-3 py-2 text-xs text-danger" role="alert">
              <span className="inline-flex min-w-0 items-center gap-2">
                <AlertTriangle aria-hidden="true" className="size-4 shrink-0" />
                <span className="truncate">{t('nav.commandPalette.searchFailed')}</span>
              </span>
              <Button
                className="h-7 shrink-0 px-2.5 text-xs"
                disabled={serverSearch.isFetching}
                size="sm"
                type="button"
                variant="outline"
                onClick={() => void serverSearch.refetch()}
              >
                <RotateCw className={cn('size-3.5', serverSearch.isFetching && 'animate-spin')} />
                {t('common.retry')}
              </Button>
            </div>
          )}
          {!(serverSearchEnabled && serverSearch.isError) && (
            <CommandEmpty>
              {isSearchingServer || (templateFocused && appTemplates.isLoading)
                ? t('nav.commandPalette.searching')
                : (hasServerOnlyRouteContext && trimmedQuery === '')
                    ? t('nav.commandPalette.contextEmptyHint', { context: activeContextLabel })
                    : t('nav.commandPalette.empty')}
            </CommandEmpty>
          )}

          {trimmedQuery === '' && scope === 'all' && routeContext === null && recents.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.recent')}>
              {recents.map(recent => (
                <CommandItem
                  key={`recent:${recent.key}`}
                  value={`recent:${recent.key}:${recent.label}`}
                  onSelect={() => run(() => navigate(recent.to))}
                >
                  <History aria-hidden="true" />
                  <span className="truncate">{recent.label}</span>
                  {recent.sublabel && (
                    <CommandShortcut className="max-w-40 truncate font-mono tracking-normal">
                      {recent.sublabel}
                    </CommandShortcut>
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          )}

          {serverItemsByGroup.applications.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.allApplications')}>
              {serverItemsByGroup.applications.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {serverItemsByGroup.gatewayRoutes.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.gatewayRoutes')}>
              {serverItemsByGroup.gatewayRoutes.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {serverItemsByGroup.deploymentTargets.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.deploymentTargets')}>
              {serverItemsByGroup.deploymentTargets.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {serverItemsByGroup.repositories.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.repositories')}>
              {serverItemsByGroup.repositories.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {serverItemsByGroup.volumes.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.volumes')}>
              {serverItemsByGroup.volumes.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {serverItemsByGroup.configs.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.configs')}>
              {serverItemsByGroup.configs.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {serverItemsByGroup.notificationChannels.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.notificationChannels')}>
              {serverItemsByGroup.notificationChannels.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {serverItemsByGroup.infrastructure.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.infrastructure')}>
              {serverItemsByGroup.infrastructure.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {serverItemsByGroup.history.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.history')}>
              {serverItemsByGroup.history.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {serverItemsByGroup.users.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.users')}>
              {serverItemsByGroup.users.map(renderServerResultItem)}
            </CommandGroup>
          )}

          {filteredSections.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.projectSections', { project: currentProject?.name ?? t('projects') })}>
              {filteredSections.map(section => (
                <CommandItem key={section.to} value={`${section.label} ${section.to}`} onSelect={() => run(() => navigate(section.to))}>
                  <section.icon aria-hidden="true" />
                  <span>{section.label}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}

          {filteredCurrentApps.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.applications', { project: currentProject?.name ?? t('projects') })}>
              {filteredCurrentApps.map(application => (
                <CommandItem
                  key={application.id}
                  value={`${application.name} ${application.identifier} ${application.id}`}
                  onSelect={() => {
                    if (user?.id) {
                      recordCommandPaletteRecent(user.id, {
                        key: `application:${application.id}`,
                        kind: 'application',
                        id: application.id,
                        projectId: application.projectId,
                        label: application.name,
                        sublabel: currentProject?.name,
                        to: `/projects/${application.projectId}/apps/${application.id}`,
                      })
                      setRecentsVersion(v => v + 1)
                    }
                    run(() => navigate(`/projects/${application.projectId}/apps/${application.id}`))
                  }}
                >
                  <AppWindow aria-hidden="true" />
                  <span className="truncate">{application.name}</span>
                  <CommandShortcut className="truncate font-mono tracking-normal">{application.identifier}</CommandShortcut>
                </CommandItem>
              ))}
            </CommandGroup>
          )}

          {(serverItemsByGroup.projects.length > 0 || filteredLocalProjects.length > 0) && (
            <CommandGroup heading={t('nav.commandPalette.groups.projects')}>
              {serverItemsByGroup.projects.map(renderServerResultItem)}
              {filteredLocalProjects.map(project => (
                <CommandItem
                  key={project.id}
                  value={`${project.name} ${project.identifier} ${project.id}`}
                  onSelect={() => {
                    if (user?.id) {
                      recordCommandPaletteRecent(user.id, {
                        key: `project:${project.id}`,
                        kind: 'project',
                        id: project.id,
                        label: project.name,
                        sublabel: project.identifier,
                        to: `/projects/${project.id}`,
                      })
                      setRecentsVersion(v => v + 1)
                    }
                    run(() => navigate(`/projects/${project.id}`))
                  }}
                >
                  <FolderKanban aria-hidden="true" />
                  <span className="truncate">{project.name}</span>
                  <CommandShortcut className="truncate font-mono tracking-normal">{project.identifier}</CommandShortcut>
                </CommandItem>
              ))}
            </CommandGroup>
          )}

          {filteredTemplates.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.appTemplates')}>
              {filteredTemplates.map(template => (
                <CommandItem
                  key={`template:${template.id}`}
                  value={`template:${template.id}:${template.name}`}
                  onSelect={() => run(() => navigate(`/app-templates?template=${encodeURIComponent(template.id)}`))}
                >
                  <Store aria-hidden="true" />
                  <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="truncate font-medium">{template.name}</span>
                    {template.description && (
                      <span className="truncate text-xs text-muted-foreground">{template.description}</span>
                    )}
                  </div>
                  <CommandShortcut className="truncate font-mono tracking-normal">{template.slug}</CommandShortcut>
                </CommandItem>
              ))}
            </CommandGroup>
          )}

          {filteredPages.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.pages')}>
              {filteredPages.map(page => (
                <CommandItem key={page.to} value={`${page.label} ${page.to}`} onSelect={() => run(() => navigate(page.to))}>
                  <page.icon aria-hidden="true" />
                  <span>{page.label}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}

          {(showCreateProjectAction || showToggleThemeAction) && (
            <CommandGroup heading={t('nav.commandPalette.groups.actions')}>
              {showCreateProjectAction && (
                <CommandItem value={t('nav.commandPalette.actions.createProject')} onSelect={() => run(() => navigate('/projects?create=1'))}>
                  <Plus aria-hidden="true" />
                  <span>{t('nav.commandPalette.actions.createProject')}</span>
                </CommandItem>
              )}
              {showToggleThemeAction && (
                <CommandItem value={t('nav.commandPalette.actions.toggleTheme')} onSelect={() => run(toggleTheme)}>
                  <SunMoon aria-hidden="true" />
                  <span>{t('nav.commandPalette.actions.toggleTheme')}</span>
                </CommandItem>
              )}
            </CommandGroup>
          )}

          {aiRuntime?.enabled && trimmedQuery.length > 0 && (
            <CommandGroup heading={t('nav.commandPalette.groups.assistant')}>
              <CommandItem value={`ask-assistant:${trimmedQuery}`} onSelect={askLunaAssistant}>
                <Sparkles aria-hidden="true" className="text-primary-text" />
                <span className="truncate">{t('nav.commandPalette.actions.askAssistant', { query: trimmedQuery })}</span>
              </CommandItem>
            </CommandGroup>
          )}
        </CommandList>
      </CommandDialog>
    </>
  )
}
