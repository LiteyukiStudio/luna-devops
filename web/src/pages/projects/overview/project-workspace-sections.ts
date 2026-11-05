import type { LucideIcon } from 'lucide-react'
import { Boxes, FileCode2, HardDrive, KeyRound, LayoutDashboard, Network, ScrollText, Users } from 'lucide-react'

export type ProjectWorkspaceSection = 'apps' | 'overview' | 'build-variables' | 'runtime-configs' | 'hooks' | 'volumes' | 'members' | 'topology'

/**
 * 项目空间工作区的分区。侧栏导航和工作区页面共用这一份定义；分区由 URL `?tab=` 表达，缺省为应用列表。
 */
export const projectWorkspaceSections: { icon: LucideIcon, labelKey: string, value: ProjectWorkspaceSection }[] = [
  { icon: Boxes, labelKey: 'nav.projectSections.apps', value: 'apps' },
  { icon: LayoutDashboard, labelKey: 'nav.projectSections.overview', value: 'overview' },
  { icon: KeyRound, labelKey: 'nav.projectSections.buildVariables', value: 'build-variables' },
  { icon: FileCode2, labelKey: 'nav.projectSections.runtimeConfigs', value: 'runtime-configs' },
  { icon: ScrollText, labelKey: 'nav.projectSections.hooks', value: 'hooks' },
  { icon: HardDrive, labelKey: 'nav.projectSections.volumes', value: 'volumes' },
  { icon: Users, labelKey: 'nav.projectSections.members', value: 'members' },
  { icon: Network, labelKey: 'nav.projectSections.topology', value: 'topology' },
]

export const defaultProjectWorkspaceSection: ProjectWorkspaceSection = 'apps'

export function normalizeProjectWorkspaceSection(value: string | null): ProjectWorkspaceSection {
  return projectWorkspaceSections.find(section => section.value === value)?.value ?? defaultProjectWorkspaceSection
}

export function projectWorkspaceSectionPath(projectId: string, section: ProjectWorkspaceSection) {
  const base = `/projects/${encodeURIComponent(projectId)}`
  return section === defaultProjectWorkspaceSection ? base : `${base}?tab=${section}`
}
