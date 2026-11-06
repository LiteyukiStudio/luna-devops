import type { DeploymentTargetFormValues } from '../deployment-target-form'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { JsonField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'

/** 容器启动与健康检查：覆盖镜像拉取策略、启动命令、生命周期钩子和探针。 */
export function ContainerFields() {
  const { t } = useTranslation()
  const { register } = useFormContext<DeploymentTargetFormValues>()
  const commandError = useFieldError('containerCommand')
  const argsError = useFieldError('containerArgs')

  return (
    <>
      <div className="grid gap-4 @xl/workspace:grid-cols-2">
        <FormField hint={t('deploymentsPage.imagePullPolicyHint')} label={t('deploymentsPage.imagePullPolicy')}>
          <NativeSelect {...register('imagePullPolicy')}>
            <option value="">{t('deploymentsPage.kubernetesDefault')}</option>
            <option value="IfNotPresent">{t('deploymentsPage.kubernetesValues.IfNotPresent')}</option>
            <option value="Always">{t('deploymentsPage.kubernetesValues.Always')}</option>
            <option value="Never">{t('deploymentsPage.kubernetesValues.Never')}</option>
          </NativeSelect>
        </FormField>
        <div className="hidden @xl/workspace:block" />
        <FormField error={commandError} hint={t('deploymentsPage.containerCommandHint')} label={t('deploymentsPage.containerCommand')}>
          <Textarea {...register('containerCommand')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.containerCommandPlaceholder') })} spellCheck={false} />
        </FormField>
        <FormField error={argsError} hint={t('deploymentsPage.containerArgsHint')} label={t('deploymentsPage.containerArgs')}>
          <Textarea {...register('containerArgs')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.containerArgsPlaceholder') })} spellCheck={false} />
        </FormField>
      </div>
      <JsonField hint={t('deploymentsPage.lifecycleHint')} label={t('deploymentsPage.lifecycle')} name="lifecycle" placeholder={t('deploymentsPage.lifecyclePlaceholder')} />
      <JsonField hint={t('deploymentsPage.readinessProbeHint')} label={t('deploymentsPage.readinessProbe')} name="readinessProbe" placeholder={t('deploymentsPage.probePlaceholder')} />
      <JsonField hint={t('deploymentsPage.livenessProbeHint')} label={t('deploymentsPage.livenessProbe')} name="livenessProbe" placeholder={t('deploymentsPage.probePlaceholder')} />
      <JsonField hint={t('deploymentsPage.startupProbeHint')} label={t('deploymentsPage.startupProbe')} name="startupProbe" placeholder={t('deploymentsPage.probePlaceholder')} />
    </>
  )
}
