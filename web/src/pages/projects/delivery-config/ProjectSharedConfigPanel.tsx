import type { ProjectBuildVariableSetsPageHandle } from './ProjectBuildVariableSetsPage'
import type { ProjectHooksPageHandle } from './ProjectHooksPage'
import type { ProjectRuntimeConfigSetsPageHandle } from './ProjectRuntimeConfigSetsPage'
import { FileCode2, KeyRound, ScrollText } from 'lucide-react'
import { lazy, Suspense, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { ContentTabs } from '@/components/common/content-tabs'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { Button } from '@/components/ui/button'
import { TabsContent } from '@/components/ui/tabs'
import { normalizeProjectSharedConfigTab, projectSharedConfigTabParam, projectSharedConfigTabs } from '@/pages/projects/overview/project-workspace-sections'

const ProjectBuildVariableSetsPage = lazy(() => import('./ProjectBuildVariableSetsPage').then(module => ({ default: module.ProjectBuildVariableSetsPage })))
const ProjectHooksPage = lazy(() => import('./ProjectHooksPage').then(module => ({ default: module.ProjectHooksPage })))
const ProjectRuntimeConfigSetsPage = lazy(() => import('./ProjectRuntimeConfigSetsPage').then(module => ({ default: module.ProjectRuntimeConfigSetsPage })))

/** 项目空间“共享配置”分区：构建变量、运行配置和钩子都由多个应用的部署配置引用，在同一分区内用 Tab 切换。 */
export function ProjectSharedConfigPanel({ projectId }: { projectId: string }) {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  // Tab 只以 URL 为来源，可刷新、可分享。
  const activeTab = normalizeProjectSharedConfigTab(searchParams.get(projectSharedConfigTabParam))
  const buildVariableSetsPageRef = useRef<ProjectBuildVariableSetsPageHandle>(null)
  const hooksPageRef = useRef<ProjectHooksPageHandle>(null)
  const runtimeConfigSetsPageRef = useRef<ProjectRuntimeConfigSetsPageHandle>(null)

  const changeTab = (value: string) => {
    const tab = normalizeProjectSharedConfigTab(value)
    setSearchParams((current) => {
      const next = new URLSearchParams(current)
      if (tab === projectSharedConfigTabs[0])
        next.delete(projectSharedConfigTabParam)
      else
        next.set(projectSharedConfigTabParam, tab)
      return next
    }, { replace: true })
  }

  const tools = activeTab === 'build-variables'
    ? (
        <Button type="button" onClick={() => buildVariableSetsPageRef.current?.openCreateDialog()}>
          <KeyRound size={16} />
          {t('buildsPage.createVariableSet')}
        </Button>
      )
    : activeTab === 'runtime-configs'
      ? (
          <Button type="button" onClick={() => runtimeConfigSetsPageRef.current?.openCreateDialog()}>
            <FileCode2 size={16} />
            {t('runtimeConfigSets.createTitle')}
          </Button>
        )
      : (
          <Button type="button" onClick={() => hooksPageRef.current?.openCreateDialog()}>
            <ScrollText size={16} />
            {t('projectHooks.createTitle')}
          </Button>
        )

  return (
    <ContentTabs
      hashRouting={false}
      tabs={[
        { label: t('nav.projectSections.buildVariables'), value: 'build-variables' },
        { label: t('nav.projectSections.runtimeConfigs'), value: 'runtime-configs' },
        { label: t('nav.projectSections.hooks'), value: 'hooks' },
      ]}
      tools={tools}
      value={activeTab}
      onValueChange={changeTab}
    >
      <Suspense fallback={<ToolViewportSkeleton />}>
        <TabsContent value="build-variables">
          <ProjectBuildVariableSetsPage ref={buildVariableSetsPageRef} projectId={projectId} />
        </TabsContent>
        <TabsContent value="runtime-configs">
          <ProjectRuntimeConfigSetsPage ref={runtimeConfigSetsPageRef} projectId={projectId} />
        </TabsContent>
        <TabsContent value="hooks">
          <ProjectHooksPage ref={hooksPageRef} projectId={projectId} />
        </TabsContent>
      </Suspense>
    </ContentTabs>
  )
}
