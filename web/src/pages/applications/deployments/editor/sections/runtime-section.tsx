import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { DeploymentTarget, Release, RuntimeCluster } from '@/api'
import { Plus } from 'lucide-react'
import { Controller, useFormContext, useFormState, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { platformDefaultRuntimeCluster } from '@/lib/runtime-cluster-default'
import { emptyRuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'
import { EditorGroup, EditorSection } from '../deployment-target-editor-fields'
import { RuntimeDataVolumesEditor } from '../runtime/application-deployment-data-volumes-editor'
import { RuntimeEnvironmentFields } from '../runtime/runtime-environment-fields'
import { RuntimeScaleFields } from '../runtime/runtime-scale-fields'
import { ServicePortsFields } from '../runtime/service-ports-fields'

/** 运行与扩展：规格预设/HPA、服务端口、数据卷挂载、环境变量与共享配置集。 */
export function RuntimeSection({ canManageRuntimeSecrets, deploymentTargets, projectId, releases, runtimeClusters }: {
  canManageRuntimeSecrets: boolean
  deploymentTargets: DeploymentTarget[]
  projectId: string
  releases: Release[]
  runtimeClusters: RuntimeCluster[]
}) {
  const { t } = useTranslation()

  return (
    <EditorSection section="runtime" title={t('deploymentTargetEditor.sections.runtime')}>
      <RuntimeScaleFields runtimeClusters={runtimeClusters} />
      <ServicePortsFields />
      <RuntimeStorageFields projectId={projectId} runtimeClusters={runtimeClusters} />
      <RuntimeEnvironmentFields
        canManageRuntimeSecrets={canManageRuntimeSecrets}
        deploymentTargets={deploymentTargets}
        projectId={projectId}
        releases={releases}
      />
    </EditorSection>
  )
}

function RuntimeStorageFields({ projectId, runtimeClusters }: { projectId: string, runtimeClusters: RuntimeCluster[] }) {
  const { t } = useTranslation()
  const { control, getValues, setValue } = useFormContext<DeploymentTargetFormValues>()
  const clusterId = useWatch({ control, name: 'clusterId' })
  const { errors } = useFormState({ control, name: 'dataVolumes' })
  const effectiveClusterId = clusterId || platformDefaultRuntimeCluster(runtimeClusters)?.id || ''
  const rowErrors = Array.isArray(errors.dataVolumes)
    ? errors.dataVolumes.map(error => typeof error?.message === 'string' ? error.message : undefined)
    : []

  return (
    <EditorGroup
      description={t('deploymentsPage.dataVolumesHint')}
      title={t('deploymentsPage.dataVolumes')}
      tools={(
        <Button
          size="sm"
          type="button"
          variant="outline"
          onClick={() => {
            const current = getValues('dataVolumes')
            setValue('dataVolumes', [...current, emptyRuntimeDataVolumeRow(current.length)], { shouldDirty: true, shouldValidate: true })
          }}
        >
          <Plus className="size-4" />
          {t('deploymentsPage.addDataVolume')}
        </Button>
      )}
    >
      <Controller
        control={control}
        name="dataVolumes"
        render={({ field }) => (
          <RuntimeDataVolumesEditor
            hideAddButton
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
