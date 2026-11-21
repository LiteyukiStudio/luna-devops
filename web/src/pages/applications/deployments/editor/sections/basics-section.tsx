import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { RuntimeCluster } from '@/api'
import { Controller, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { RuntimeClusterPressureBadge } from '@/components/common/runtime-cluster-pressure'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { platformDefaultRuntimeCluster } from '@/lib/runtime-cluster-default'
import { useRuntimeClusterPressure } from '@/lib/runtime-cluster-pressure'
import { EditorSection, SwitchField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { isPublicDeploymentStage, publicDeploymentStages } from '../deployment-target-form'

const defaultClusterValue = '__default__'

/** 基础配置：名称、阶段、运行集群、工作负载类型与启用开关。 */
export function BasicsSection({ editing, projectId, runtimeClusters }: {
  editing: boolean
  projectId: string
  runtimeClusters: RuntimeCluster[]
}) {
  const { t } = useTranslation()
  const { control, getValues, register, setValue } = useFormContext<DeploymentTargetFormValues>()
  const nameError = useFieldError('name')
  const pressure = useRuntimeClusterPressure({ clusterIds: runtimeClusters.map(cluster => cluster.id), projectId })
  const defaultCluster = platformDefaultRuntimeCluster(runtimeClusters)
  const stageLabels = {
    dev: t('deploymentsPage.stageDev'),
    test: t('deploymentsPage.stageTest'),
    staging: t('deploymentsPage.stageStaging'),
    prod: t('deploymentsPage.stageProd'),
  }

  const clearProjectVolumeSelections = () => {
    // 项目数据卷按集群隔离，切换集群后原有卷选择不再有效。
    setValue('dataVolumes', getValues('dataVolumes').map(volume => volume.sourceType === 'projectVolume'
      ? { ...volume, devicePath: '', projectVolumeId: '' }
      : volume), { shouldDirty: true, shouldValidate: true })
  }

  return (
    <EditorSection
      section="basics"
      title={t('deploymentTargetEditor.sections.basics')}
      tools={(
        <SwitchField
          inline
          description={t('deploymentTargetEditor.enabledDescription')}
          label={t('deploymentTargetEditor.enabled')}
          name="enabled"
        />
      )}
    >
      <div className="grid gap-4 @md/workspace:grid-cols-2 @3xl/workspace:grid-cols-4">
        <FormField error={nameError} hint={t('deploymentsPage.deploymentConfigNameHint')} label={t('common.name')} required>
          <Input {...register('name')} autoComplete="off" placeholder={t('deploymentsPage.deploymentConfigNamePattern')} />
        </FormField>
        <FormField hint={editing ? t('deploymentTargetEditor.stageLockedHint') : undefined} label={t('deploymentsPage.stage')}>
          {fieldControl => (
            <Controller
              control={control}
              name="stage"
              render={({ field }) => (
                <Select disabled={editing} value={field.value || undefined} onValueChange={field.onChange}>
                  <SelectTrigger {...fieldControl} ref={field.ref} className="w-full" onBlur={field.onBlur}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Boolean(field.value) && !isPublicDeploymentStage(field.value) && (
                      <SelectItem value={field.value}>{t('deploymentsPage.stageSystemManaged', { stage: field.value })}</SelectItem>
                    )}
                    {publicDeploymentStages.map(item => <SelectItem key={item} value={item}>{stageLabels[item]}</SelectItem>)}
                  </SelectContent>
                </Select>
              )}
            />
          )}
        </FormField>
        <FormField hint={t('deploymentsPage.runtimeEnvironmentHint')} label={t('clustersPage.runtimeCluster')}>
          {fieldControl => (
            <Controller
              control={control}
              name="clusterId"
              render={({ field }) => (
                <Select
                  value={field.value || defaultClusterValue}
                  onValueChange={(value) => {
                    clearProjectVolumeSelections()
                    field.onChange(value === defaultClusterValue ? '' : value)
                  }}
                >
                  <SelectTrigger {...fieldControl} className="w-full">
                    <SelectValue placeholder={t('common.select')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={defaultClusterValue}>
                      <span>{defaultCluster ? t('deploymentsPage.clusterDefaultOption', { name: defaultCluster.name }) : t('deploymentTargetEditor.platformDefaultCluster')}</span>
                      {defaultCluster && <RuntimeClusterPressureBadge loading={pressure.isPending} pressure={pressure.byClusterId[defaultCluster.id]} />}
                    </SelectItem>
                    {runtimeClusters.map(cluster => (
                      <SelectItem key={cluster.id} value={cluster.id}>
                        <span>{cluster.name}</span>
                        <RuntimeClusterPressureBadge loading={pressure.isPending} pressure={pressure.byClusterId[cluster.id]} />
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          )}
        </FormField>
        <FormField hint={t('deploymentsPage.workloadTypeHint')} label={t('deploymentsPage.workloadType')}>
          {fieldControl => (
            <Controller
              control={control}
              name="workloadType"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger {...fieldControl} ref={field.ref} className="w-full" onBlur={field.onBlur}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="Deployment">{t('deploymentsPage.kubernetesValues.Deployment')}</SelectItem>
                    <SelectItem value="StatefulSet">{t('deploymentsPage.kubernetesValues.StatefulSet')}</SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
          )}
        </FormField>
      </div>
    </EditorSection>
  )
}
