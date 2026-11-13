import type { TFunction } from 'i18next'
import type { LucideIcon } from 'lucide-react'
import type { SearchResourceType, SearchResultItem } from '@/api'
import {
  Bell,
  ChartNoAxesCombined,
  CircleUserRound,
  Container,
  CreditCard,
  Fingerprint,
  FolderKanban,
  GitBranch,
  Inbox,
  LayoutDashboard,
  ScrollText,
  Server,
  Settings,
  Store,
  Users,
} from 'lucide-react'

export type CommandPaletteScope
  = | 'all'
    | 'application'
    | 'gateway_route'
    | 'deployment_target'
    | 'project'
    | 'navigation'

export const COMMAND_PALETTE_SCOPES: readonly CommandPaletteScope[] = [
  'all',
  'application',
  'gateway_route',
  'deployment_target',
  'project',
  'navigation',
]

export interface PageCommand {
  to: string
  label: string
  icon: LucideIcon
  keywords: string
  permission?: string
}

export interface RecentPaletteVisit {
  key: string
  kind: 'project' | 'application'
  id: string
  projectId?: string
  label: string
  sublabel?: string
  to: string
  visitedAt: number
}

const MAX_RECENT_ITEMS = 6
const RECENTS_STORAGE_PREFIX = 'luna.devops.commandPalette.recents.'

const INLINE_RESOURCE_PREFIXES = new Set([
  'proj',
  'project',
  'space',
  'app',
  'apps',
  'host',
  'domain',
  'route',
  'gateway',
  'deploy',
  'target',
  'pod',
  'image',
  'repo',
  'git',
  'vol',
  'volume',
  'pvc',
  'config',
  'env',
  'notify',
  'channel',
  'cluster',
  'k8s',
  'registry',
  'harbor',
  'build',
  'release',
  'user',
  'member',
])

// 与侧栏、顶栏收件箱和头像菜单入口一致；系统管理页仍由后端最终鉴权，这里仅按权限隐藏入口。
export function buildPageCommands(t: TFunction): PageCommand[] {
  return [
    {
      to: '/dashboard',
      label: t('dashboard'),
      icon: LayoutDashboard,
      keywords: 'dashboard home overview 工作台 仪表盘 概览 首页',
    },
    {
      to: '/projects',
      label: t('projects'),
      icon: FolderKanban,
      keywords: 'projects spaces workspace namespace 项目空间 工作区 命名空间',
    },
    {
      to: '/app-templates',
      label: t('appTemplates'),
      icon: Store,
      keywords: 'marketplace templates store helm addon 应用商城 模板 市场 组件',
    },
    {
      to: '/events',
      label: t('events'),
      icon: ScrollText,
      keywords: 'events audit activity timeline logs 事件 动态 审计 记录',
    },
    {
      to: '/inbox',
      label: t('inbox.title'),
      icon: Inbox,
      keywords: 'inbox messages approvals unread 收件箱 站内信 消息 审批 待办',
    },
    {
      to: '/code-repositories',
      label: t('codeRepositories'),
      icon: GitBranch,
      keywords: 'git github gitlab gitea repositories webhook 代码仓库 源码 仓库',
    },
    {
      to: '/registries',
      label: t('registries'),
      icon: Container,
      keywords: 'registries harbor dockerhub ghcr container images 制品仓库 镜像仓库 凭证',
    },
    {
      to: '/clusters',
      label: t('clusters'),
      icon: Server,
      keywords: 'clusters kubernetes k8s nodes pods workloads kubeconfig 集群 节点 资源',
    },
    {
      to: '/notification-channels',
      label: t('notificationChannels'),
      icon: Bell,
      keywords: 'notification channels webhook feishu lark dingtalk wecom slack 通知渠道 飞书 企微 钉钉',
    },
    {
      to: '/billing',
      label: t('billing'),
      icon: CreditCard,
      keywords: 'billing wallet credits usage spend invoice 账单 余额 额度 费用 消费',
    },
    {
      to: '/settings/account',
      label: t('account'),
      icon: CircleUserRound,
      keywords: 'account profile pat personal access token password oauth security 账号 个人访问令牌 令牌 密码 安全',
    },
    {
      to: '/settings/auth-providers',
      label: t('authProviders'),
      icon: Fingerprint,
      keywords: 'oidc oauth2 sso login registration identity 认证 登录 单点登录 注册',
      permission: 'user.manage',
    },
    {
      to: '/settings/users',
      label: t('users'),
      icon: Users,
      keywords: 'users members admins accounts roles 用户管理 账号 角色 管理员',
      permission: 'user.manage',
    },
    {
      to: '/settings/notifications',
      label: t('notifications'),
      icon: Bell,
      keywords: 'smtp mail email templates rules system notifications 邮件 通知设置 模板 规则',
      permission: 'user.manage',
    },
    {
      to: '/settings/operations',
      label: t('operationsDashboard'),
      icon: ChartNoAxesCombined,
      keywords: 'operations observability prometheus loki tempo metrics traces 运营面板 监控 指标 链路 可观测',
      permission: 'user.manage',
    },
    {
      to: '/settings/site',
      label: t('siteSettings'),
      icon: Settings,
      keywords: 'global site settings brand retention quota build environment 全局设置 站点 品牌 数据保留 配额',
      permission: 'user.manage',
    },
  ]
}

export function searchTypesForScope(scope: CommandPaletteScope): SearchResourceType[] | undefined {
  switch (scope) {
    case 'all':
      return undefined
    case 'application':
      return ['application']
    case 'gateway_route':
      return ['gateway_route']
    case 'deployment_target':
      return ['deployment_target', 'release', 'build_run']
    case 'project':
      return ['project', 'project_volume', 'runtime_config_set', 'build_variable_set']
    case 'navigation':
      return []
  }
}

export function hasInlineResourcePrefix(rawQuery: string): boolean {
  const trimmed = rawQuery.trim()
  const colonIdx = trimmed.indexOf(':')
  if (colonIdx <= 0)
    return false
  if (trimmed.slice(colonIdx + 1).startsWith('//'))
    return false
  const prefix = trimmed.slice(0, colonIdx).trim().toLowerCase()
  return INLINE_RESOURCE_PREFIXES.has(prefix)
}

export function matchesLocalSearchQuery(rawQuery: string, ...fields: Array<string | undefined>): boolean {
  const trimmed = rawQuery.trim().toLowerCase()
  if (!trimmed)
    return true
  const tokens = trimmed.split(/\s+/).filter(Boolean)
  if (tokens.length === 0)
    return true
  const haystack = fields
    .filter((field): field is string => Boolean(field))
    .join(' ')
    .toLowerCase()
  return tokens.every(token => haystack.includes(token))
}

export function resolveSearchResultPath(item: SearchResultItem, fallbackProjectId?: string): string {
  const projectId = item.projectId || fallbackProjectId || ''
  switch (item.type) {
    case 'project':
      return `/projects/${item.id}`
    case 'application':
      return projectId ? `/projects/${projectId}/apps/${item.id}` : '/projects'
    case 'deployment_target':
      return projectId && item.applicationId
        ? `/projects/${projectId}/apps/${item.applicationId}?tab=deployments#targetId=${encodeURIComponent(item.id)}`
        : '/projects'
    case 'gateway_route':
      return projectId && item.applicationId
        ? `/projects/${projectId}/apps/${item.applicationId}?tab=gateway`
        : '/projects'
    case 'repository_binding':
      return projectId && item.applicationId
        ? `/projects/${projectId}/apps/${item.applicationId}?tab=settings`
        : '/code-repositories'
    case 'project_volume':
      return projectId ? `/projects/${projectId}?tab=volumes` : '/projects'
    case 'runtime_config_set':
      return projectId ? `/projects/${projectId}?tab=shared-config&config=runtime-configs` : '/projects'
    case 'build_variable_set':
      return projectId ? `/projects/${projectId}?tab=shared-config` : '/projects'
    case 'notification_channel':
      return item.projectId
        ? `/projects/${item.projectId}?tab=notifications&subtab=channels`
        : '/notification-channels'
    case 'runtime_cluster':
      return '/clusters'
    case 'artifact_registry':
      return '/registries'
    case 'build_run':
      return projectId && item.applicationId
        ? `/projects/${projectId}/apps/${item.applicationId}?tab=builds#buildRunId=${encodeURIComponent(item.id)}`
        : '/projects'
    case 'release':
      return projectId && item.applicationId
        ? `/projects/${projectId}/apps/${item.applicationId}?tab=deployments#releaseId=${encodeURIComponent(item.id)}`
        : '/projects'
    case 'user':
      return '/settings/users'
  }
}

export function formatSearchResultContext(item: SearchResultItem): string {
  const parts: string[] = []
  if (item.projectName && item.type !== 'project')
    parts.push(item.projectName)
  if (item.applicationName && item.type !== 'application')
    parts.push(item.applicationName)
  if (item.subtitle)
    parts.push(item.subtitle)
  return parts.join(' · ')
}

export function readCommandPaletteRecents(userId?: string): RecentPaletteVisit[] {
  if (!userId || typeof window === 'undefined')
    return []
  try {
    const raw = window.localStorage.getItem(`${RECENTS_STORAGE_PREFIX}${userId}`)
    if (!raw)
      return []
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed))
      return []
    return parsed
      .filter((entry): entry is RecentPaletteVisit => (
        typeof entry === 'object'
        && entry !== null
        && typeof (entry as RecentPaletteVisit).key === 'string'
        && ((entry as RecentPaletteVisit).kind === 'project' || (entry as RecentPaletteVisit).kind === 'application')
        && typeof (entry as RecentPaletteVisit).id === 'string'
        && typeof (entry as RecentPaletteVisit).label === 'string'
        && typeof (entry as RecentPaletteVisit).to === 'string'
        && (entry as RecentPaletteVisit).to.startsWith('/projects/')
      ))
      .slice(0, MAX_RECENT_ITEMS)
  }
  catch {
    return []
  }
}

export function recordCommandPaletteRecent(userId: string | undefined, visit: Omit<RecentPaletteVisit, 'visitedAt'>) {
  if (!userId || typeof window === 'undefined' || !visit.to.startsWith('/projects/'))
    return
  try {
    const existing = readCommandPaletteRecents(userId).filter(item => item.key !== visit.key)
    const next: RecentPaletteVisit[] = [
      { ...visit, visitedAt: Date.now() },
      ...existing,
    ].slice(0, MAX_RECENT_ITEMS)
    window.localStorage.setItem(`${RECENTS_STORAGE_PREFIX}${userId}`, JSON.stringify(next))
  }
  catch {
    // 浏览器隐私模式或配额受限时静默降级，不阻塞导航。
  }
}
