import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { RuntimeCluster } from '@/api'
import { Controller, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { RuntimeClusterPressureBadge } from '@/components/common/runtime-cluster-pressure'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useRuntimeClusterPressure } from '@/lib/runtime-cluster-pressure'
import { EditorSection, SwitchField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { isPublicDeploymentStage, publicDeploymentStages } from '../deployment-target-form'

const defaultClusterValue = '__default__'

export function BasicsSection({ editing, projectId, runtimeClusters }: {
  editing: boolean
  projectId: string
  runtimeClusters: RuntimeCluster[]
}) {
  const { t } = useTranslation()
  const { control, getValues, register, setValue, watch } = useFormContext<DeploymentTargetFormValues>()
  const stage = watch('stage')
  const nameError = useFieldError('name')
  const pressure = useRuntimeClusterPressure({ clusterIds: runtimeClusters.map(cluster => cluster.id), projectId })
  const defaultCluster = runtimeClusters.find(cluster => cluster.isDefault) ?? runtimeClusters[0]
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
    <EditorSection description={t('deploymentTargetEditor.sections.basicsDescription')} section="basics" title={t('deploymentTargetEditor.sections.basics')}>
      <div className="grid gap-4 md:grid-cols-2">
        <FormField error={nameError} hint={t('deploymentsPage.deploymentConfigNameHint')} label={t('common.name')} required>
          <Input {...register('name')} autoComplete="off" placeholder={t('deploymentsPage.deploymentConfigNamePattern')} />
        </FormField>
        <FormField hint={editing ? t('deploymentTargetEditor.stageLockedHint') : undefined} label={t('deploymentsPage.stage')}>
          <NativeSelect {...register('stage')} disabled={editing}>
            {!isPublicDeploymentStage(stage) && (
              <option value={stage}>{t('deploymentsPage.stageSystemManaged', { stage })}</option>
            )}
            {publicDeploymentStages.map(item => <option key={item} value={item}>{stageLabels[item]}</option>)}
          </NativeSelect>
        </FormField>
        <FormField className="md:col-span-2" hint={t('deploymentsPage.runtimeEnvironmentHint')} label={t('clustersPage.runtimeCluster')}>
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
        <div className="md:col-span-2">
          <SwitchField description={t('deploymentTargetEditor.enabledDescription')} label={t('deploymentTargetEditor.enabled')} name="enabled" />
        </div>
      </div>
    </EditorSection>
  )
}
