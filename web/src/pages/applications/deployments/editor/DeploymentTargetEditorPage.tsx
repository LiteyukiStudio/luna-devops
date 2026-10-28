import type { DeploymentTargetEditorProps } from './deployment-target-editor'
import type { DeploymentTargetFormValues } from './deployment-target-form'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ErrorState } from '@/components/common/error-state'
import { SettingsSkeleton } from '@/components/common/loading-states'
import { PageShell } from '@/components/common/page-shell'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { isPlatformAdmin, ProjectRole } from '@/lib/roles'
import { defaultTargetImageRef } from '@/pages/applications/application-config-utils'
import { DeploymentTargetEditor } from './deployment-target-editor'
import { deploymentTargetToFormValues, emptyDeploymentTargetFormValues } from './deployment-target-form'
import { dockerfileBuildDefaults, repositoryBuildOptionsQuery, targetImageTemplateQuery } from './deployment-target-queries'

const runtimeSecretManagerRoles: readonly string[] = [ProjectRole.Owner, ProjectRole.Admin, ProjectRole.Developer]

/** 部署配置独立编辑页：/projects/:projectId/apps/:applicationId/deployments/new 与 /:targetId/edit。 */
export function DeploymentTargetEditorPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const { applicationId = '', projectId = '', targetId } = useParams()
  const editing = Boolean(targetId)
  const ids = Boolean(projectId && applicationId)
  const application = useQuery({ queryKey: ['application', projectId, applicationId], queryFn: () => api.getApplication(projectId, applicationId), enabled: ids })
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => api.getProject(projectId), enabled: Boolean(projectId) })
  const registries = useQuery({ ...liveObservationQueryPolicy, queryKey: ['registries', projectId], queryFn: () => api.listRegistries(projectId), enabled: Boolean(projectId) })
  const repositoryBindings = useQuery({ ...liveObservationQueryPolicy, queryKey: ['repository-bindings', projectId, applicationId], queryFn: () => api.listRepositoryBindings(projectId, applicationId), enabled: ids })
  const deploymentTargets = useQuery({ ...liveObservationQueryPolicy, queryKey: ['deployment-targets', projectId, applicationId], queryFn: () => api.listDeploymentTargets(projectId, applicationId), enabled: ids })
  const releases = useQuery({ queryKey: ['releases', projectId, applicationId], queryFn: () => api.listReleases(projectId, applicationId), enabled: ids })
  const runtimeClusters = useQuery({ ...liveObservationQueryPolicy, queryKey: ['runtime-clusters', projectId], queryFn: () => api.listRuntimeClusters(projectId), enabled: Boolean(projectId) })
  const buildEnvironment = useQuery({
    queryKey: ['build-environment-config', 'deployment', projectId, applicationId, targetId],
    queryFn: () => api.getBuildEnvironmentConfig({ scope: 'deployment', projectId, applicationId, deploymentTargetId: targetId }),
    enabled: ids && editing,
    retry: false,
  })

  const bindings = (repositoryBindings.data ?? []).filter(binding => binding.applicationId === applicationId)
  const target = targetId ? deploymentTargets.data?.find(item => item.id === targetId) ?? null : null
  const defaultRegistry = registries.data?.find(registry => registry.credentialSet && registry.isDefault)
    ?? registries.data?.find(registry => registry.credentialSet)
    ?? registries.data?.find(registry => registry.isDefault)
    ?? registries.data?.[0]
  const defaultBinding = bindings[0]
  // 新建时预取仓库结构和镜像模板，初始值一次成型，避免挂载后再用 Effect 回填。
  const initialBuildOptions = useQuery({ ...repositoryBuildOptionsQuery(defaultBinding), enabled: !editing && Boolean(repositoryBuildOptionsQuery(defaultBinding).enabled), retry: false })
  const initialImageTemplate = useQuery({
    ...targetImageTemplateQuery({ applicationId, projectId, registryId: defaultRegistry?.id ?? '', stage: 'dev', targetName: '' }),
    enabled: !editing && Boolean(defaultRegistry?.id && ids),
    retry: false,
  })

  const required = [application, project, registries, repositoryBindings, deploymentTargets, releases, runtimeClusters]
  const title = editing ? t('deploymentsPage.editDeploymentTarget') : t('deploymentsPage.createDeploymentTarget')
  if (required.some(query => query.isError))
    return <ErrorState description={t('deploymentTargetEditor.loadFailedDescription')} title={t('deploymentTargetEditor.loadFailedTitle')} />
  const initialQueriesSettled = editing
    ? !buildEnvironment.isPending
    : (initialBuildOptions.fetchStatus === 'idle' && initialImageTemplate.fetchStatus === 'idle')
  if (required.some(query => query.isPending) || !initialQueriesSettled) {
    return (
      <PageShell width="content">
        <PageHeading description={t('deploymentTargetEditor.pageDescription')} title={title} />
        <SettingsSkeleton />
      </PageShell>
    )
  }
  if (editing && !target)
    return <ErrorState description={t('deploymentTargetEditor.notFoundDescription')} title={t('deploymentTargetEditor.notFoundTitle')} />

  const buildOptions = initialBuildOptions.data
  const detectedDockerfile = buildOptions?.dockerfiles?.[0]
  const newTargetDefaults = detectedDockerfile
    ? dockerfileBuildDefaults(detectedDockerfile, buildOptions.directories ?? [], buildOptions.exposedPorts ?? {})
    : {}

  return (
    <PageShell width="content">
      <PageHeading description={t('deploymentTargetEditor.pageDescription')} title={target ? `${title} · ${target.name}` : title} />
      <ReadyEditor
        key={targetId ?? 'new'}
        applicationId={applicationId}
        buildEnvironmentAvailable={!editing || buildEnvironment.isSuccess}
        canManageRuntimeSecrets={isPlatformAdmin(user?.role) || runtimeSecretManagerRoles.includes(project.data?.currentUserRole ?? '')}
        createInitialValues={() => target
          ? deploymentTargetToFormValues(target, buildEnvironment.data)
          : {
              ...emptyDeploymentTargetFormValues(),
              ...newTargetDefaults,
              clusterId: (runtimeClusters.data ?? []).find(cluster => cluster.isDefault)?.id ?? runtimeClusters.data?.[0]?.id ?? '',
              repositoryBindingId: defaultBinding?.id ?? '',
              targetRegistryId: defaultRegistry?.id ?? '',
              targetImageRef: initialImageTemplate.data?.targetImageRef
                || defaultTargetImageRef(defaultRegistry, project.data?.identifier ?? '', application.data?.identifier ?? ''),
            }}
        deploymentTargets={deploymentTargets.data ?? []}
        projectId={projectId}
        registries={registries.data ?? []}
        releases={releases.data ?? []}
        repositoryBindings={bindings}
        runtimeClusters={runtimeClusters.data ?? []}
        target={target}
      />
    </PageShell>
  )
}

/** 数据就绪后一次性建立表单基线；切换目标时由 key 重新挂载。 */
function ReadyEditor({ createInitialValues, ...props }: Omit<DeploymentTargetEditorProps, 'initialValues'> & {
  createInitialValues: () => DeploymentTargetFormValues
}) {
  const [initialValues] = useState(createInitialValues)
  return <DeploymentTargetEditor {...props} initialValues={initialValues} />
}

function PageHeading({ description, title }: { description: string, title: string }) {
  return (
    <header className="grid gap-1">
      <h1 className="text-xl font-semibold tracking-tight text-foreground">{title}</h1>
      <p className="max-w-3xl text-sm text-muted-foreground">{description}</p>
    </header>
  )
}
