import type { DeploymentTargetFormValues } from '../deployment-target-form'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { EditorSection, SwitchField } from '../deployment-target-editor-fields'

/** 发布与触发：分支/标签匹配只对源码构建生效，已有镜像来源时不展示但保留原值。 */
export function ReleaseSection() {
  const { t } = useTranslation()
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const sourceType = useWatch({ control, name: 'sourceType' })

  return (
    <EditorSection description={t('deploymentsPage.progressivePolicyDescription')} section="release" title={t('deploymentTargetEditor.sections.release')}>
      <SwitchField description={t('deploymentTargetEditor.autoDeployDescription')} label={t('deploymentsPage.autoDeploy')} name="autoDeploy" />
      {sourceType === 'repository'
        ? (
            <div className="grid gap-4 @xl/workspace:grid-cols-2">
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
