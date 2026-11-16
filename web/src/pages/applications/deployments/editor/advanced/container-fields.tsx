import type { DeploymentTargetFormValues } from '../deployment-target-form'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { JsonField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { ProbeFieldEditor } from './probe-field-editor'
import { AuxContainersFields } from './security-scheduling-fields'

/** 健康检查探针：Startup / Readiness / Liveness 三类探针独立分组。 */
export function ProbesFields() {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const servicePorts = useWatch({ control, name: 'servicePorts' })
  const defaultPort = Number(servicePorts?.[0]?.port) || 8080

  return (
    <div className="grid gap-3">
      <ProbeFieldEditor defaultPort={defaultPort} hint={t('deploymentsPage.startupProbeHint')} label={t('deploymentsPage.startupProbe')} name="startupProbe" placeholder={t('deploymentsPage.probePlaceholder')} />
      <ProbeFieldEditor defaultPort={defaultPort} hint={t('deploymentsPage.readinessProbeHint')} label={t('deploymentsPage.readinessProbe')} name="readinessProbe" placeholder={t('deploymentsPage.probePlaceholder')} />
      <ProbeFieldEditor defaultPort={defaultPort} hint={t('deploymentsPage.livenessProbeHint')} label={t('deploymentsPage.livenessProbe')} name="livenessProbe" placeholder={t('deploymentsPage.probePlaceholder')} />
    </div>
  )
}

/** 容器命令与运行控制：覆盖启动命令、参数、生命周期钩子、Web Console 策略与辅助容器。 */
export function ContainerFields() {
  const { t } = useTranslation()
  const { register } = useFormContext<DeploymentTargetFormValues>()
  const commandError = useFieldError('containerCommand')
  const argsError = useFieldError('containerArgs')

  return (
    <div className="grid gap-4">
      <div className="grid gap-4 @xl/workspace:grid-cols-2">
        <FormField error={commandError} hint={t('deploymentsPage.containerCommandHint')} label={t('deploymentsPage.containerCommand')}>
          <Textarea {...register('containerCommand')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.containerCommandPlaceholder') })} spellCheck={false} />
        </FormField>
        <FormField error={argsError} hint={t('deploymentsPage.containerArgsHint')} label={t('deploymentsPage.containerArgs')}>
          <Textarea {...register('containerArgs')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.containerArgsPlaceholder') })} spellCheck={false} />
        </FormField>
        <FormField hint={t('deploymentsPage.webConsoleOverrideHint')} label={t('deploymentsPage.webConsoleOverride')}>
          <NativeSelect {...register('webConsoleEnabled')}>
            <option value="inherit">{t('deploymentsPage.webConsoleInheritProject')}</option>
            <option value="disabled">{t('deploymentsPage.webConsoleDisableForTarget')}</option>
          </NativeSelect>
        </FormField>
      </div>
      <JsonField hint={t('deploymentsPage.lifecycleHint')} label={t('deploymentsPage.lifecycle')} name="lifecycle" placeholder={t('deploymentsPage.lifecyclePlaceholder')} />
      <AuxContainersFields />
    </div>
  )
}
