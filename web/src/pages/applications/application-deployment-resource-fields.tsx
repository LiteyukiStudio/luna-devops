import type { UseFormReturn } from 'react-hook-form'
import type { DeploymentTargetPayload } from '@/api'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { UnitInput } from '@/components/common/unit-input'
import { Input } from '@/components/ui/input'

interface RuntimeResourceFieldsProps {
  form: UseFormReturn<DeploymentTargetPayload>
  priceText: string
}

export function RuntimeResourceFields({ form, priceText }: RuntimeResourceFieldsProps) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-3 md:col-span-2">
      <div className="grid gap-3 md:grid-cols-3">
        <Field label={t('deploymentsPage.replicas')} required>
          <Input {...form.register('replicas', { valueAsNumber: true })} min={1} type="number" />
        </Field>
        <Field label={t('deploymentsPage.cpuRequest')} required>
          <UnitInput
            unitSelectLabel={t('deploymentsPage.cpuRequest')}
            units={[
              { label: 'm', value: 'm' },
              { label: t('deploymentsPage.cpuUnits.core'), value: '' },
            ]}
            value={form.watch('cpuRequest')}
            onChange={value => form.setValue('cpuRequest', value, { shouldDirty: true, shouldValidate: true })}
          />
        </Field>
        <Field label={t('deploymentsPage.memoryRequest')} required>
          <UnitInput
            unitSelectLabel={t('deploymentsPage.memoryRequest')}
            units={[
              { label: 'Mi', value: 'Mi' },
              { label: 'Gi', value: 'Gi' },
            ]}
            value={form.watch('memoryRequest')}
            onChange={value => form.setValue('memoryRequest', value, { shouldDirty: true, shouldValidate: true })}
          />
        </Field>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        {t('deploymentsPage.runtimeEstimatedPrice', { price: priceText })}
      </p>
    </div>
  )
}

interface BuildEnvironmentFieldsProps {
  buildTimeoutMinutes: number
  form: UseFormReturn<DeploymentTargetPayload>
  priceText: string
}

export function BuildEnvironmentFields({ buildTimeoutMinutes, form, priceText }: BuildEnvironmentFieldsProps) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-3">
      <div>
        <h3 className="text-sm font-semibold">{t('deploymentsPage.buildEnvironment')}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{t('deploymentsPage.buildEnvironmentDescription')}</p>
      </div>
      <div className="grid gap-3 md:grid-cols-3">
        <Field label={t('deploymentsPage.buildCpuRequest')} required>
          <UnitInput
            unitSelectLabel={t('deploymentsPage.buildCpuRequest')}
            units={[
              { label: 'm', value: 'm' },
              { label: t('deploymentsPage.cpuUnits.core'), value: '' },
            ]}
            value={form.watch('buildCpuRequest')}
            onChange={value => form.setValue('buildCpuRequest', value, { shouldDirty: true, shouldValidate: true })}
          />
        </Field>
        <Field label={t('deploymentsPage.buildMemoryRequest')} required>
          <UnitInput
            unitSelectLabel={t('deploymentsPage.buildMemoryRequest')}
            units={[
              { label: 'Mi', value: 'Mi' },
              { label: 'Gi', value: 'Gi' },
            ]}
            value={form.watch('buildMemoryRequest')}
            onChange={value => form.setValue('buildMemoryRequest', value, { shouldDirty: true, shouldValidate: true })}
          />
          <p className="mt-1 text-xs text-muted-foreground">
            {t('deploymentsPage.buildEstimatedPrice', { price: priceText })}
          </p>
        </Field>
        <Field hint={t('deploymentsPage.buildTimeoutHint')} label={t('deploymentsPage.buildTimeoutMinutes')} required>
          <Input
            min={1}
            type="number"
            value={buildTimeoutMinutes}
            onChange={event => form.setValue('buildTimeoutSeconds', Math.max(1, Number(event.target.value) || 1) * 60, { shouldDirty: true, shouldValidate: true })}
          />
        </Field>
      </div>
    </div>
  )
}
