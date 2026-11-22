import type { AISelfHealMode, ProjectAIAutomationConfig } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Activity, LoaderCircle, Save, Sparkles, Wallet } from 'lucide-react'
import { useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { OverviewSkeleton } from '@/components/common/loading-states'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { ProjectAIDiagnosisHistorySection } from './project-ai-diagnosis-history'

const INHERIT_STAGE_MODE = '__inherit__' as const

const RUNTIME_HEAL_TOOLS = [
  'delivery_restart_target',
  'delivery_rollback_target',
  'delivery_scale_target',
  'delivery_update_target_runtime',
  'delivery_deploy_target',
] as const

type StageKey = 'dev' | 'test' | 'staging' | 'prod'
type StageModeValue = AISelfHealMode | typeof INHERIT_STAGE_MODE

const selfHealModeEnum = z.enum(['suggest_only', 'auto_l1', 'auto_l2'])
const stageModeEnum = z.enum(['__inherit__', 'suggest_only', 'auto_l1', 'auto_l2'])

const aiAutomationFormSchema = z.object({
  deliveryDiagnosisEnabled: z.boolean(),
  deliveryCooldownSeconds: z
    .number({ error: () => i18next.t('errors.ai_automation.cooldown_invalid') })
    .int({ error: () => i18next.t('errors.ai_automation.cooldown_invalid') })
    .min(0, { error: () => i18next.t('errors.ai_automation.cooldown_invalid') })
    .max(86400, { error: () => i18next.t('errors.ai_automation.cooldown_invalid') }),
  deliveryDailyLimit: z
    .number({ error: () => i18next.t('errors.ai_automation.daily_limit_invalid') })
    .int({ error: () => i18next.t('errors.ai_automation.daily_limit_invalid') })
    .min(1, { error: () => i18next.t('errors.ai_automation.daily_limit_invalid') })
    .max(500, { error: () => i18next.t('errors.ai_automation.daily_limit_invalid') }),
  deliveryBillingUserId: z.string(),
  runtimeDiagnosisEnabled: z.boolean(),
  runtimeCooldownSeconds: z
    .number({ error: () => i18next.t('errors.ai_automation.cooldown_invalid') })
    .int({ error: () => i18next.t('errors.ai_automation.cooldown_invalid') })
    .min(0, { error: () => i18next.t('errors.ai_automation.cooldown_invalid') })
    .max(86400, { error: () => i18next.t('errors.ai_automation.cooldown_invalid') }),
  runtimeDailyLimit: z
    .number({ error: () => i18next.t('errors.ai_automation.daily_limit_invalid') })
    .int({ error: () => i18next.t('errors.ai_automation.daily_limit_invalid') })
    .min(1, { error: () => i18next.t('errors.ai_automation.daily_limit_invalid') })
    .max(500, { error: () => i18next.t('errors.ai_automation.daily_limit_invalid') }),
  runtimeBillingUserId: z.string(),
  selfHealMode: selfHealModeEnum,
  stageModeDev: stageModeEnum,
  stageModeTest: stageModeEnum,
  stageModeStaging: stageModeEnum,
  stageModeProd: stageModeEnum,
  allowedAutoHealTools: z.array(z.string()),
  maxConsecutiveAutoHeals: z
    .number({ error: () => i18next.t('errors.ai_automation.circuit_breaker_invalid') })
    .int({ error: () => i18next.t('errors.ai_automation.circuit_breaker_invalid') })
    .min(1, { error: () => i18next.t('errors.ai_automation.circuit_breaker_invalid') })
    .max(10, { error: () => i18next.t('errors.ai_automation.circuit_breaker_invalid') }),
  circuitBreakerWindowSeconds: z
    .number({ error: () => i18next.t('errors.ai_automation.circuit_breaker_invalid') })
    .int({ error: () => i18next.t('errors.ai_automation.circuit_breaker_invalid') })
    .min(60, { error: () => i18next.t('errors.ai_automation.circuit_breaker_invalid') })
    .max(86400, { error: () => i18next.t('errors.ai_automation.circuit_breaker_invalid') }),
})

type AIAutomationFormValues = z.infer<typeof aiAutomationFormSchema>

function toStageModeValue(map: Record<string, AISelfHealMode> | undefined, stage: StageKey): StageModeValue {
  const raw = map?.[stage]
  if (raw === 'suggest_only' || raw === 'auto_l1' || raw === 'auto_l2') {
    if (stage === 'prod' && raw === 'auto_l1')
      return INHERIT_STAGE_MODE
    return raw
  }
  return INHERIT_STAGE_MODE
}

function buildStageSelfHealModes(values: AIAutomationFormValues): Record<string, AISelfHealMode> {
  const out: Record<string, AISelfHealMode> = {}
  if (values.stageModeDev !== INHERIT_STAGE_MODE)
    out.dev = values.stageModeDev
  if (values.stageModeTest !== INHERIT_STAGE_MODE)
    out.test = values.stageModeTest
  if (values.stageModeStaging !== INHERIT_STAGE_MODE)
    out.staging = values.stageModeStaging
  if (values.stageModeProd !== INHERIT_STAGE_MODE && values.stageModeProd !== 'auto_l1')
    out.prod = values.stageModeProd
  return out
}

function hasAutoL2Configured(values: AIAutomationFormValues): boolean {
  return (
    values.selfHealMode === 'auto_l2'
    || values.stageModeDev === 'auto_l2'
    || values.stageModeTest === 'auto_l2'
    || values.stageModeStaging === 'auto_l2'
    || values.stageModeProd === 'auto_l2'
  )
}

function configToFormValues(data: {
  deliveryDiagnosisEnabled: boolean
  deliveryCooldownSeconds: number
  deliveryDailyLimit: number
  deliveryBillingUserId?: string
  runtimeDiagnosisEnabled?: boolean
  runtimeCooldownSeconds?: number
  runtimeDailyLimit?: number
  runtimeBillingUserId?: string
  selfHealMode?: AISelfHealMode
  stageSelfHealModes?: Record<string, AISelfHealMode>
  allowedAutoHealTools?: string[]
  maxConsecutiveAutoHeals?: number
  circuitBreakerWindowSeconds?: number
}): AIAutomationFormValues {
  return {
    deliveryDiagnosisEnabled: data.deliveryDiagnosisEnabled,
    deliveryCooldownSeconds: data.deliveryCooldownSeconds,
    deliveryDailyLimit: data.deliveryDailyLimit,
    deliveryBillingUserId: data.deliveryBillingUserId || '',
    runtimeDiagnosisEnabled: Boolean(data.runtimeDiagnosisEnabled),
    runtimeCooldownSeconds: data.runtimeCooldownSeconds ?? 300,
    runtimeDailyLimit: data.runtimeDailyLimit ?? 20,
    runtimeBillingUserId: data.runtimeBillingUserId || '',
    selfHealMode: data.selfHealMode ?? 'suggest_only',
    stageModeDev: toStageModeValue(data.stageSelfHealModes, 'dev'),
    stageModeTest: toStageModeValue(data.stageSelfHealModes, 'test'),
    stageModeStaging: toStageModeValue(data.stageSelfHealModes, 'staging'),
    stageModeProd: toStageModeValue(data.stageSelfHealModes, 'prod'),
    allowedAutoHealTools: data.allowedAutoHealTools ?? [],
    maxConsecutiveAutoHeals: data.maxConsecutiveAutoHeals ?? 2,
    circuitBreakerWindowSeconds: data.circuitBreakerWindowSeconds ?? 1800,
  }
}

export function ProjectAIAutomationPanel({
  canManage,
  projectId,
}: {
  canManage: boolean
  projectId: string
}) {
  const { t } = useTranslation()

  const configQuery = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-ai-automation-config', projectId],
    queryFn: ({ signal }) => api.getProjectAIAutomationConfig(projectId, signal),
    enabled: Boolean(projectId),
  })

  if (configQuery.isLoading || !configQuery.data) {
    if (configQuery.isError) {
      return (
        <ErrorState
          description={configQuery.error instanceof Error ? configQuery.error.message : t('notificationsPage.loadFailedDescription')}
          retrying={configQuery.isFetching}
          title={t('notificationsPage.loadFailedTitle')}
          onRetry={() => void configQuery.refetch()}
        />
      )
    }
    return <OverviewSkeleton />
  }

  return (
    <div className="space-y-8">
      <ProjectAIAutomationForm
        canManage={canManage}
        config={configQuery.data}
        key={`form:${configQuery.data.projectId}`}
        projectId={projectId}
      />
      <ProjectAIDiagnosisHistorySection key={`history:${projectId}`} projectId={projectId} />
    </div>
  )
}

function ProjectAIAutomationForm({
  canManage,
  config,
  projectId,
}: {
  canManage: boolean
  config: ProjectAIAutomationConfig
  projectId: string
}) {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [pendingAutoL2Values, setPendingAutoL2Values] = useState<AIAutomationFormValues | null>(null)

  const form = useForm<AIAutomationFormValues>({
    resolver: zodResolver(aiAutomationFormSchema),
    defaultValues: configToFormValues(config),
    mode: 'onChange',
  })
  const { errors, isDirty } = form.formState

  const saveMutation = useMutation({
    mutationFn: (values: AIAutomationFormValues) => api.updateProjectAIAutomationConfig(projectId, {
      deliveryDiagnosisEnabled: values.deliveryDiagnosisEnabled,
      deliveryCooldownSeconds: values.deliveryCooldownSeconds,
      deliveryDailyLimit: values.deliveryDailyLimit,
      deliveryBillingUserId: values.deliveryBillingUserId,
      runtimeDiagnosisEnabled: values.runtimeDiagnosisEnabled,
      runtimeCooldownSeconds: values.runtimeCooldownSeconds,
      runtimeDailyLimit: values.runtimeDailyLimit,
      runtimeBillingUserId: values.runtimeBillingUserId,
      selfHealMode: values.selfHealMode,
      stageSelfHealModes: buildStageSelfHealModes(values),
      allowedAutoHealTools: values.allowedAutoHealTools,
      maxConsecutiveAutoHeals: values.maxConsecutiveAutoHeals,
      circuitBreakerWindowSeconds: values.circuitBreakerWindowSeconds,
    }),
    onSuccess: (updated) => {
      setPendingAutoL2Values(null)
      queryClient.setQueryData(['project-ai-automation-config', projectId], updated)
      form.reset(configToFormValues(updated))
      toast.success(t('aiAssistant.projectAutomation.savedToast'))
    },
    onError: (error) => {
      setPendingAutoL2Values(null)
      toast.error(error instanceof Error ? error.message : t('common.unavailable'))
    },
  })

  const watchedDeliveryBillingUserId = form.watch('deliveryBillingUserId')
  const watchedDeliveryEnabled = form.watch('deliveryDiagnosisEnabled')
  const watchedRuntimeBillingUserId = form.watch('runtimeBillingUserId')
  const watchedRuntimeEnabled = form.watch('runtimeDiagnosisEnabled')
  const currentUserId = user?.id ?? ''

  const resolveBillingDisplay = (billingUserId: string, savedUserId?: string, savedUserName?: string) => {
    if (!billingUserId)
      return t('aiAssistant.projectAutomation.billingOwnerUnbound')
    if (billingUserId === savedUserId && savedUserName)
      return savedUserName
    if (billingUserId === currentUserId)
      return user?.name || user?.email || currentUserId
    return billingUserId
  }

  const deliveryOwnerDisplay = resolveBillingDisplay(
    watchedDeliveryBillingUserId,
    config.deliveryBillingUserId,
    config.deliveryBillingUserName,
  )
  const runtimeOwnerDisplay = resolveBillingDisplay(
    watchedRuntimeBillingUserId,
    config.runtimeBillingUserId,
    config.runtimeBillingUserName,
  )

  const handleFormSubmit = (values: AIAutomationFormValues) => {
    if (hasAutoL2Configured(values)) {
      setPendingAutoL2Values(values)
      return
    }
    saveMutation.mutate(values)
  }

  const stageFields: Array<{ fieldName: 'stageModeDev' | 'stageModeTest' | 'stageModeStaging' | 'stageModeProd', label: string, allowAutoL1: boolean }> = [
    { fieldName: 'stageModeDev', label: 'dev', allowAutoL1: true },
    { fieldName: 'stageModeTest', label: 'test', allowAutoL1: true },
    { fieldName: 'stageModeStaging', label: 'staging', allowAutoL1: true },
    { fieldName: 'stageModeProd', label: 'prod', allowAutoL1: false },
  ]

  return (
    <>
      <form
        className="grid gap-6"
        onSubmit={form.handleSubmit(handleFormSubmit)}
      >
        <Section
          description={t('aiAssistant.projectAutomation.deliveryDescription')}
          icon={<Sparkles className="size-4" />}
          title={t('aiAssistant.projectAutomation.deliveryTitle')}
          variant="bordered"
        >
          <div className="grid gap-4">
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-control border border-border bg-surface-subtle/50 px-3.5 py-3">
              <label className="text-sm font-medium text-foreground" htmlFor="delivery-diagnosis-enabled">
                {t('aiAssistant.projectAutomation.enabledLabel')}
              </label>
              <Controller
                control={form.control}
                name="deliveryDiagnosisEnabled"
                render={({ field }) => (
                  <Switch
                    checked={field.value}
                    disabled={!canManage}
                    id="delivery-diagnosis-enabled"
                    onCheckedChange={(checked) => {
                      field.onChange(checked)
                      if (checked && !form.getValues('deliveryBillingUserId') && currentUserId) {
                        form.setValue('deliveryBillingUserId', currentUserId, { shouldDirty: true, shouldValidate: true })
                      }
                    }}
                  />
                )}
              />
            </div>

            <div className="grid items-start gap-4 sm:grid-cols-2">
              <Field
                error={form.formState.errors.deliveryCooldownSeconds?.message}
                hint={t('aiAssistant.projectAutomation.cooldownHint')}
                label={t('aiAssistant.projectAutomation.cooldownLabel')}
                required
              >
                {fieldControl => (
                  <Input
                    {...fieldControl}
                    {...form.register('deliveryCooldownSeconds', { valueAsNumber: true })}
                    disabled={!canManage}
                    max={86400}
                    min={0}
                    type="number"
                  />
                )}
              </Field>

              <Field
                error={form.formState.errors.deliveryDailyLimit?.message}
                hint={t('aiAssistant.projectAutomation.dailyLimitHint')}
                label={t('aiAssistant.projectAutomation.dailyLimitLabel')}
                required
              >
                {fieldControl => (
                  <Input
                    {...fieldControl}
                    {...form.register('deliveryDailyLimit', { valueAsNumber: true })}
                    disabled={!canManage}
                    max={500}
                    min={1}
                    type="number"
                  />
                )}
              </Field>
            </div>

            <Field
              hint={t('aiAssistant.projectAutomation.billingOwnerHint')}
              label={t('aiAssistant.projectAutomation.billingOwnerLabel')}
            >
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-control border border-border bg-surface-subtle/40 px-3.5 py-2.5">
                <div className="flex min-w-0 items-center gap-2 text-sm">
                  <Wallet aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
                  <span className="truncate text-foreground">{deliveryOwnerDisplay}</span>
                  {watchedDeliveryEnabled && watchedDeliveryBillingUserId && (
                    <StatusBadge tone="info">{t('common.enabled')}</StatusBadge>
                  )}
                </div>
                {canManage && currentUserId && watchedDeliveryBillingUserId !== currentUserId && (
                  <Button
                    size="sm"
                    type="button"
                    variant="outline"
                    onClick={() => form.setValue('deliveryBillingUserId', currentUserId, { shouldDirty: true, shouldValidate: true })}
                  >
                    {t('aiAssistant.projectAutomation.bindMyWallet')}
                  </Button>
                )}
              </div>
            </Field>
          </div>
        </Section>

        <Section
          description={t('aiAssistant.projectAutomation.runtimeDescription')}
          icon={<Activity className="size-4" />}
          title={t('aiAssistant.projectAutomation.runtimeTitle')}
          variant="bordered"
        >
          <div className="grid gap-4">
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-control border border-border bg-surface-subtle/50 px-3.5 py-3">
              <label className="text-sm font-medium text-foreground" htmlFor="runtime-diagnosis-enabled">
                {t('aiAssistant.projectAutomation.runtimeEnabledLabel')}
              </label>
              <Controller
                control={form.control}
                name="runtimeDiagnosisEnabled"
                render={({ field }) => (
                  <Switch
                    checked={field.value}
                    disabled={!canManage}
                    id="runtime-diagnosis-enabled"
                    onCheckedChange={(checked) => {
                      field.onChange(checked)
                      if (checked && !form.getValues('runtimeBillingUserId') && currentUserId) {
                        form.setValue('runtimeBillingUserId', currentUserId, { shouldDirty: true, shouldValidate: true })
                      }
                    }}
                  />
                )}
              />
            </div>

            <div className="grid items-start gap-4 sm:grid-cols-2">
              <Field
                error={form.formState.errors.runtimeCooldownSeconds?.message}
                hint={t('aiAssistant.projectAutomation.cooldownHint')}
                label={t('aiAssistant.projectAutomation.cooldownLabel')}
                required
              >
                {fieldControl => (
                  <Input
                    {...fieldControl}
                    {...form.register('runtimeCooldownSeconds', { valueAsNumber: true })}
                    disabled={!canManage}
                    max={86400}
                    min={0}
                    type="number"
                  />
                )}
              </Field>

              <Field
                error={form.formState.errors.runtimeDailyLimit?.message}
                hint={t('aiAssistant.projectAutomation.dailyLimitHint')}
                label={t('aiAssistant.projectAutomation.dailyLimitLabel')}
                required
              >
                {fieldControl => (
                  <Input
                    {...fieldControl}
                    {...form.register('runtimeDailyLimit', { valueAsNumber: true })}
                    disabled={!canManage}
                    max={500}
                    min={1}
                    type="number"
                  />
                )}
              </Field>
            </div>

            <Field
              hint={t('aiAssistant.projectAutomation.runtimeBillingOwnerHint')}
              label={t('aiAssistant.projectAutomation.runtimeBillingOwnerLabel')}
            >
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-control border border-border bg-surface-subtle/40 px-3.5 py-2.5">
                <div className="flex min-w-0 items-center gap-2 text-sm">
                  <Wallet aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
                  <span className="truncate text-foreground">{runtimeOwnerDisplay}</span>
                  {watchedRuntimeEnabled && watchedRuntimeBillingUserId && (
                    <StatusBadge tone="info">{t('common.enabled')}</StatusBadge>
                  )}
                </div>
                {canManage && currentUserId && watchedRuntimeBillingUserId !== currentUserId && (
                  <Button
                    size="sm"
                    type="button"
                    variant="outline"
                    onClick={() => form.setValue('runtimeBillingUserId', currentUserId, { shouldDirty: true, shouldValidate: true })}
                  >
                    {t('aiAssistant.projectAutomation.bindMyWallet')}
                  </Button>
                )}
              </div>
            </Field>

            <Field
              hint={t('aiAssistant.projectAutomation.selfHealModeHint')}
              label={t('aiAssistant.projectAutomation.selfHealModeLabel')}
            >
              {fieldControl => (
                <Controller
                  control={form.control}
                  name="selfHealMode"
                  render={({ field }) => (
                    <Select
                      disabled={!canManage}
                      value={field.value}
                      onValueChange={val => field.onChange(val as AISelfHealMode)}
                    >
                      <SelectTrigger aria-label={t('aiAssistant.projectAutomation.selfHealModeLabel')} id={fieldControl.id}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="suggest_only">{t('aiAssistant.projectAutomation.modeSuggestOnly')}</SelectItem>
                        <SelectItem value="auto_l1">{t('aiAssistant.projectAutomation.modeAutoL1')}</SelectItem>
                        <SelectItem value="auto_l2">{t('aiAssistant.projectAutomation.modeAutoL2')}</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              )}
            </Field>

            <Field
              hint={t('aiAssistant.projectAutomation.stageOverridesHint')}
              label={t('aiAssistant.projectAutomation.stageOverridesLabel')}
            >
              <div className="grid gap-3 sm:grid-cols-2">
                {stageFields.map(item => (
                  <div className="flex items-center justify-between gap-3 rounded-control border border-border bg-surface-subtle/30 px-3 py-2" key={item.fieldName}>
                    <span className="font-mono text-xs font-medium text-foreground">{item.label}</span>
                    <Controller
                      control={form.control}
                      name={item.fieldName}
                      render={({ field }) => (
                        <Select
                          disabled={!canManage}
                          value={field.value}
                          onValueChange={val => field.onChange(val as StageModeValue)}
                        >
                          <SelectTrigger aria-label={`${t('aiAssistant.projectAutomation.stageOverridesLabel')} ${item.label}`} className="w-52">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value={INHERIT_STAGE_MODE}>{t('aiAssistant.projectAutomation.stageInherit')}</SelectItem>
                            <SelectItem value="suggest_only">{t('aiAssistant.projectAutomation.modeSuggestOnly')}</SelectItem>
                            {item.allowAutoL1 && (
                              <SelectItem value="auto_l1">{t('aiAssistant.projectAutomation.modeAutoL1')}</SelectItem>
                            )}
                            <SelectItem value="auto_l2">{t('aiAssistant.projectAutomation.modeAutoL2')}</SelectItem>
                          </SelectContent>
                        </Select>
                      )}
                    />
                  </div>
                ))}
              </div>
            </Field>

            <Field
              hint={t('aiAssistant.projectAutomation.allowedToolsHint')}
              label={t('aiAssistant.projectAutomation.allowedToolsLabel')}
            >
              <Controller
                control={form.control}
                name="allowedAutoHealTools"
                render={({ field }) => (
                  <div className="grid gap-2 sm:grid-cols-2">
                    {RUNTIME_HEAL_TOOLS.map((toolName) => {
                      const checked = field.value.includes(toolName)
                      const checkboxId = `runtime-heal-tool-${toolName}`
                      return (
                        <div
                          className="flex items-center gap-2.5 rounded-control border border-border bg-surface-subtle/30 px-3 py-2 text-xs text-foreground"
                          key={toolName}
                        >
                          <Checkbox
                            checked={checked}
                            disabled={!canManage}
                            id={checkboxId}
                            onCheckedChange={(nextChecked) => {
                              const next = nextChecked === true
                                ? Array.from(new Set([...field.value, toolName]))
                                : field.value.filter(item => item !== toolName)
                              field.onChange(next)
                            }}
                          />
                          <label className="min-w-0 flex-1 cursor-pointer truncate" htmlFor={checkboxId}>
                            {toolDisplayName(t, toolName)}
                            {' '}
                            <span className="font-mono text-muted-foreground">{`(${toolName})`}</span>
                          </label>
                        </div>
                      )
                    })}
                  </div>
                )}
              />
            </Field>

            <div className="grid items-start gap-4 sm:grid-cols-2">
              <Field
                error={form.formState.errors.maxConsecutiveAutoHeals?.message}
                hint={t('aiAssistant.projectAutomation.maxConsecutiveHint')}
                label={t('aiAssistant.projectAutomation.maxConsecutiveLabel')}
                required
              >
                {fieldControl => (
                  <Input
                    {...fieldControl}
                    {...form.register('maxConsecutiveAutoHeals', { valueAsNumber: true })}
                    disabled={!canManage}
                    max={10}
                    min={1}
                    type="number"
                  />
                )}
              </Field>

              <Field
                error={form.formState.errors.circuitBreakerWindowSeconds?.message}
                hint={t('aiAssistant.projectAutomation.circuitWindowHint')}
                label={t('aiAssistant.projectAutomation.circuitWindowLabel')}
                required
              >
                {fieldControl => (
                  <Input
                    {...fieldControl}
                    {...form.register('circuitBreakerWindowSeconds', { valueAsNumber: true })}
                    disabled={!canManage}
                    max={86400}
                    min={60}
                    type="number"
                  />
                )}
              </Field>
            </div>
          </div>
        </Section>

        {canManage && (
          <div className="flex justify-end">
            <Button
              disabled={!isDirty || Object.keys(errors).length > 0 || saveMutation.isPending}
              type="submit"
            >
              {saveMutation.isPending ? <LoaderCircle aria-hidden="true" className="size-4 animate-spin motion-reduce:animate-none" /> : <Save className="size-4" />}
              {t('common.save')}
            </Button>
          </div>
        )}
      </form>

      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('aiAssistant.projectAutomation.autoL2ConfirmAction')}
        confirmVariant="destructive"
        description={t('aiAssistant.projectAutomation.autoL2ConfirmDescription')}
        open={pendingAutoL2Values !== null}
        pending={saveMutation.isPending}
        title={t('aiAssistant.projectAutomation.autoL2ConfirmTitle')}
        onConfirm={() => {
          if (pendingAutoL2Values)
            saveMutation.mutate(pendingAutoL2Values)
        }}
        onOpenChange={(open) => {
          if (!open && !saveMutation.isPending)
            setPendingAutoL2Values(null)
        }}
      />
    </>
  )
}
