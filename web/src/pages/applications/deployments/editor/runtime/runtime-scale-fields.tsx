import type { ComponentProps } from 'react'
import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { RuntimeCluster } from '@/api'
import type { RuntimeResourcePolicy } from '@/lib/billing-display'
import { ChevronDown } from 'lucide-react'
import { useId, useState } from 'react'
import { Controller, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { CpuQuotaInput, MemoryQuotaInput } from '@/components/common/unit-input'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { RadioGroup } from '@/components/ui/radio-group'
import { useBillingDisplay } from '@/lib/billing-display'
import { matchRuntimeSizePreset, runtimeSizePresets } from '@/lib/resource-units'
import { cn } from '@/lib/utils'
import { ChoiceCard, EditorGroup, JsonField, SwitchField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'

/** 规格与伸缩：规格优先用预设；副本数与 HPA 互斥展示，避免同时编辑两个互相覆盖的副本来源。 */
export function RuntimeScaleFields({ runtimeClusters }: { runtimeClusters: RuntimeCluster[] }) {
  const { t } = useTranslation()
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const [clusterId, autoScalingEnabled, autoScalingBehavior] = useWatch({ control, name: ['clusterId', 'autoScalingEnabled', 'autoScalingBehavior'] })
  const runtimeCluster = runtimeClusters.find(cluster => cluster.id === clusterId)
    ?? (clusterId ? undefined : runtimeClusters.find(cluster => cluster.isDefault) ?? runtimeClusters[0])
  const replicasError = useFieldError('replicas')
  const minError = useFieldError('autoScalingMinReplicas')
  const maxError = useFieldError('autoScalingMaxReplicas')
  const cpuPercentError = useFieldError('autoScalingCpuPercent')
  const memoryPercentError = useFieldError('autoScalingMemoryPercent')
  const behaviorError = useFieldError('autoScalingBehavior')
  const [behaviorOpen, setBehaviorOpen] = useState(() => Boolean(autoScalingBehavior?.trim()))
  const effectiveBehaviorOpen = behaviorOpen || Boolean(behaviorError)

  return (
    <EditorGroup
      title={t('deploymentTargetEditor.scaleTitle')}
      tools={(
        <SwitchField
          inline
          description={t('deploymentsPage.autoScalingEnabledHint')}
          label={t('deploymentsPage.autoScalingEnabled')}
          name="autoScalingEnabled"
          revalidate={['autoScalingMinReplicas', 'autoScalingMaxReplicas', 'autoScalingCpuPercent', 'autoScalingMemoryPercent']}
        />
      )}
    >
      <RuntimeSizeField />
      {autoScalingEnabled
        ? (
            <div className="grid gap-4 @lg/workspace:grid-cols-2 @2xl/workspace:grid-cols-4">
              <FormField error={minError} hint={t('deploymentsPage.autoScalingMinReplicasHint')} label={t('deploymentsPage.autoScalingMinReplicas')} required>
                <Input {...register('autoScalingMinReplicas', { valueAsNumber: true, deps: ['autoScalingMaxReplicas'] })} className="font-mono tabular-nums" inputMode="numeric" min={1} type="number" />
              </FormField>
              <FormField error={maxError} hint={t('deploymentsPage.autoScalingMaxReplicasHint')} label={t('deploymentsPage.autoScalingMaxReplicas')} required>
                <Input {...register('autoScalingMaxReplicas', { valueAsNumber: true })} className="font-mono tabular-nums" inputMode="numeric" min={1} type="number" />
              </FormField>
              <FormField error={cpuPercentError} hint={t('deploymentsPage.autoScalingCpuPercentHint')} label={t('deploymentsPage.autoScalingCpuPercent')}>
                <PercentInput {...register('autoScalingCpuPercent', { valueAsNumber: true, deps: ['autoScalingMemoryPercent'] })} />
              </FormField>
              <FormField error={memoryPercentError} hint={t('deploymentsPage.autoScalingMemoryPercentHint')} label={t('deploymentsPage.autoScalingMemoryPercent')}>
                <PercentInput {...register('autoScalingMemoryPercent', { valueAsNumber: true, deps: ['autoScalingCpuPercent'] })} />
              </FormField>
            </div>
          )
        : (
            <div className="grid gap-4 @xl/workspace:grid-cols-3">
              <FormField error={replicasError} hint={t('deploymentsPage.replicasHint')} label={t('deploymentsPage.replicas')} required>
                <Input {...register('replicas', { valueAsNumber: true })} className="font-mono tabular-nums" inputMode="numeric" min={0} type="number" />
              </FormField>
            </div>
          )}
      {(autoScalingEnabled || Boolean(behaviorError)) && (
        <Collapsible open={effectiveBehaviorOpen} onOpenChange={setBehaviorOpen}>
          <div className="rounded-control border border-border bg-surface-subtle/40">
            <CollapsibleTrigger className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-xs font-medium text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring">
              <span>{t('deploymentsPage.autoScalingBehavior')}</span>
              <ChevronDown aria-hidden="true" className={cn('size-3.5 transition-transform duration-fast', effectiveBehaviorOpen && 'rotate-180')} />
            </CollapsibleTrigger>
            <CollapsibleContent className="border-t border-border p-3">
              <JsonField hint={t('deploymentsPage.autoScalingBehaviorHint')} label={t('deploymentsPage.autoScalingBehavior')} name="autoScalingBehavior" placeholder={t('deploymentsPage.autoScalingBehaviorPlaceholder')} />
            </CollapsibleContent>
          </div>
        </Collapsible>
      )}
      <RuntimeCostEstimate policy={runtimeCluster} />
    </EditorGroup>
  )
}

/**
 * 运行费用估算：实际按每分钟 max(request × 期望副本, 实际用量) 计费，实际用量不超过 limit，
 * 因此显示「保底（request）~ 上限（limit）」区间；request/limit 比例来自所选运行集群的资源策略。
 */
function RuntimeCostEstimate({ policy }: { policy?: RuntimeResourcePolicy }) {
  const { i18n, t } = useTranslation()
  const billing = useBillingDisplay(i18n.language)
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const [replicas, cpuRequest, memoryRequest, autoScalingEnabled, autoScalingMinReplicas, autoScalingMaxReplicas] = useWatch({
    control,
    name: ['replicas', 'cpuRequest', 'memoryRequest', 'autoScalingEnabled', 'autoScalingMinReplicas', 'autoScalingMaxReplicas'],
  })
  if (!policy)
    return <p className="text-xs text-muted-foreground">{t('deploymentsPage.runtimeEstimatedPriceNoCluster')}</p>

  const range = billing.runtimeHourCostRange({
    cpuQuota: cpuRequest,
    memoryQuota: memoryRequest,
    floorReplicas: autoScalingEnabled ? autoScalingMinReplicas : replicas,
    ceilingReplicas: autoScalingEnabled ? autoScalingMaxReplicas : replicas,
    policy,
  })
  const percent = (value: number) => value > 0 ? `${value}%` : t('deploymentsPage.runtimePolicyNotSet')
  return (
    <div className="grid gap-1 text-xs text-muted-foreground">
      <p>
        {range.ceiling === null
          ? t('deploymentsPage.runtimeEstimatedPriceUnbounded', { floor: billing.formatAmountWithUnit(range.floor) })
          : t('deploymentsPage.runtimeEstimatedPriceRange', { ceiling: billing.formatAmountWithUnit(range.ceiling), floor: billing.formatAmountWithUnit(range.floor) })}
        {autoScalingEnabled && ` ${t('deploymentTargetEditor.autoScalingPriceNote')}`}
      </p>
      <p>
        {t('deploymentsPage.runtimeEstimatedPricePolicy', {
          cpuLimit: percent(policy.cpuLimitPercent),
          cpuRequest: percent(policy.cpuRequestPercent),
          memoryLimit: percent(policy.memoryLimitPercent),
          memoryRequest: percent(policy.memoryRequestPercent),
        })}
      </p>
    </div>
  )
}

/** 规格预设：选择 S/M/L 只写入 cpuRequest 与 memoryRequest；当前值不匹配预设或用户选择自定义时展示配额输入框。 */
function RuntimeSizeField() {
  const { i18n, t } = useTranslation()
  const labelId = useId()
  const { control, setValue } = useFormContext<DeploymentTargetFormValues>()
  const [cpuRequest, memoryRequest] = useWatch({ control, name: ['cpuRequest', 'memoryRequest'] })
  const [customSelected, setCustomSelected] = useState(false)
  const cpuError = useFieldError('cpuRequest')
  const memoryError = useFieldError('memoryRequest')
  const matched = matchRuntimeSizePreset(cpuRequest, memoryRequest)
  const selected = customSelected || !matched ? 'custom' : matched
  const numberFormat = new Intl.NumberFormat(i18n.language)
  const presetLabels = {
    s: t('deploymentTargetEditor.sizePresets.s'),
    m: t('deploymentTargetEditor.sizePresets.m'),
    l: t('deploymentTargetEditor.sizePresets.l'),
  }

  const select = (value: string) => {
    const preset = runtimeSizePresets.find(item => item.id === value)
    setCustomSelected(!preset)
    if (!preset)
      return
    const update = { shouldDirty: true, shouldValidate: true }
    setValue('cpuRequest', preset.cpuRequest, update)
    setValue('memoryRequest', preset.memoryRequest, update)
  }

  return (
    <div className="grid min-w-0 gap-3">
      <div className="grid gap-1.5">
        <span className="text-sm font-medium text-foreground" id={labelId}>{t('deploymentTargetEditor.sizeLabel')}</span>
        <RadioGroup aria-labelledby={labelId} className="grid gap-2 @lg/workspace:grid-cols-2 @2xl/workspace:grid-cols-4" value={selected} onValueChange={select}>
          {runtimeSizePresets.map(preset => (
            <ChoiceCard
              key={preset.id}
              description={t('deploymentTargetEditor.sizePresetDescription', { cpu: numberFormat.format(preset.cores), memory: preset.memoryRequest })}
              id={`deployment-target-size-${preset.id}`}
              label={presetLabels[preset.id]}
              value={preset.id}
            />
          ))}
          <ChoiceCard description={t('deploymentTargetEditor.sizeCustomDescription')} id="deployment-target-size-custom" label={t('deploymentTargetEditor.sizeCustom')} value="custom" />
        </RadioGroup>
      </div>
      {selected === 'custom' && (
        <div className="grid gap-4 rounded-control border border-border bg-surface-subtle/60 p-3.5 @xl/workspace:grid-cols-2">
          <FormField error={cpuError} hint={t('deploymentsPage.runtimeQuotaPolicyHint')} label={t('deploymentsPage.cpuRequest')} required>
            {fieldControl => (
              <Controller
                control={control}
                name="cpuRequest"
                render={({ field }) => <CpuQuotaInput inputProps={fieldControl} value={field.value} onChange={field.onChange} />}
              />
            )}
          </FormField>
          <FormField error={memoryError} hint={t('deploymentsPage.runtimeQuotaPolicyHint')} label={t('deploymentsPage.memoryRequest')} required>
            {fieldControl => (
              <Controller
                control={control}
                name="memoryRequest"
                render={({ field }) => <MemoryQuotaInput inputProps={fieldControl} value={field.value} onChange={field.onChange} />}
              />
            )}
          </FormField>
        </div>
      )}
    </div>
  )
}

function PercentInput(props: ComponentProps<typeof Input>) {
  const isInvalid = props['aria-invalid'] === true || props['aria-invalid'] === 'true'
  return (
    <div
      className={cn(
        'flex h-9 min-w-0 items-center overflow-hidden rounded-control border border-input bg-surface shadow-xs transition-[color,box-shadow] dark:bg-input/30',
        isInvalid
          ? 'border-danger ring-danger/20 focus-within:border-danger focus-within:ring-3 focus-within:ring-danger/20 dark:ring-danger/40'
          : 'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50',
      )}
    >
      <Input
        {...props}
        className="h-full rounded-none border-0 bg-transparent font-mono tabular-nums shadow-none focus-visible:ring-0 aria-invalid:border-0 aria-invalid:ring-0 dark:bg-transparent"
        inputMode="numeric"
        max={1000}
        min={0}
        type="number"
      />
      <span aria-hidden="true" className="px-3 text-xs font-medium text-muted-foreground">%</span>
    </div>
  )
}
