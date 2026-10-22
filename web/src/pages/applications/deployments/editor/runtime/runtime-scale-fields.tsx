import type { ComponentProps } from 'react'
import type { DeploymentTargetFormValues } from '../deployment-target-form'
import { Controller, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { UnitInput } from '@/components/common/unit-input'
import { Input } from '@/components/ui/input'
import { useBillingDisplay } from '@/lib/billing-display'
import { AdvancedOptions, EditorGroup, JsonField, SwitchField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { changedFieldCount } from '../deployment-target-form'
import { cpuUnits, memoryUnits } from '../resource-units'

/** 规格与伸缩：副本数与 HPA 互斥展示，避免同时编辑两个互相覆盖的副本来源。 */
export function RuntimeScaleFields() {
  const { i18n, t } = useTranslation()
  const billing = useBillingDisplay(i18n.language)
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const [replicas, cpuRequest, memoryRequest, autoScalingEnabled, autoScalingMinReplicas, autoScalingBehavior] = useWatch({
    control,
    name: ['replicas', 'cpuRequest', 'memoryRequest', 'autoScalingEnabled', 'autoScalingMinReplicas', 'autoScalingBehavior'],
  })
  const replicasError = useFieldError('replicas')
  const cpuError = useFieldError('cpuRequest')
  const memoryError = useFieldError('memoryRequest')
  const minError = useFieldError('autoScalingMinReplicas')
  const maxError = useFieldError('autoScalingMaxReplicas')
  const cpuPercentError = useFieldError('autoScalingCpuPercent')
  const memoryPercentError = useFieldError('autoScalingMemoryPercent')

  return (
    <EditorGroup description={t('deploymentTargetEditor.scaleDescription')} title={t('deploymentTargetEditor.scaleTitle')}>
      <div className="grid gap-4 md:grid-cols-3">
        {!autoScalingEnabled && (
          <FormField error={replicasError} hint={t('deploymentsPage.replicasHint')} label={t('deploymentsPage.replicas')} required>
            <Input {...register('replicas', { valueAsNumber: true })} inputMode="numeric" min={0} type="number" />
          </FormField>
        )}
        <FormField error={cpuError} hint={t('deploymentsPage.runtimeQuotaPolicyHint')} label={t('deploymentsPage.cpuRequest')} required>
          {fieldControl => (
            <Controller
              control={control}
              name="cpuRequest"
              render={({ field }) => <UnitInput unitSelectLabel={t('deploymentTargetEditor.cpuUnitLabel')} units={cpuUnits(t)} inputProps={fieldControl} value={field.value} onChange={field.onChange} />}
            />
          )}
        </FormField>
        <FormField error={memoryError} hint={t('deploymentsPage.runtimeQuotaPolicyHint')} label={t('deploymentsPage.memoryRequest')} required>
          {fieldControl => (
            <Controller
              control={control}
              name="memoryRequest"
              render={({ field }) => <UnitInput unitSelectLabel={t('deploymentTargetEditor.memoryUnitLabel')} units={memoryUnits} inputProps={fieldControl} value={field.value} onChange={field.onChange} />}
            />
          )}
        </FormField>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('deploymentsPage.runtimeEstimatedPrice', { price: billing.formatAmountWithUnit(billing.runtimeHourCost(autoScalingEnabled ? autoScalingMinReplicas : replicas, cpuRequest, memoryRequest)) })}
        {autoScalingEnabled && ` ${t('deploymentTargetEditor.autoScalingPriceNote')}`}
      </p>
      <SwitchField
        description={t('deploymentsPage.autoScalingEnabledHint')}
        label={t('deploymentsPage.autoScalingEnabled')}
        name="autoScalingEnabled"
        revalidate={['autoScalingMinReplicas', 'autoScalingMaxReplicas', 'autoScalingCpuPercent', 'autoScalingMemoryPercent']}
      />
      {autoScalingEnabled && (
        <div className="grid gap-4">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <FormField error={minError} hint={t('deploymentsPage.autoScalingMinReplicasHint')} label={t('deploymentsPage.autoScalingMinReplicas')} required>
              <Input {...register('autoScalingMinReplicas', { valueAsNumber: true, deps: ['autoScalingMaxReplicas'] })} inputMode="numeric" min={1} type="number" />
            </FormField>
            <FormField error={maxError} hint={t('deploymentsPage.autoScalingMaxReplicasHint')} label={t('deploymentsPage.autoScalingMaxReplicas')} required>
              <Input {...register('autoScalingMaxReplicas', { valueAsNumber: true })} inputMode="numeric" min={1} type="number" />
            </FormField>
            <FormField error={cpuPercentError} hint={t('deploymentsPage.autoScalingCpuPercentHint')} label={t('deploymentsPage.autoScalingCpuPercent')}>
              <PercentInput {...register('autoScalingCpuPercent', { valueAsNumber: true, deps: ['autoScalingMemoryPercent'] })} />
            </FormField>
            <FormField error={memoryPercentError} hint={t('deploymentsPage.autoScalingMemoryPercentHint')} label={t('deploymentsPage.autoScalingMemoryPercent')}>
              <PercentInput {...register('autoScalingMemoryPercent', { valueAsNumber: true, deps: ['autoScalingCpuPercent'] })} />
            </FormField>
          </div>
          <AdvancedOptions configuredCount={changedFieldCount({ autoScalingBehavior }, ['autoScalingBehavior'])} fields={['autoScalingBehavior']} title={t('deploymentsPage.autoScalingBehavior')}>
            <JsonField hint={t('deploymentsPage.autoScalingBehaviorHint')} label={t('deploymentsPage.autoScalingBehavior')} name="autoScalingBehavior" placeholder={t('deploymentsPage.autoScalingBehaviorPlaceholder')} />
          </AdvancedOptions>
        </div>
      )}
    </EditorGroup>
  )
}

function PercentInput(props: ComponentProps<typeof Input>) {
  return (
    <div className="flex min-w-0 items-center overflow-hidden rounded-md border border-input bg-background shadow-xs focus-within:ring-2 focus-within:ring-ring/50">
      <Input {...props} className="rounded-none border-0 shadow-none focus-visible:ring-0" inputMode="numeric" max={1000} min={0} type="number" />
      <span aria-hidden="true" className="px-3 text-sm text-muted-foreground">%</span>
    </div>
  )
}
