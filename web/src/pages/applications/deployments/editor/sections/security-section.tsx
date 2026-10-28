import type { DeploymentTargetFormValues } from '../deployment-target-form'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { AdvancedOptions, EditorGroup, EditorSection, JsonField, SwitchField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { changedFieldCount, deploymentTargetSectionFields } from '../deployment-target-form'

const advancedSecurityFields = deploymentTargetSectionFields.security.filter(field => field !== 'webConsoleEnabled')

/** 安全与调度：Web Console 是平台访问策略，始终可见；Pod 安全、调度和辅助容器属于低频高级项。 */
export function SecuritySection() {
  const { t } = useTranslation()
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const values = useWatch({ control, name: advancedSecurityFields })
  const configuredCount = changedFieldCount(Object.fromEntries(advancedSecurityFields.map((field, index) => [field, values[index]])), advancedSecurityFields)
  const runAsUserError = useFieldError('runAsUser')
  const runAsGroupError = useFieldError('runAsGroup')
  const fsGroupError = useFieldError('fsGroup')
  const capabilityDropError = useFieldError('capabilityDrop')
  const nodeSelectorError = useFieldError('nodeSelector')

  return (
    <EditorSection description={t('deploymentTargetEditor.sections.securityDescription')} section="security" title={t('deploymentTargetEditor.sections.security')}>
      <EditorGroup title={t('deploymentsPage.webConsole')}>
        <FormField hint={t('deploymentsPage.webConsoleOverrideHint')} label={t('deploymentsPage.webConsoleOverride')}>
          <NativeSelect {...register('webConsoleEnabled')}>
            <option value="inherit">{t('deploymentsPage.webConsoleInheritProject')}</option>
            <option value="disabled">{t('deploymentsPage.webConsoleDisableForTarget')}</option>
          </NativeSelect>
        </FormField>
      </EditorGroup>
      <EditorGroup title={t('deploymentTargetEditor.kubernetesAdvancedTitle')}>
        <AdvancedOptions configuredCount={configuredCount} fields={advancedSecurityFields} title={t('deploymentTargetEditor.kubernetesAdvancedToggle')}>
          <div className="grid gap-4">
            <h4 className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedSecurity')}</h4>
            <div className="grid gap-4 md:grid-cols-3">
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
            <div className="grid gap-4 md:grid-cols-2">
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
          <div className="grid gap-4">
            <h4 className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedMultiContainer')}</h4>
            <JsonField hint={t('deploymentsPage.initContainersHint')} label={t('deploymentsPage.initContainers')} name="initContainers" placeholder={t('deploymentsPage.auxContainersPlaceholder')} />
            <JsonField hint={t('deploymentsPage.sidecarContainersHint')} label={t('deploymentsPage.sidecarContainers')} name="sidecarContainers" placeholder={t('deploymentsPage.auxContainersPlaceholder')} />
          </div>
        </AdvancedOptions>
      </EditorGroup>
    </EditorSection>
  )
}
