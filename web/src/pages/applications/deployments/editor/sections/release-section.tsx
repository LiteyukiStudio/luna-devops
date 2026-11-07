import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { BuildConcurrencyPolicy } from '@/api'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { EditorSection, SwitchField } from '../deployment-target-editor-fields'
import { buildConcurrencyPolicies } from '../deployment-target-form'

/** 发布与触发：分支/标签匹配只对源码构建生效，已有镜像来源时不展示但保留原值。 */
export function ReleaseSection() {
  const { t } = useTranslation()
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const sourceType = useWatch({ control, name: 'sourceType' })
  const buildConcurrencyPolicy = useWatch({ control, name: 'buildConcurrencyPolicy' })
  const policyOptions: Record<BuildConcurrencyPolicy, { hint: string, label: string }> = {
    queue: { label: t('deploymentTargetEditor.buildConcurrencyPolicies.queue'), hint: t('deploymentTargetEditor.buildConcurrencyPolicyHints.queue') },
    cancel_in_progress: { label: t('deploymentTargetEditor.buildConcurrencyPolicies.cancel_in_progress'), hint: t('deploymentTargetEditor.buildConcurrencyPolicyHints.cancel_in_progress') },
    parallel: { label: t('deploymentTargetEditor.buildConcurrencyPolicies.parallel'), hint: t('deploymentTargetEditor.buildConcurrencyPolicyHints.parallel') },
  }

  return (
    <EditorSection description={t('deploymentsPage.progressivePolicyDescription')} section="release" title={t('deploymentTargetEditor.sections.release')}>
      <SwitchField description={t('deploymentTargetEditor.autoDeployDescription')} label={t('deploymentsPage.autoDeploy')} name="autoDeploy" />
      {sourceType === 'repository'
        ? (
            <div className="grid gap-4 @xl/workspace:grid-cols-2">
              <FormField className="@xl/workspace:col-span-2" hint={policyOptions[buildConcurrencyPolicy].hint} label={t('deploymentTargetEditor.buildConcurrencyPolicy')}>
                <NativeSelect {...register('buildConcurrencyPolicy')}>
                  {buildConcurrencyPolicies.map(policy => <option key={policy} value={policy}>{policyOptions[policy].label}</option>)}
                </NativeSelect>
              </FormField>
              <FormField hint={t('deploymentsPage.branchPatternHint')} label={t('deploymentsPage.branchPattern')}>
                <Input {...register('branchPattern')} placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.branchPatternPlaceholder') })} />
              </FormField>
              <FormField hint={t('deploymentsPage.tagPatternHint')} label={t('deploymentsPage.tagPattern')}>
                <Input {...register('tagPattern')} placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.tagPatternPlaceholder') })} />
              </FormField>
            </div>
          )
        : <p className="text-sm text-muted-foreground">{t('deploymentTargetEditor.imageSourceTriggerNote')}</p>}
    </EditorSection>
  )
}
