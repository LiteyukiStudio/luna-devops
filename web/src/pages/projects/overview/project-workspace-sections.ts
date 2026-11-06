import type { LucideIcon } from 'lucide-react'
import { Boxes, HardDrive, LayoutDashboard, Network, SlidersHorizontal, Users } from 'lucide-react'

export type ProjectWorkspaceSection = 'overview' | 'apps' | 'shared-config' | 'volumes' | 'members' | 'topology'

/** “共享配置”分区内的 Tab；值沿用原独立分区的 `?tab=`，旧链接可一一映射。 */
export type ProjectSharedConfigTab = 'build-variables' | 'runtime-configs' | 'hooks'

/**
 * 项目空间工作区的分区。侧栏导航、命令面板和工作区页面共用这一份定义；分区由 URL `?tab=` 表达，缺省为概览。
 */
export const projectWorkspaceSections: { icon: LucideIcon, labelKey: string, value: ProjectWorkspaceSection }[] = [
  { icon: LayoutDashboard, labelKey: 'nav.projectSections.overview', value: 'overview' },
  { icon: Boxes, labelKey: 'nav.projectSections.apps', value: 'apps' },
  { icon: SlidersHorizontal, labelKey: 'nav.projectSections.sharedConfig', value: 'shared-config' },
  { icon: HardDrive, labelKey: 'nav.projectSections.volumes', value: 'volumes' },
  { icon: Users, labelKey: 'nav.projectSections.members', value: 'members' },
  { icon: Network, labelKey: 'nav.projectSections.topology', value: 'topology' },
]

export const projectSharedConfigTabs: readonly ProjectSharedConfigTab[] = ['build-variables', 'runtime-configs', 'hooks']

export const defaultProjectWorkspaceSection: ProjectWorkspaceSection = 'overview'
const defaultProjectSharedConfigTab: ProjectSharedConfigTab = 'build-variables'
/** 共享配置分区内 Tab 的 URL 参数名。 */
export const projectSharedConfigTabParam = 'config'

function sharedConfigTabOf(value: string | null) {
  return projectSharedConfigTabs.find(tab => tab === value)
}

export function normalizeProjectWorkspaceSection(value: string | null): ProjectWorkspaceSection {
  if (sharedConfigTabOf(value))
    return 'shared-config'
  return projectWorkspaceSections.find(section => section.value === value)?.value ?? defaultProjectWorkspaceSection
}

export function normalizeProjectSharedConfigTab(value: string | null): ProjectSharedConfigTab {
  return sharedConfigTabOf(value) ?? defaultProjectSharedConfigTab
}

/**
 * 构建变量、运行配置和钩子曾是独立分区（`?tab=build-variables` 等）。
 * 旧链接返回新分区对应 Tab 的查询串，其余参数保持不变；不是旧链接时返回 null。
 */
export function legacyProjectWorkspaceSearch(searchParams: URLSearchParams): string | null {
  const legacyTab = sharedConfigTabOf(searchParams.get('tab'))
  if (!legacyTab)
    return null
  const next = new URLSearchParams(searchParams)
  next.set('tab', 'shared-config')
  if (legacyTab === defaultProjectSharedConfigTab)
    next.delete(projectSharedConfigTabParam)
  else
    next.set(projectSharedConfigTabParam, legacyTab)
  return `?${next.toString()}`
}

export function projectWorkspaceSectionPath(projectId: string, section: ProjectWorkspaceSection) {
  const base = `/projects/${encodeURIComponent(projectId)}`
  return section === defaultProjectWorkspaceSection ? base : `${base}?tab=${section}`
}
