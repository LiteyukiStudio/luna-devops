import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { LoaderCircle, Save, Sparkles, Wallet } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { OverviewSkeleton } from '@/components/common/loading-states'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

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
})

type AIAutomationFormValues = z.infer<typeof aiAutomationFormSchema>

const defaultFormValues: AIAutomationFormValues = {
  deliveryDiagnosisEnabled: false,
  deliveryCooldownSeconds: 300,
  deliveryDailyLimit: 20,
  deliveryBillingUserId: '',
}

export function ProjectAIAutomationPanel({
  canManage,
  projectId,
}: {
  canManage: boolean
  projectId: string
}) {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const syncedProjectIdRef = useRef('')

  const configQuery = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-ai-automation-config', projectId],
    queryFn: ({ signal }) => api.getProjectAIAutomationConfig(projectId, signal),
    enabled: Boolean(projectId),
  })

  const form = useForm<AIAutomationFormValues>({
    resolver: zodResolver(aiAutomationFormSchema),
    defaultValues: defaultFormValues,
    mode: 'onChange',
  })

  useEffect(() => {
    if (!configQuery.data)
      return
    if (syncedProjectIdRef.current !== configQuery.data.projectId || !form.formState.isDirty) {
      syncedProjectIdRef.current = configQuery.data.projectId
      form.reset({
        deliveryDiagnosisEnabled: configQuery.data.deliveryDiagnosisEnabled,
        deliveryCooldownSeconds: configQuery.data.deliveryCooldownSeconds,
        deliveryDailyLimit: configQuery.data.deliveryDailyLimit,
        deliveryBillingUserId: configQuery.data.deliveryBillingUserId || '',
      })
    }
  }, [configQuery.data, form])

  const saveMutation = useMutation({
    mutationFn: (values: AIAutomationFormValues) => api.updateProjectAIAutomationConfig(projectId, {
      deliveryDiagnosisEnabled: values.deliveryDiagnosisEnabled,
      deliveryCooldownSeconds: values.deliveryCooldownSeconds,
      deliveryDailyLimit: values.deliveryDailyLimit,
      deliveryBillingUserId: values.deliveryBillingUserId,
    }),
    onSuccess: (updated) => {
      syncedProjectIdRef.current = updated.projectId
      queryClient.setQueryData(['project-ai-automation-config', projectId], updated)
      form.reset({
        deliveryDiagnosisEnabled: updated.deliveryDiagnosisEnabled,
        deliveryCooldownSeconds: updated.deliveryCooldownSeconds,
        deliveryDailyLimit: updated.deliveryDailyLimit,
        deliveryBillingUserId: updated.deliveryBillingUserId || '',
      })
      toast.success(t('aiAssistant.projectAutomation.savedToast'))
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('common.unavailable'))
    },
  })

  if (configQuery.isLoading)
    return <OverviewSkeleton />

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

  const watchedBillingUserId = form.watch('deliveryBillingUserId')
  const watchedEnabled = form.watch('deliveryDiagnosisEnabled')
  const currentUserId = user?.id ?? ''
  const billingOwnerDisplay = watchedBillingUserId
    ? (watchedBillingUserId === configQuery.data?.deliveryBillingUserId && configQuery.data.deliveryBillingUserName
        ? configQuery.data.deliveryBillingUserName
        : watchedBillingUserId === currentUserId
          ? (user?.name || user?.email || currentUserId)
          : watchedBillingUserId)
    : t('aiAssistant.projectAutomation.billingOwnerUnbound')

  return (
    <Section
      description={t('aiAssistant.projectAutomation.deliveryDescription')}
      icon={<Sparkles className="size-4" />}
      title={t('aiAssistant.projectAutomation.deliveryTitle')}
      variant="bordered"
    >
      <form
        className="grid gap-4"
        onSubmit={form.handleSubmit(values => saveMutation.mutate(values))}
      >
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
              <span className="truncate text-foreground">{billingOwnerDisplay}</span>
              {watchedEnabled && watchedBillingUserId && (
                <StatusBadge tone="info">{t('common.enabled')}</StatusBadge>
              )}
            </div>
            {canManage && currentUserId && watchedBillingUserId !== currentUserId && (
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

        {canManage && (
          <div className="flex justify-end border-t border-border pt-4">
            <Button
              disabled={!form.formState.isDirty || !form.formState.isValid || saveMutation.isPending}
              type="submit"
            >
              {saveMutation.isPending ? <LoaderCircle aria-hidden="true" className="size-4 animate-spin motion-reduce:animate-none" /> : <Save className="size-4" />}
              {t('common.save')}
            </Button>
          </div>
        )}
      </form>
    </Section>
  )
}
