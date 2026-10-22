import type { DeploymentTargetFormValues } from '../deployment-target-form'
import { Plus, X } from 'lucide-react'
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { AdvancedOptions, EditorGroup } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { changedFieldCount, maxServicePorts } from '../deployment-target-form'

/** 服务端口表格：第一行是默认端口，列标题可见，每个输入仍有独立可访问名称。 */
export function ServicePortsFields() {
  const { t } = useTranslation()
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const { append, fields, remove } = useFieldArray({ control, name: 'servicePorts' })
  const [serviceSessionAffinity, serviceAnnotations] = useWatch({ control, name: ['serviceSessionAffinity', 'serviceAnnotations'] })
  const annotationsError = useFieldError('serviceAnnotations')

  return (
    <EditorGroup
      description={t('deploymentsPage.servicePortsHint')}
      title={t('deploymentsPage.servicePorts')}
      tools={(
        <Button
          disabled={fields.length >= maxServicePorts}
          size="sm"
          type="button"
          variant="outline"
          onClick={() => append({ name: `port-${fields.length + 1}`, port: 9000 + fields.length, appProtocol: '' })}
        >
          <Plus className="size-4" />
          {t('deploymentsPage.addServicePort')}
        </Button>
      )}
    >
      <div className="grid gap-2">
        <div className="hidden gap-2 px-1 text-xs font-medium text-muted-foreground md:grid md:grid-cols-[minmax(0,1.2fr)_8rem_minmax(0,1fr)_2.25rem]" aria-hidden="true">
          <span>{t('deploymentsPage.servicePortName')}</span>
          <span>{t('deploymentsPage.servicePortNumber')}</span>
          <span>{t('deploymentsPage.servicePortAppProtocol')}</span>
        </div>
        {fields.map((field, index) => (
          <ServicePortRow key={field.id} index={index} removable={fields.length > 1} onRemove={() => remove(index)} />
        ))}
      </div>
      <AdvancedOptions configuredCount={changedFieldCount({ serviceSessionAffinity, serviceAnnotations }, ['serviceSessionAffinity', 'serviceAnnotations'])} fields={['serviceSessionAffinity', 'serviceAnnotations']} title={t('deploymentTargetEditor.serviceAdvanced')}>
        <div className="grid gap-4 md:grid-cols-2">
          <FormField hint={t('deploymentsPage.serviceSessionAffinityHint')} label={t('deploymentsPage.serviceSessionAffinity')}>
            <NativeSelect {...register('serviceSessionAffinity')}>
              <option value="">{t('deploymentsPage.kubernetesDefaultNone')}</option>
              <option value="None">{t('deploymentsPage.kubernetesValues.None')}</option>
              <option value="ClientIP">{t('deploymentsPage.kubernetesValues.ClientIP')}</option>
            </NativeSelect>
          </FormField>
          <FormField error={annotationsError} hint={t('deploymentsPage.serviceAnnotationsHint')} label={t('deploymentsPage.serviceAnnotations')}>
            <Textarea {...register('serviceAnnotations')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.serviceAnnotationsPlaceholder') })} />
          </FormField>
        </div>
      </AdvancedOptions>
    </EditorGroup>
  )
}

function ServicePortRow({ index, onRemove, removable }: { index: number, onRemove: () => void, removable: boolean }) {
  const { t } = useTranslation()
  const { register } = useFormContext<DeploymentTargetFormValues>()
  const nameError = useFieldError(`servicePorts.${index}.name`)
  const portError = useFieldError(`servicePorts.${index}.port`)
  const protocolError = useFieldError(`servicePorts.${index}.appProtocol`)
  const errors = [nameError, portError, protocolError].filter(Boolean)
  const errorId = `deployment-target-port-${index}-error`
  const rowLabel = t('deploymentTargetEditor.portRowLabel', { index: index + 1 })

  return (
    <div className="grid gap-1">
      <div className="grid gap-2 rounded-control bg-surface-inset/60 p-2 md:grid-cols-[minmax(0,1.2fr)_8rem_minmax(0,1fr)_2.25rem] md:items-center md:bg-transparent md:p-0">
        <Input
          {...register(`servicePorts.${index}.name`)}
          aria-describedby={errors.length ? errorId : undefined}
          aria-invalid={nameError ? true : undefined}
          aria-label={`${rowLabel} ${t('deploymentsPage.servicePortName')}`}
          placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: index === 0 ? 'http' : 'metrics' })}
        />
        <Input
          {...register(`servicePorts.${index}.port`, { valueAsNumber: true })}
          aria-describedby={errors.length ? errorId : undefined}
          aria-invalid={portError ? true : undefined}
          aria-label={`${rowLabel} ${t('deploymentsPage.servicePortNumber')}`}
          inputMode="numeric"
          max={65535}
          min={1}
          type="number"
        />
        <Input
          {...register(`servicePorts.${index}.appProtocol`)}
          aria-describedby={errors.length ? errorId : undefined}
          aria-invalid={protocolError ? true : undefined}
          aria-label={`${rowLabel} ${t('deploymentsPage.servicePortAppProtocol')}`}
          placeholder={t('deploymentTargetEditor.optional')}
        />
        <Button
          aria-label={t('deploymentTargetEditor.removePort', { index: index + 1 })}
          className="justify-self-end"
          disabled={!removable}
          size="icon"
          type="button"
          variant="ghost"
          onClick={onRemove}
        >
          <X className="size-4" />
        </Button>
      </div>
      {errors.length > 0 && <p className="text-xs text-danger" id={errorId} role="alert">{errors.join(' ')}</p>}
    </div>
  )
}
