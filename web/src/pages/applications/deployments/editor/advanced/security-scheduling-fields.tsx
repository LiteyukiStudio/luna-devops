import type { DeploymentTargetFormValues } from '../deployment-target-form'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { JsonField, SwitchField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'

/** 安全上下文与调度：Pod 安全设置、节点选择、容忍、亲和与拓扑分布。 */
export function SecuritySchedulingFields() {
  const { t } = useTranslation()
  const { register } = useFormContext<DeploymentTargetFormValues>()
  const runAsUserError = useFieldError('runAsUser')
  const runAsGroupError = useFieldError('runAsGroup')
  const fsGroupError = useFieldError('fsGroup')
  const capabilityDropError = useFieldError('capabilityDrop')
  const nodeSelectorError = useFieldError('nodeSelector')

  return (
    <>
      <div className="grid gap-4">
        <h4 className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedSecurity')}</h4>
        <div className="grid gap-4 @xl/workspace:grid-cols-3">
          <FormField error={runAsUserError} hint={t('deploymentsPage.runAsUserHint')} label={t('deploymentsPage.runAsUser')}>
            <Input {...register('runAsUser')} inputMode="numeric" />
          </FormField>
          <FormField error={runAsGroupError} hint={t('deploymentsPage.runAsGroupHint')} label={t('deploymentsPage.runAsGroup')}>
            <Input {...register('runAsGroup')} inputMode="numeric" />
          </FormField>
          <FormField error={fsGroupError} hint={t('deploymentsPage.fsGroupHint')} label={t('deploymentsPage.fsGroup')}>
            <Input {...register('fsGroup')} inputMode="numeric" />
          </FormField>
          <FormField hint={t('deploymentsPage.fsGroupChangePolicyHint')} label={t('deploymentsPage.fsGroupChangePolicy')}>
            <NativeSelect {...register('fsGroupChangePolicy')}>
              <option value="">{t('deploymentsPage.kubernetesDefault')}</option>
              <option value="OnRootMismatch">{t('deploymentsPage.kubernetesValues.OnRootMismatch')}</option>
              <option value="Always">{t('deploymentsPage.kubernetesValues.Always')}</option>
            </NativeSelect>
          </FormField>
          <FormField error={capabilityDropError} hint={t('deploymentsPage.capabilityDropHint')} label={t('deploymentsPage.capabilityDrop')}>
            <Input {...register('capabilityDrop')} className="font-mono" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.capabilityDropPlaceholder') })} />
          </FormField>
        </div>
        <SwitchField description={t('deploymentsPage.readOnlyRootFilesystemHint')} label={t('deploymentsPage.readOnlyRootFilesystem')} name="readOnlyRootFilesystem" />
      </div>
      <div className="grid gap-4">
        <h4 className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedScheduling')}</h4>
        <div className="grid gap-4 @xl/workspace:grid-cols-2">
          <FormField hint={t('deploymentsPage.priorityClassNameHint')} label={t('deploymentsPage.priorityClassName')}>
            <Input {...register('priorityClassName')} placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.priorityClassNamePlaceholder') })} />
          </FormField>
          <FormField error={nodeSelectorError} hint={t('deploymentsPage.nodeSelectorHint')} label={t('deploymentsPage.nodeSelector')}>
            <Textarea {...register('nodeSelector')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.nodeSelectorPlaceholder') })} spellCheck={false} />
          </FormField>
        </div>
        <JsonField hint={t('deploymentsPage.tolerationsHint')} label={t('deploymentsPage.tolerations')} name="tolerations" placeholder={t('deploymentsPage.tolerationsPlaceholder')} />
        <JsonField hint={t('deploymentsPage.affinityHint')} label={t('deploymentsPage.affinity')} name="affinity" placeholder={t('deploymentsPage.affinityPlaceholder')} />
        <JsonField hint={t('deploymentsPage.topologySpreadConstraintsHint')} label={t('deploymentsPage.topologySpreadConstraints')} name="topologySpreadConstraints" placeholder={t('deploymentsPage.topologySpreadConstraintsPlaceholder')} />
      </div>
    </>
  )
}

/** 初始化容器与辅助容器。 */
export function AuxContainersFields() {
  const { t } = useTranslation()
  return (
    <>
      <JsonField hint={t('deploymentsPage.initContainersHint')} label={t('deploymentsPage.initContainers')} name="initContainers" placeholder={t('deploymentsPage.auxContainersPlaceholder')} />
      <JsonField hint={t('deploymentsPage.sidecarContainersHint')} label={t('deploymentsPage.sidecarContainers')} name="sidecarContainers" placeholder={t('deploymentsPage.auxContainersPlaceholder')} />
    </>
  )
}
