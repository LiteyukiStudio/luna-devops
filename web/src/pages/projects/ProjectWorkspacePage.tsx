import type { ApplicationsPageHandle } from '@/pages/applications/ApplicationsPage'
import type { ProjectMembersPageHandle } from '@/pages/projects/ProjectMembersPage'
import { useQuery } from '@tanstack/react-query'
import { Plus, UserPlus } from 'lucide-react'
import { motion } from 'motion/react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { api } from '@/api/client'
import { ContentTabs } from '@/components/common/content-tabs'
import { ErrorState } from '@/components/common/error-state'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { TabsContent } from '@/components/ui/tabs'
import { ApplicationsPage } from '@/pages/applications/ApplicationsPage'
import { ProjectMembersPage } from '@/pages/projects/ProjectMembersPage'

export function ProjectWorkspacePage() {
  const { t } = useTranslation()
  const { projectId = '' } = useParams()
  const [activeTab, setActiveTab] = useState('overview')
  const applicationsPageRef = useRef<ApplicationsPageHandle>(null)
  const membersPageRef = useRef<ProjectMembersPageHandle>(null)
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => api.getProject(projectId), enabled: Boolean(projectId) })
  const applications = useQuery({ queryKey: ['applications', projectId], queryFn: () => api.listApplications(projectId), enabled: Boolean(projectId) })
  const members = useQuery({ queryKey: ['project-members', projectId], queryFn: () => api.listProjectMembers(projectId), enabled: Boolean(projectId) })

  if (project.isError)
    return <ErrorState title={t('projectSpaces.workspaceLoadFailedTitle')} description={t('projectSpaces.workspaceLoadFailedDescription')} />

  const currentProject = project.data
  const activeContent = (() => {
    switch (activeTab) {
      case 'apps':
        return <ApplicationsPage ref={applicationsPageRef} embedded projectId={projectId} />
      case 'members':
        return <ProjectMembersPage ref={membersPageRef} embedded projectId={projectId} />
      default:
        return (
          <Card className="grid gap-3">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <h2 className="truncate text-lg font-semibold">{currentProject?.name ?? t('projectSpaces.title')}</h2>
                <p className="text-sm text-muted-foreground">{currentProject?.description || t('common.noDescription')}</p>
              </div>
              <StatusBadge>{currentProject?.namespaceStrategy === 'project' ? t('projectSpaces.namespaceProject') : currentProject?.namespaceStrategy ?? t('projectSpaces.namespaceProject')}</StatusBadge>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <ProjectMetric label={t('projectSpaces.apps')} value={applications.data?.length ?? 0} />
              <ProjectMetric label={t('projectSpaces.members')} value={members.data?.length ?? 0} />
            </div>
          </Card>
        )
    }
  })()

  const contentTools = (() => {
    if (activeTab === 'apps') {
      return (
        <Button type="button" onClick={() => applicationsPageRef.current?.openCreateDialog()}>
          <Plus size={16} />
          {t('apps.createTitle')}
        </Button>
      )
    }

    if (activeTab === 'members') {
      return (
        <Button type="button" onClick={() => membersPageRef.current?.openAddMemberDialog()}>
          <UserPlus size={16} />
          {t('projectMembers.addTitle')}
        </Button>
      )
    }

    return null
  })()

  return (
    <div className="grid gap-6">
      <ContentTabs
        tabs={[
          { value: 'overview', label: t('projectSpaces.overviewTab') },
          { value: 'apps', label: t('projectSpaces.apps') },
          { value: 'members', label: t('projectSpaces.members') },
        ]}
        tools={contentTools}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <TabsContent value={activeTab}>
          <motion.div
            key={`${projectId}-${activeTab}`}
            animate={{ opacity: 1, y: 0 }}
            initial={{ opacity: 0, y: 6 }}
            transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
          >
            {activeContent}
          </motion.div>
        </TabsContent>
      </ContentTabs>
    </div>
  )
}

function ProjectMetric({ label, value }: { label: string, value: number }) {
  return (
    <div className="rounded-md border border-border bg-background p-3">
      <p className="text-sm text-muted-foreground">{label}</p>
      <p className="mt-2 text-2xl font-semibold">{value}</p>
    </div>
  )
}
