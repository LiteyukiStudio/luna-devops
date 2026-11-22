import type { TFunction } from 'i18next'
import type { AIAssistantWorkspaceLocation } from './runtime-context'
import type { Project } from '@/api'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { normalizeProjectWorkspaceSection } from '@/pages/projects/overview/project-workspace-sections'
import { isAIAssistantRoutePath } from './route-state'

export type WorkspaceContextVisualKind = 'application' | 'project' | 'dashboard' | 'settings' | 'page'

export interface WorkspaceContextDisplay {
  kind: WorkspaceContextVisualKind
  label: string
}

interface WorkspaceContextEntities {
  applicationName?: string
  projectName?: string
}

export function formatWorkspaceContextDisplay(
  location: Pick<AIAssistantWorkspaceLocation, 'pathname' | 'search'>,
  t: TFunction,
  entities: WorkspaceContextEntities = {},
): WorkspaceContextDisplay {
  const normalized = location.pathname.replace(/\/+$/u, '') || '/'

  const appRouteMatch = normalized.match(/^\/projects\/([^/]+)\/apps\/([^/]+)(\/deployments\/(?:new|[^/]+\/edit))?$/)
  if (appRouteMatch?.[1] && appRouteMatch[2]) {
    const projectName = entities.projectName?.trim() || appRouteMatch[1]
    const appName = entities.applicationName?.trim() || appRouteMatch[2]
    const deploymentSegment = appRouteMatch[3]
    if (deploymentSegment?.endsWith('/new')) {
      return {
        kind: 'application',
        label: t('aiAssistant.contextSummary.deploymentCreate', { projectName, name: appName }),
      }
    }
    if (deploymentSegment) {
      return {
        kind: 'application',
        label: t('aiAssistant.contextSummary.deploymentEdit', { projectName, name: appName }),
      }
    }
    return {
      kind: 'application',
      label: t('aiAssistant.contextSummary.application', { projectName, name: appName }),
    }
  }

  const projectMatch = normalized.match(/^\/projects\/([^/]+)$/)
  if (projectMatch?.[1]) {
    const projectName = entities.projectName?.trim() || projectMatch[1]
    const section = normalizeProjectWorkspaceSection(new URLSearchParams(location.search).get('tab'))
    if (section !== 'overview') {
      const sectionLabel = resolveProjectSectionLabel(section, t)
      return {
        kind: 'project',
        label: t('aiAssistant.contextSummary.projectSection', { name: projectName, section: sectionLabel }),
      }
    }
    return {
      kind: 'project',
      label: t('aiAssistant.contextSummary.project', { name: projectName }),
    }
  }

  if (normalized === '/' || normalized === '/dashboard' || isAIAssistantRoutePath(normalized)) {
    return { kind: 'dashboard', label: t('dashboard') }
  }
  if (normalized === '/projects') {
    return { kind: 'project', label: t('projects') }
  }
  if (normalized === '/events') {
    return { kind: 'page', label: t('events') }
  }
  if (normalized === '/inbox') {
    return { kind: 'page', label: t('inbox.title') }
  }
  if (normalized === '/app-templates') {
    return { kind: 'page', label: t('appTemplates') }
  }
  if (normalized === '/code-repositories') {
    return { kind: 'page', label: t('codeRepositories') }
  }
  if (normalized === '/registries') {
    return { kind: 'page', label: t('registries') }
  }
  if (normalized === '/clusters') {
    return { kind: 'page', label: t('clusters') }
  }
  if (normalized === '/notification-channels') {
    return { kind: 'page', label: t('notificationChannels') }
  }
  if (normalized === '/billing') {
    return { kind: 'page', label: t('billing') }
  }
  if (normalized === '/settings/account') {
    return { kind: 'settings', label: t('aiAssistant.navigation.routes.account') }
  }
  if (normalized === '/settings/auth-providers') {
    return { kind: 'settings', label: t('authProviders') }
  }
  if (normalized === '/settings/notifications') {
    return { kind: 'settings', label: t('aiAssistant.navigation.routes.notifications') }
  }
  if (normalized === '/settings/operations') {
    return { kind: 'settings', label: t('operationsDashboard') }
  }
  if (normalized === '/settings/users') {
    return { kind: 'settings', label: t('aiAssistant.navigation.routes.users') }
  }
  if (normalized === '/settings' || normalized.startsWith('/settings/')) {
    return { kind: 'settings', label: t('siteSettings') }
  }

  return {
    kind: 'page',
    label: normalized,
  }
}

function resolveProjectSectionLabel(
  section: Exclude<ReturnType<typeof normalizeProjectWorkspaceSection>, 'overview'>,
  t: TFunction,
): string {
  switch (section) {
    case 'apps':
      return t('aiAssistant.contextSummary.projectSections.apps')
    case 'shared-config':
      return t('aiAssistant.contextSummary.projectSections.sharedConfig')
    case 'volumes':
      return t('aiAssistant.contextSummary.projectSections.volumes')
    case 'members':
      return t('aiAssistant.contextSummary.projectSections.members')
    case 'topology':
      return t('aiAssistant.contextSummary.projectSections.topology')
    case 'notifications':
      return t('aiAssistant.contextSummary.projectSections.notifications')
    case 'ai-diagnosis':
      return t('aiAssistant.contextSummary.projectSections.aiDiagnosis')
  }
}

export function useWorkspaceContextDisplay(location: AIAssistantWorkspaceLocation): WorkspaceContextDisplay {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const normalized = location.pathname.replace(/\/+$/u, '') || '/'
  const projectMatch = normalized.match(/^\/projects\/([^/]+)/)
  const appMatch = normalized.match(/^\/projects\/([^/]+)\/apps\/([^/]+)/)
  const projectId = projectMatch?.[1] ?? ''
  const appId = appMatch?.[2] ?? ''

  const projectQuery = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => api.getProject(projectId),
    enabled: Boolean(user && projectId),
    retry: false,
    staleTime: 60_000,
  })
  const applicationQuery = useQuery({
    queryKey: ['application', projectId, appId],
    queryFn: () => api.getApplication(projectId, appId),
    enabled: Boolean(user && projectId && appId),
    retry: false,
    staleTime: 60_000,
  })

  const cachedProjects = projectId ? queryClient.getQueryData<Project[]>(['projects']) : undefined
  const cachedProject = cachedProjects?.find(item => item.id === projectId || item.identifier === projectId)
  const projectName = projectQuery.data?.name ?? cachedProject?.name
  const applicationName = applicationQuery.data?.name

  return formatWorkspaceContextDisplay(location, t, {
    applicationName,
    projectName,
  })
}
