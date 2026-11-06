import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { DeploymentTarget, Release, RuntimeCluster } from '@/api'
import { Controller, useFormContext, useFormState, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { platformDefaultRuntimeCluster } from '@/lib/runtime-cluster-default'
import { EditorGroup, EditorSection } from '../deployment-target-editor-fields'
import { RuntimeDataVolumesEditor } from '../runtime/application-deployment-data-volumes-editor'
import { RuntimeEnvironmentFields } from '../runtime/runtime-environment-fields'
import { RuntimeScaleFields } from '../runtime/runtime-scale-fields'
import { ServicePortsFields } from '../runtime/service-ports-fields'

/** 运行：规格与副本、服务端口、环境变量和数据卷；Kubernetes 原生选项在高级分区。 */
export function RuntimeSection({ canManageRuntimeSecrets, deploymentTargets, projectId, releases, runtimeClusters }: {
  canManageRuntimeSecrets: boolean
  deploymentTargets: DeploymentTarget[]
  projectId: string
  releases: Release[]
  runtimeClusters: RuntimeCluster[]
}) {
  const { t } = useTranslation()

  return (
    <EditorSection description={t('deploymentTargetEditor.sections.runtimeDescription')} section="runtime" title={t('deploymentTargetEditor.sections.runtime')}>
      <RuntimeScaleFields runtimeClusters={runtimeClusters} />
      <ServicePortsFields />
      <RuntimeEnvironmentFields
        canManageRuntimeSecrets={canManageRuntimeSecrets}
        deploymentTargets={deploymentTargets}
        projectId={projectId}
        releases={releases}
      />
      <RuntimeStorageFields projectId={projectId} runtimeClusters={runtimeClusters} />
    </EditorSection>
  )
}

function RuntimeStorageFields({ projectId, runtimeClusters }: { projectId: string, runtimeClusters: RuntimeCluster[] }) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const clusterId = useWatch({ control, name: 'clusterId' })
  const { errors } = useFormState({ control, name: 'dataVolumes' })
  const effectiveClusterId = clusterId || platformDefaultRuntimeCluster(runtimeClusters)?.id || ''
  const rowErrors = Array.isArray(errors.dataVolumes)
    ? errors.dataVolumes.map(error => typeof error?.message === 'string' ? error.message : undefined)
    : []

  return (
    <EditorGroup description={t('deploymentsPage.dataVolumesHint')} title={t('deploymentsPage.dataVolumes')}>
      <Controller
        control={control}
        name="dataVolumes"
        render={({ field }) => (
          <RuntimeDataVolumesEditor
            clusterId={effectiveClusterId}
            projectId={projectId}
            rowErrors={rowErrors}
            rows={field.value}
            runtimeClusters={runtimeClusters}
            onChange={field.onChange}
          />
        )}
      />
    </EditorGroup>
  )
}
